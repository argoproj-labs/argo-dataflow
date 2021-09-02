package s3

import (
	"context"
	"encoding/json"
	"fmt"
	"os"

	apierr "k8s.io/apimachinery/pkg/api/errors"

	dfv1 "github.com/argoproj-labs/argo-dataflow/api/v1alpha1"
	"github.com/argoproj-labs/argo-dataflow/runner/sidecar/sink"
	"github.com/aws/aws-sdk-go-v2/aws"
	v4 "github.com/aws/aws-sdk-go-v2/aws/signer/v4"
	"github.com/aws/aws-sdk-go-v2/service/s3"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	v1 "k8s.io/client-go/kubernetes/typed/core/v1"
)

type s3Sink struct {
	client *s3.Client
	bucket string
}

type message struct {
	Key  string `json:"key"`
	Path string `json:"path"`
}

func New(ctx context.Context, secretInterface v1.SecretInterface, x dfv1.S3Sink) (sink.Interface, error) {
	var accessKeyID string
	{
		secretName := x.Credentials.AccessKeyID.Name
		secret, err := secretInterface.Get(ctx, secretName, metav1.GetOptions{})
		if err != nil {
			return nil, fmt.Errorf("failed to get secret %q: %w", secretName, err)
		}
		accessKeyID = string(secret.Data[x.Credentials.AccessKeyID.Key])
	}
	var secretAccessKey string
	{
		secretName := x.Credentials.SecretAccessKey.Name
		secret, err := secretInterface.Get(ctx, secretName, metav1.GetOptions{})
		if err != nil {
			return nil, fmt.Errorf("failed to get secret %q: %w", secretName, err)
		}
		secretAccessKey = string(secret.Data[x.Credentials.SecretAccessKey.Key])
	}
	var sessionToken string
	{
		secretName := x.Credentials.SessionToken.Name
		secret, err := secretInterface.Get(ctx, secretName, metav1.GetOptions{})
		if err == nil {
			sessionToken = string(secret.Data[x.Credentials.SessionToken.Key])
		} else {
			// it is okay for sessionToken to be missing
			if !apierr.IsNotFound(err) {
				return nil, err
			}
		}
	}
	options := s3.Options{
		Region: x.Region,
		Credentials: aws.CredentialsProviderFunc(func(ctx context.Context) (aws.Credentials, error) {
			return aws.Credentials{AccessKeyID: accessKeyID, SecretAccessKey: secretAccessKey, SessionToken: sessionToken}, nil
		}),
	}
	if e := x.Endpoint; e != nil {
		options.EndpointResolver = s3.EndpointResolverFunc(func(region string, options s3.EndpointResolverOptions) (aws.Endpoint, error) {
			return aws.Endpoint{URL: e.URL, SigningRegion: region, HostnameImmutable: true}, nil
		})
	}
	return s3Sink{client: s3.New(options), bucket: x.Bucket}, nil
}

func (h s3Sink) Sink(msg []byte) error {
	m := &message{}
	if err := json.Unmarshal(msg, m); err != nil {
		return err
	}
	f, err := os.Open(m.Path)
	if err != nil {
		return fmt.Errorf("failed to open %q: %w", m.Path, err)
	}
	_, err = h.client.PutObject(context.Background(), &s3.PutObjectInput{
		Bucket: &h.bucket,
		Key:    &m.Key,
		Body:   f,
	}, s3.WithAPIOptions(
		// https://aws.github.io/aws-sdk-go-v2/docs/sdk-utilities/s3/#unseekable-streaming-input
		v4.SwapComputePayloadSHA256ForUnsignedPayloadMiddleware,
	))
	return err
}
