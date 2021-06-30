package v1alpha1

type Source struct {
	// +kubebuilder:default=default
	Name  string      `json:"name,omitempty" protobuf:"bytes,1,opt,name=name"`
	Cron  *Cron       `json:"cron,omitempty" protobuf:"bytes,2,opt,name=cron"`
	STAN  *STAN       `json:"stan,omitempty" protobuf:"bytes,3,opt,name=stan"`
	Kafka *Kafka      `json:"kafka,omitempty" protobuf:"bytes,4,opt,name=kafka"`
	HTTP  *HTTPSource `json:"http,omitempty" protobuf:"bytes,5,opt,name=http"`
	// +kubebuilder:default={steps: 2, jitterPercentage: 10}
	Retry Backoff `json:"retry,omitempty" protobuf:"bytes,7,opt,name=retry"`
}
