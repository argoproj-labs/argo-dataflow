// +build test

package stress

import (
	"testing"
	"time"

	. "github.com/argoproj-labs/argo-dataflow/api/v1alpha1"
	. "github.com/argoproj-labs/argo-dataflow/test"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

func TestHTTPFMEA_PodDeletedDisruption_OneReplica(t *testing.T) {
	defer Setup(t)()

	CreatePipeline(Pipeline{
		ObjectMeta: metav1.ObjectMeta{Name: "http"},
		Spec: PipelineSpec{
			Steps: []StepSpec{{
				Name:    "main",
				Cat:     &Cat{},
				Sources: []Source{{HTTP: &HTTPSource{}}},
				Sinks:   []Sink{{Log: &Log{}}},
			}},
		},
	})

	WaitForPipeline()

	n := 15

	// with a single replica, if you loose a replica, you loose service
	go PumpHTTPTolerantly(n)

	WaitForPipeline(UntilMessagesSunk)

	DeletePod("http-main-0") // delete the pod to see that we recover and continue to process messages
	WaitForPod("http-main-0")

	WaitForStep(TotalSourceMessages(n), 1*time.Minute)
	WaitForStep(NoRecentErrors)
}

func TestHTTPFMEA_PodDeletedDisruption_TwoReplicas(t *testing.T) {
	defer Setup(t)()

	CreatePipeline(Pipeline{
		ObjectMeta: metav1.ObjectMeta{Name: "http"},
		Spec: PipelineSpec{
			Steps: []StepSpec{{
				Name:     "main",
				Cat:      &Cat{},
				Replicas: 2,
				Sources:  []Source{{HTTP: &HTTPSource{}}},
				Sinks:    []Sink{{Log: &Log{}}},
			}},
		},
	})

	WaitForPipeline()

	WaitForPod("http-main-0")
	WaitForPod("http-main-1")
	WaitForService()

	n := 15

	go PumpHTTPTolerantly(n)

	WaitForPipeline(UntilMessagesSunk)

	DeletePod("http-main-0") // delete the pod to see that we continue to process messages
	WaitForPod("http-main-0")

	WaitForStep(TotalSunkMessages(n), 1*time.Minute)
	WaitForStep(NoRecentErrors)
}
