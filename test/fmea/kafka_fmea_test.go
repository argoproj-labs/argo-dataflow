// +build test

package stress

import (
	. "github.com/argoproj-labs/argo-dataflow/api/v1alpha1"
	. "github.com/argoproj-labs/argo-dataflow/test"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"testing"
	"time"
)

func TestKafkaFMEA_PodDeletedDisruption(t *testing.T) {

	Setup(t)
	defer Teardown(t)

	topic := CreateKafkaTopic()

	CreatePipeline(Pipeline{
		ObjectMeta: metav1.ObjectMeta{Name: "kafka"},
		Spec: PipelineSpec{
			Steps: []StepSpec{{
				Name:    "main",
				Cat:     &Cat{},
				Sources: []Source{{Kafka: &Kafka{Topic: topic}}},
				Sinks:   []Sink{{Log: &Log{}}},
			}},
		},
	})

	WaitForPipeline()

	WaitForPod()

	n := 500 * 30
	go PumpKafkaTopic(topic, n)

	DeletePod("kafka-main-0") // delete the pod to see that we recover and continue to process messages
	WaitForPod("kafka-main-0")

	WaitForStep(TotalSunkMessages(n), 2*time.Minute)
}

func TestKafkaFMEA_KafkaServiceDisruption(t *testing.T) {

	t.SkipNow()

	Setup(t)
	defer Teardown(t)

	WaitForPod("kafka-broker-0")

	topic := CreateKafkaTopic()
	CreatePipeline(Pipeline{
		ObjectMeta: metav1.ObjectMeta{Name: "kafka"},
		Spec: PipelineSpec{
			Steps: []StepSpec{{
				Name:    "main",
				Cat:     &Cat{},
				Sources: []Source{{Kafka: &Kafka{Topic: topic}}},
				Sinks:   []Sink{{Log: &Log{}}},
			}},
		},
	})

	WaitForPipeline()

	WaitForPod()

	n := 500 * 30
	go PumpKafkaTopic(topic, n)

	PodExec("kafka-broker-0", "main", []string{"kill", "-1", "1"})
	RestartStatefulSet("kafka-broker")
	WaitForPod("kafka-broker-0")

	WaitForStep(TotalSunkMessages(n), 3*time.Minute)
	ExpectLogLine("kafka-main-0", "sidecar", "Failed to connect to broker kafka-broker:9092")
}
