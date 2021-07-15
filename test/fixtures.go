// +build test

package test

import (
	"fmt"
	"os"
	"runtime/debug"
	"testing"

	"k8s.io/client-go/dynamic"
	"k8s.io/client-go/kubernetes"
	_ "k8s.io/client-go/plugin/pkg/client/auth"

	ctrl "sigs.k8s.io/controller-runtime"
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

func SkipIfCI(t *testing.T) {
	if os.Getenv("CI") == "true" {
		t.SkipNow()
	}
}

func Setup(t *testing.T) (teardown func()) {
	DeletePipelines()
	WaitForPodsToBeDeleted()

	WaitForPod("zookeeper-0")
	WaitForPod("kafka-broker-0")
	WaitForPod("nats-0")
	WaitForPod("stan-0")
	WaitForPod("testapi-0")

	stopTestAPIPortForward = StartPortForward("testapi-0", 8378)

	ResetCount()

	return func() {
		stopTestAPIPortForward()
		r := recover() // tests should panic on error, we recover so we can run other tests
		if r != nil {
			t.Log(fmt.Sprintf("❌ FAIL: %s %v", t.Name(), r))
			debug.PrintStack()
			t.Fail()
		} else if t.Failed() {
			t.Log(fmt.Sprintf("❌ FAIL: %s", t.Name()))
		} else {
			t.Log(fmt.Sprintf("✅ PASS: %s", t.Name()))
		}
	}
}
