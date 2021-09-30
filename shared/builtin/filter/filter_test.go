package filter

import (
	"context"
	"testing"

	dfv1 "github.com/argoproj-labs/argo-dataflow/api/v1alpha1"
	"github.com/stretchr/testify/assert"
)

func TestNew(t *testing.T) {
	ctx := dfv1.ContextWithMeta(context.Background(), dfv1.Meta{Source: "my-source", ID: "my-id", Time: 0})
	p, err := New(`string(msg) == "accept"`)
	assert.NoError(t, err)
	resp, err := p(ctx, []byte("accept"))
	assert.NoError(t, err)
	assert.NotNil(t, resp)
	resp, err = p(ctx, []byte("deny"))
	assert.NoError(t, err)
	assert.Nil(t, resp)
}
