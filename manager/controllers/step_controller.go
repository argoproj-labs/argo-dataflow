/*


Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

package controllers

import (
	"context"
	"fmt"
	"strconv"
	"time"

	dfv1 "github.com/argoproj-labs/argo-dataflow/api/v1alpha1"
	"github.com/argoproj-labs/argo-dataflow/manager/controllers/scaling"
	"github.com/argoproj-labs/argo-dataflow/shared/containerkiller"
	"github.com/argoproj-labs/argo-dataflow/shared/util"
	"github.com/go-logr/logr"
	corev1 "k8s.io/api/core/v1"
	apierr "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/labels"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/dynamic"
	"k8s.io/client-go/tools/cache"
	"k8s.io/client-go/tools/record"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
)

const stepFinalizer = "step-controller"

// StepReconciler reconciles a Step object.
type StepReconciler struct {
	client.Client
	Log                 logr.Logger
	Scheme              *runtime.Scheme
	Recorder            record.EventRecorder
	ContainerKiller     containerkiller.Interface
	DynamicInterface    dynamic.Interface
	MetricsCacheHandler *scaling.MetricsCacheHandler
	Cluster             string
}

type hash struct {
	RunnerImage string        `json:"runnerImage"`
	StepSpec    dfv1.StepSpec `json:"stepSpec"`
}

// +kubebuilder:rbac:groups=dataflow.argoproj.io,resources=steps,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=dataflow.argoproj.io,resources=steps/status,verbs=get;update;patch
// +kubebuilder:rbac:groups=,resources=pods,verbs=get;watch;list;create
// +kubebuilder:rbac:groups=,resources=services,verbs=get;watch;list;create
// +kubebuilder:rbac:groups="",resources=events,verbs=create;patch
func (r *StepReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	log := r.Log.WithValues("step", req.NamespacedName.String())
	step := &dfv1.Step{}
	if err := r.Get(ctx, req.NamespacedName, step); err != nil {
		return ctrl.Result{}, client.IgnoreNotFound(err)
	}

	if step.GetDeletionTimestamp() != nil {
		if controllerutil.ContainsFinalizer(step, stepFinalizer) {
			if err := r.stopMetricsCacheLoop(step); err != nil {
				return ctrl.Result{}, err
			}
			controllerutil.RemoveFinalizer(step, stepFinalizer)
			if err := r.Client.Update(ctx, step); err != nil {
				return ctrl.Result{}, err
			}
		}
		return ctrl.Result{}, nil
	}

	pipelineName := step.GetLabels()[dfv1.KeyPipelineName]
	stepName := step.Spec.Name

	log.Info("reconciling")

	currentReplicas := int(step.Status.Replicas)
	if step.Spec.Scale.DesiredReplicas != "" {
		if err := r.startMetricsCacheLoop(step); err != nil {
			return ctrl.Result{}, fmt.Errorf("failed to start metrics cache loop: %w", err)
		}
		desiredReplicas, err := scaling.GetDesiredReplicas(*step)
		if err != nil {
			return ctrl.Result{}, err
		}
		if int(step.Spec.Replicas) != desiredReplicas {
			log.Info("auto-scaling step", "currentReplicas", currentReplicas, "desiredReplicas", desiredReplicas)
			if _, err := r.DynamicInterface.
				Resource(dfv1.StepGroupVersionResource).
				Namespace(step.Namespace).
				Patch(
					ctx,
					step.Name,
					types.MergePatchType,
					[]byte(util.MustJSON(
						map[string]interface{}{
							"spec": map[string]interface{}{
								"replicas": desiredReplicas,
							},
						})),
					metav1.PatchOptions{},
					"scale",
				); err != nil {
				return ctrl.Result{}, fmt.Errorf("failed to scale step: %w", err)
			}
			return ctrl.Result{}, nil
		}
	}

	desiredReplicas := int(step.Spec.Replicas)

	oldStatus := step.Status.DeepCopy()
	if currentReplicas != desiredReplicas || step.Status.Selector == "" {
		log.Info("replicas changed", "currentReplicas", currentReplicas, "desiredReplicas", desiredReplicas)
		step.Status.Replicas = uint32(desiredReplicas)
		step.Status.LastScaledAt = metav1.Time{Time: time.Now()}
		r.Recorder.Eventf(step, "Normal", eventReason(currentReplicas, desiredReplicas), "Scaling from %d to %d", currentReplicas, desiredReplicas)
	}

	selector, _ := labels.Parse(dfv1.KeyPipelineName + "=" + pipelineName + "," + dfv1.KeyStepName + "=" + stepName)
	hash := util.MustHash(hash{runnerImage, step.Spec.WithOutReplicas()}) // we must remove data (e.g. replicas) which does not change the pod, otherwise it would cause the pod to be re-created all the time
	step.Status.Phase, step.Status.Reason, step.Status.Message = dfv1.StepUnknown, "", ""
	step.Status.Selector = selector.String()

	ownerReferences := []metav1.OwnerReference{*metav1.NewControllerRef(step.GetObjectMeta(), dfv1.StepGroupVersionKind)}
	headlessSvcName := step.GetHeadlessServiceName()

	for replica := 0; replica < desiredReplicas; replica++ {
		podName := fmt.Sprintf("%s-%d", step.Name, replica)
		_labels := map[string]string{}
		annotations := map[string]string{}
		if x := step.Spec.Metadata; x != nil {
			for k, v := range x.Annotations {
				annotations[k] = v
			}
			for k, v := range x.Labels {
				_labels[k] = v
			}
		}
		_labels[dfv1.KeyStepName] = stepName
		_labels[dfv1.KeyPipelineName] = pipelineName
		annotations[dfv1.KeyReplica] = strconv.Itoa(replica)
		annotations[dfv1.KeyHash] = hash
		annotations[dfv1.KeyDefaultContainer] = dfv1.CtrMain
		annotations[dfv1.KeyKillCmd(dfv1.CtrMain)] = util.MustJSON([]string{dfv1.PathKill, "1"})
		annotations[dfv1.KeyKillCmd(dfv1.CtrSidecar)] = util.MustJSON([]string{dfv1.PathKill, "1"})

		var reqImagePullSecrets []corev1.LocalObjectReference

		if len(step.Spec.ImagePullSecrets) > 0 {
			reqImagePullSecrets = step.Spec.ImagePullSecrets
		} else if len(imagePullSecrets) > 0 {
			for _, element := range imagePullSecrets {
				reqImagePullSecrets = append(reqImagePullSecrets, corev1.LocalObjectReference{Name: element})
			}
		}

		if err := r.Client.Create(
			ctx,
			&corev1.Pod{
				ObjectMeta: metav1.ObjectMeta{
					Namespace:       step.Namespace,
					Name:            podName,
					Labels:          _labels,
					Annotations:     annotations,
					OwnerReferences: ownerReferences,
				},
				Spec: step.GetPodSpec(
					dfv1.GetPodSpecReq{
						Cluster:          r.Cluster,
						PipelineName:     pipelineName,
						Replica:          int32(replica),
						ImageFormat:      imageFormat,
						RunnerImage:      runnerImage,
						PullPolicy:       pullPolicy,
						UpdateInterval:   updateInterval,
						StepStatus:       step.Status,
						Sidecar:          step.Spec.Sidecar,
						ImagePullSecrets: reqImagePullSecrets,
						Hostname:         podName,
						Subdomain:        headlessSvcName,
					},
				),
			},
		); apierr.IsAlreadyExists(err) {
			// ignore
		} else if err != nil {
			x := dfv1.MinStepPhaseMessage(dfv1.NewStepPhaseMessage(step.Status.Phase, step.Status.Reason, step.Status.Message), dfv1.NewStepPhaseMessage(dfv1.StepFailed, "", fmt.Sprintf("failed to create pod %s: %v", podName, err)))
			step.Status.Phase, step.Status.Reason, step.Status.Message = x.GetPhase(), x.GetReason(), x.GetMessage()
		} else {
			log.Info("pod created", "pod", podName)
		}
	}

	serviceObjMap := make(map[string]*corev1.Service)
	serviceObjMap[headlessSvcName] = step.GetServiceObj(headlessSvcName, pipelineName, true)
	for _, s := range step.Spec.Sources {
		serviceName := pipelineName + "-" + stepName
		if x := s.HTTP; x != nil {
			if n := x.ServiceName; n != "" {
				serviceName = n
			}
			serviceObjMap[serviceName] = step.GetServiceObj(serviceName, pipelineName, false)
		} else if x := s.S3; x != nil {
			serviceObjMap[serviceName] = step.GetServiceObj(serviceName, pipelineName, false)
		} else if x := s.Volume; x != nil {
			serviceObjMap[serviceName] = step.GetServiceObj(serviceName, pipelineName, false)
		}
	}

	for _, obj := range serviceObjMap {
		if err := r.Client.Create(ctx, obj); util.IgnoreAlreadyExists(err) != nil {
			x := dfv1.MinStepPhaseMessage(dfv1.NewStepPhaseMessage(step.Status.Phase, step.Status.Reason, step.Status.Message), dfv1.NewStepPhaseMessage(dfv1.StepFailed, "", fmt.Sprintf("failed to create service %s: %v", step.Name, err)))
			step.Status.Phase, step.Status.Reason, step.Status.Message = x.GetPhase(), x.GetReason(), x.GetMessage()
		}
	}

	pods := &corev1.PodList{}
	if err := r.Client.List(ctx, pods, &client.ListOptions{Namespace: step.Namespace, LabelSelector: selector}); err != nil {
		return ctrl.Result{}, fmt.Errorf("failed to list pods: %w", err)
	}

	for _, pod := range pods.Items {
		replica, err := strconv.Atoi(pod.GetAnnotations()[dfv1.KeyReplica])
		if err != nil {
			return ctrl.Result{}, fmt.Errorf("failed to parse replica of pod %q: %w", pod.Name, err)
		}
		if replica >= desiredReplicas || hash != pod.GetAnnotations()[dfv1.KeyHash] {
			log.Info("deleting excess pod", "podName", pod.Name)
			if err := r.Client.Delete(ctx, &pod); client.IgnoreNotFound(err) != nil {
				x := dfv1.MinStepPhaseMessage(dfv1.NewStepPhaseMessage(step.Status.Phase, step.Status.Reason, step.Status.Message), dfv1.NewStepPhaseMessage(dfv1.StepFailed, "", fmt.Sprintf("failed to delete excess pod %s: %v", pod.Name, err)))
				step.Status.Phase, step.Status.Reason, step.Status.Message = x.GetPhase(), x.GetReason(), x.GetMessage()
			}
		} else {
			phase, reason, message := inferPhase(pod)
			x := dfv1.MinStepPhaseMessage(dfv1.NewStepPhaseMessage(step.Status.Phase, step.Status.Reason, step.Status.Message), dfv1.NewStepPhaseMessage(phase, reason, message))
			step.Status.Phase, step.Status.Reason, step.Status.Message = x.GetPhase(), x.GetReason(), x.GetMessage()

			// if the main container has terminated, kill all sidecars
			mainCtrTerminated := false
			for _, s := range pod.Status.ContainerStatuses {
				mainCtrTerminated = mainCtrTerminated || (s.Name == dfv1.CtrMain && s.State.Terminated != nil && s.State.Terminated.ExitCode == 0)
			}
			log.Info("pod", "name", pod.Name, "mainCtrTerminated", mainCtrTerminated)
			if mainCtrTerminated {
				for _, s := range pod.Status.ContainerStatuses {
					if s.Name != dfv1.CtrMain {
						if err := r.ContainerKiller.KillContainer(pod, s.Name); err != nil {
							log.Error(err, "failed to kill container", "pod", pod.Name, "container", s.Name)
						}
					}
				}
			}
		}
	}

	if notEqual, patch := util.NotEqual(oldStatus, step.Status); notEqual {
		log.Info("updating step", "patch", patch)
		if err := r.Status().Update(ctx, step); err != nil {
			if apierr.IsConflict(err) {
				return ctrl.Result{}, nil // conflict is ok, we will reconcile again soon
			} else {
				return ctrl.Result{}, fmt.Errorf("failed to update status: %w", err)
			}
		}
	}

	requeueAfter, err := scaling.RequeueAfter(*step)
	if err != nil {
		return ctrl.Result{}, err
	}
	if requeueAfter > 0 {
		log.Info("requeue", "requeueAfter", requeueAfter.String())
	}
	return ctrl.Result{RequeueAfter: requeueAfter}, nil
}

func (r *StepReconciler) startMetricsCacheLoop(step *dfv1.Step) error {
	key, err := cache.MetaNamespaceKeyFunc(step)
	if err != nil {
		return fmt.Errorf("failed to get key for step object: %w", err)
	}
	if r.MetricsCacheHandler.Contains(key) {
		return nil
	}
	return r.MetricsCacheHandler.StartWatchingStep(step)
}

func (r *StepReconciler) stopMetricsCacheLoop(step *dfv1.Step) error {
	key, err := cache.MetaNamespaceKeyFunc(step)
	if err != nil {
		return fmt.Errorf("failed to get key for step object: %w", err)
	}
	if !r.MetricsCacheHandler.Contains(key) {
		return nil
	}
	return r.MetricsCacheHandler.StopWatchingStep(step)
}

func eventReason(currentReplicas, desiredReplicas int) string {
	eventType := "ScaleDown"
	if desiredReplicas > currentReplicas {
		eventType = "ScaleUp"
	}
	return eventType
}

func (r *StepReconciler) SetupWithManager(mgr ctrl.Manager) error {
	if r.Cluster == "" {
		return fmt.Errorf("cluster must be set")
	}
	return ctrl.NewControllerManagedBy(mgr).
		For(&dfv1.Step{}).
		Owns(&corev1.Pod{}).
		Owns(&corev1.Service{}).
		Complete(r)
}
