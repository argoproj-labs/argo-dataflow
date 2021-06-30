// +build test

package test

import (
	"k8s.io/client-go/dynamic"
	"k8s.io/client-go/kubernetes"
	_ "k8s.io/client-go/plugin/pkg/client/auth"
	ctrl "sigs.k8s.io/controller-runtime"
	"testing"
)

const (
	namespace = "argo-dataflow-system"
)

var (
	restConfig             = ctrl.GetConfigOrDie()
	dynamicInterface       = dynamic.NewForConfigOrDie(restConfig)
	kubernetesInterface    = kubernetes.NewForConfigOrDie(restConfig)
	stopTestAPIPortForward func()
)

func Setup(t *testing.T) {
	DeletePipelines()
	WaitForPodsToBeDeleted()

	WaitForPod("zookeeper-0")
	WaitForPod("kafka-broker-0")
	WaitForPod("nats-0")
	WaitForPod("stan-0")

	stopTestAPIPortForward = StartPortForward("testapi-0", 8378)
}

func Teardown(*testing.T) {
	stopTestAPIPortForward()
}
