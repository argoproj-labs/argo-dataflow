package sidecar

import (
	"context"

	dfv1 "github.com/argoproj-labs/argo-dataflow/api/v1alpha1"
	corev1 "k8s.io/api/core/v1"
	apierr "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

func jetStreamFromSecret(s *dfv1.JetStream, secret *corev1.Secret) error {
	s.NATSURL = dfv1.StringOr(s.NATSURL, string(secret.Data["natsUrl"]))
	if _, ok := secret.Data["authToken"]; ok {
		s.Auth = &dfv1.NATSAuth{
			Token: &corev1.SecretKeySelector{
				LocalObjectReference: corev1.LocalObjectReference{
					Name: secret.Name,
				},
				Key: "authToken",
			},
		}
	}
	return nil
}

func enrichJetStream(ctx context.Context, x *dfv1.JetStream) error {
	secret, err := secretInterface.Get(ctx, "dataflow-jetstream-"+x.Name, metav1.GetOptions{})
	if err != nil {
		if !apierr.IsNotFound(err) {
			return err
		}
	} else {
		if err = jetStreamFromSecret(x, secret); err != nil {
			return err
		}
	}
	return nil
}
