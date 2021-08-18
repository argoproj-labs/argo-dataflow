package v1alpha1

type Source struct {
	// +kubebuilder:default=default
	Name   string        `json:"name,omitempty" protobuf:"bytes,1,opt,name=name"`
	Cron   *Cron         `json:"cron,omitempty" protobuf:"bytes,2,opt,name=cron"`
	STAN   *STAN         `json:"stan,omitempty" protobuf:"bytes,3,opt,name=stan"`
	Kafka  *KafkaSource  `json:"kafka,omitempty" protobuf:"bytes,4,opt,name=kafka"`
	HTTP   *HTTPSource   `json:"http,omitempty" protobuf:"bytes,5,opt,name=http"`
	S3     *S3Source     `json:"s3,omitempty" protobuf:"bytes,8,opt,name=s3"`
	DB     *DBSource     `json:"db,omitempty" protobuf:"bytes,6,opt,name=db"`
	Volume *VolumeSource `json:"volume,omitempty" protobuf:"bytes,9,opt,name=volume"`
	// +kubebuilder:default={duration: "100ms", steps: 20, factorPercentage: 200, jitterPercentage: 10}
	Retry Backoff `json:"retry,omitempty" protobuf:"bytes,7,opt,name=retry"`
}
