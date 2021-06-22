// +build test

package stress

import (
	"testing"
	"time"

	. "github.com/argoproj-labs/argo-dataflow/api/v1alpha1"
	. "github.com/argoproj-labs/argo-dataflow/test"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

func TestStanStress(t *testing.T) {

	Setup(t)
	defer Teardown(t)
	subject := RandomSTANSubject()

	CreatePipeline(Pipeline{
		ObjectMeta: metav1.ObjectMeta{Name: "stan"},
		Spec: PipelineSpec{
			Steps: []StepSpec{{
				Name:    "main",
				Cat:     &Cat{},
				Sources: []Source{{STAN: &STAN{Subject: subject}}},
				Sinks:   []Sink{{Log: &Log{}}},
			}},
		},
	})

	stopPortForward := StartPortForward("stan-main-0")
	defer stopPortForward()
	stopMetricsLogger := StartMetricsLogger()
	defer stopMetricsLogger()

	WaitForPipeline(UntilRunning)
	WaitForPod("stan-main-0", ToBeReady)
	PumpStanSubject("argo-dataflow-system.stan."+subject, 100, 1*time.Millisecond)
	WaitForStep("main", MessagesPending)
	WaitForStep("main", NothingPending)
	WaitForever()
}
