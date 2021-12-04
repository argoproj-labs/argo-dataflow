package v1alpha1

import (
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/intstr"
)

type KafkaSink struct {
	Kafka `json:",inline" protobuf:"bytes,1,opt,name=kafka"`
	Async bool `json:"async,omitempty" protobuf:"varint,2,opt,name=async"`
	// +kubebuilder:default="100Ki"
	BatchSize *resource.Quantity `json:"batchSize,omitempty" protobuf:"bytes,3,opt,name=batchSize"`
	Linger    *metav1.Duration   `json:"linger,omitempty" protobuf:"bytes,4,opt,name=linger"`
	// +kubebuilder:default="lz4"
	CompressionType string `json:"compressionType,omitempty" protobuf:"bytes,5,opt,name=compressionType"`
	// +kubebuilder:default="all"
	Acks *intstr.IntOrString `json:"acks,omitempty" protobuf:"bytes,6,opt,name=acks"`
	// +kubebuilder:default=true
	EnableIdempotence bool `json:"enableIdempotence,omitempty" protobuf:"varint,7,opt,name=enableIdempotence"`
	// +kubebuilder:default="30s"
	MessageTimeout *metav1.Duration `json:"messageTimeout,omitempty" protobuf:"bytes,8,opt,name=messageTimeout"`
}

func (m *KafkaSink) GetBatchSize() int {
	return int(m.BatchSize.Value())
}

func (m *KafkaSink) GetLingerMs() int {
	if m.Linger == nil {
		if m.Async {
			return 5
		} else {
			return 0
		}
	}
	return int(m.Linger.Milliseconds())
}

func (m *KafkaSink) GetMessageTimeoutMs() int {
	return int(m.MessageTimeout.Milliseconds())
}

func (m *KafkaSink) GetAcks() interface{} {
	if m.Acks.Type == intstr.String {
		return m.Acks.String()
	}
	return m.Acks.IntValue()
}

func (m *KafkaSink) GetMessageMaxBytes() int {
	return m.Kafka.GetMessageMaxBytes()
}
