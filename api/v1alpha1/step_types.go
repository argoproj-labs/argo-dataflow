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

package v1alpha1

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"

	"k8s.io/apimachinery/pkg/util/intstr"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/utils/pointer"
)

// +kubebuilder:object:root=true
// +kubebuilder:subresource:status
// +kubebuilder:subresource:scale:specpath=.spec.replicas,statuspath=.status.replicas,selectorpath=.status.selector
// +kubebuilder:printcolumn:name="Phase",type=string,JSONPath=`.status.phase`
// +kubebuilder:printcolumn:name="Reason",type=string,JSONPath=`.status.reason`
// +kubebuilder:printcolumn:name="Message",type=string,JSONPath=`.status.message`
// +kubebuilder:printcolumn:name="Desired",type=string,JSONPath=`.spec.replicas`
// +kubebuilder:printcolumn:name="Current",type=string,JSONPath=`.status.replicas`
type Step struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty" protobuf:"bytes,1,opt,name=metadata"`

	Spec   StepSpec   `json:"spec" protobuf:"bytes,2,opt,name=spec"`
	Status StepStatus `json:"status,omitempty" protobuf:"bytes,3,opt,name=status"`
}

func (in Step) GetPodSpec(req GetPodSpecReq) corev1.PodSpec {
	const (
		varVolumeName = "var-run-argo-dataflow"
		sshVolumeName = "ssh"
	)
	volumes := []corev1.Volume{
		{
			Name:         varVolumeName,
			VolumeSource: corev1.VolumeSource{EmptyDir: &corev1.EmptyDirVolumeSource{}},
		},
		{
			Name: sshVolumeName,
			VolumeSource: corev1.VolumeSource{
				Secret: &corev1.SecretVolumeSource{
					SecretName:  "ssh",
					DefaultMode: pointer.Int32Ptr(0o644),
				},
			},
		},
	}
	volumeMounts := []corev1.VolumeMount{{Name: varVolumeName, MountPath: PathVarRun}}
	for _, source := range in.Spec.Sources {
		if x := source.Volume; x != nil {
			name := fmt.Sprintf("source-%s", source.Name)
			volumes = append(volumes, corev1.Volume{
				Name:         name,
				VolumeSource: corev1.VolumeSource(x.AbstractVolumeSource),
			})
			volumeMounts = append(volumeMounts, corev1.VolumeMount{
				Name:      name,
				ReadOnly:  x.ReadOnly,
				MountPath: filepath.Join(PathVarRun, "sources", source.Name),
			})
		}
	}
	for _, source := range in.Spec.Sinks {
		if x := source.Volume; x != nil {
			name := fmt.Sprintf("sink-%s", source.Name)
			volumes = append(volumes, corev1.Volume{
				Name:         name,
				VolumeSource: corev1.VolumeSource(x.AbstractVolumeSource),
			})
			volumeMounts = append(volumeMounts, corev1.VolumeMount{
				Name:      name,
				MountPath: filepath.Join(PathVarRun, "sinks", source.Name),
			})
		}
	}
	step, _ := json.Marshal(in.withoutManagedFields())
	envVars := []corev1.EnvVar{
		{Name: EnvCluster, Value: req.Cluster},
		{Name: EnvDebug, Value: strconv.FormatBool(req.Debug)},
		{Name: EnvNamespace, ValueFrom: &corev1.EnvVarSource{FieldRef: &corev1.ObjectFieldSelector{FieldPath: "metadata.namespace"}}},
		{Name: EnvPod, ValueFrom: &corev1.EnvVarSource{FieldRef: &corev1.ObjectFieldSelector{FieldPath: "metadata.name"}}},
		{Name: EnvPipelineName, Value: req.PipelineName},
		{Name: EnvReplica, Value: strconv.Itoa(int(req.Replica))},
		{Name: EnvStep, Value: string(step)},
		{Name: EnvUpdateInterval, Value: req.UpdateInterval.String()},
		{Name: "GODEBUG", Value: os.Getenv("GODEBUG")},
	}

	// add all Jaeger envvar
	for _, kv := range os.Environ() {
		if strings.HasPrefix(kv, "JAEGER_") {
			parts := strings.SplitN(kv, "=", 2)
			envVars = append(envVars, corev1.EnvVar{Name: parts[0], Value: parts[1]})
		}
	}

	dropAll := &corev1.SecurityContext{
		Capabilities: &corev1.Capabilities{
			Drop: []corev1.Capability{"all"},
		},
		AllowPrivilegeEscalation: pointer.BoolPtr(false),
	}
	priorityClassName := ""
	if req.Replica == 0 {
		priorityClassName = "lead-replica"
	}
	return corev1.PodSpec{
		Hostname:           req.Hostname,
		Subdomain:          req.Subdomain,
		Volumes:            append(in.Spec.Volumes, volumes...),
		RestartPolicy:      in.Spec.RestartPolicy,
		NodeSelector:       in.Spec.NodeSelector,
		ServiceAccountName: in.Spec.ServiceAccountName,
		SecurityContext: &corev1.PodSecurityContext{
			RunAsNonRoot: pointer.BoolPtr(true),
			RunAsUser:    pointer.Int64Ptr(9653),
		},
		PriorityClassName: priorityClassName,
		Affinity:          in.Spec.Affinity,
		Tolerations:       in.Spec.Tolerations,
		InitContainers: []corev1.Container{
			{
				Name:            CtrInit,
				Image:           req.RunnerImage,
				ImagePullPolicy: req.PullPolicy,
				Args:            []string{"init"},
				Env:             envVars,
				VolumeMounts: append(volumeMounts, corev1.VolumeMount{
					Name:      sshVolumeName,
					ReadOnly:  true,
					MountPath: "/.ssh",
				}),
				Resources:       standardResources,
				SecurityContext: dropAll,
			},
		},
		ImagePullSecrets: req.ImagePullSecrets,
		Containers: []corev1.Container{
			{
				Name:            CtrSidecar,
				Image:           req.RunnerImage,
				ImagePullPolicy: req.PullPolicy,
				Args:            []string{"sidecar"},
				Env:             envVars,
				VolumeMounts:    volumeMounts,
				Resources:       req.Sidecar.Resources,
				Ports: []corev1.ContainerPort{
					{ContainerPort: 3570},
				},
				ReadinessProbe: &corev1.Probe{
					Handler: corev1.Handler{
						HTTPGet: &corev1.HTTPGetAction{Scheme: "HTTPS", Path: "/ready", Port: intstr.FromInt(3570)},
					},
				},
				Lifecycle: &corev1.Lifecycle{
					PreStop: &corev1.Handler{
						HTTPGet: &corev1.HTTPGetAction{
							Path:   "/pre-stop?source=kubernetes",
							Port:   intstr.FromInt(3570),
							Scheme: "HTTPS",
						},
					},
				},
				SecurityContext: dropAll,
			},
			in.Spec.getType().getContainer(getContainerReq{
				imageFormat:     req.ImageFormat,
				imagePullPolicy: req.PullPolicy,
				lifecycle: &corev1.Lifecycle{
					PreStop: &corev1.Handler{
						Exec: &corev1.ExecAction{
							Command: []string{PathPreStop},
						},
					},
				},
				runnerImage:     req.RunnerImage,
				securityContext: dropAll,
				volumeMounts:    volumeMounts,
			}),
		},
	}
}

func (in Step) GetHeadlessServiceName(pipelineName string) string {
	return "step-" + pipelineName + "-" + in.Spec.Name
}

func (in Step) GetServiceObj(serviceName, pipelineName string, isHeadless bool) *corev1.Service {
	svc := &corev1.Service{
		ObjectMeta: metav1.ObjectMeta{
			Namespace:       in.Namespace,
			Name:            serviceName,
			OwnerReferences: []metav1.OwnerReference{*metav1.NewControllerRef(in.GetObjectMeta(), StepGroupVersionKind)},
			// useful for auto-detecting the service as exporting Prometheus
			Labels: map[string]string{
				KeyStepName:     in.Spec.Name,
				KeyPipelineName: pipelineName,
			},
		},
		Spec: corev1.ServiceSpec{
			Ports: []corev1.ServicePort{
				{Port: 443, TargetPort: intstr.FromInt(3570)},
			},
			Selector: map[string]string{
				KeyPipelineName: pipelineName,
				KeyStepName:     in.Spec.Name,
			},
		},
	}
	if isHeadless {
		svc.Spec.ClusterIP = "None"
		svc.Spec.Ports[0].Port = 3570
	}
	return svc
}

func (in Step) withoutManagedFields() Step {
	y := *in.DeepCopy()
	y.ManagedFields = nil
	return y
}

// +kubebuilder:object:root=true

type StepList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty" protobuf:"bytes,1,opt,name=metadata"`
	Items           []Step `json:"items" protobuf:"bytes,2,rep,name=items"`
}

func init() {
	SchemeBuilder.Register(&Step{}, &StepList{})
}
