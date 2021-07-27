package v1alpha1

import (
	corev1 "k8s.io/api/core/v1"
)

type Git struct {
	Image           string                    `json:"image" protobuf:"bytes,1,opt,name=image"`
	Command         []string                  `json:"command,omitempty" protobuf:"bytes,6,rep,name=command"`
	URL             string                    `json:"url" protobuf:"bytes,2,opt,name=url"`
	AuthUsernameKey *corev1.SecretKeySelector `json:"authUsernameKey,omitempty" protobuf:"bytes,7,opt,name=authUsernameKey"`
	AuthPasswordKey *corev1.SecretKeySelector `json:"authPasswordKey,omitempty" protobuf:"bytes,8,opt,name=authPasswordKey"`
	// +kubebuilder:default=.
	Path string `json:"path,omitempty" protobuf:"bytes,3,opt,name=path"`
	// +kubebuilder:default=main
	Branch string          `json:"branch,omitempty" protobuf:"bytes,4,opt,name=branch"`
	Env    []corev1.EnvVar `json:"env,omitempty" protobuf:"bytes,5,rep,name=env"`
}

func (in Git) getContainer(req getContainerReq) corev1.Container {
	return containerBuilder{}.
		init(req).
		image(in.Image).
		command(in.Command...).
		appendEnv(in.Env...).
		workingDir(PathWorkingDir).
		build()
}
