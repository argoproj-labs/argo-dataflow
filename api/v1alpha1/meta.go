package v1alpha1

import (
	"context"
	"fmt"
	"net/http"
	"time"
)

var (
	// MetaID is a unique ID for the message.
	// Required.
	// https://github.com/cloudevents/spec/blob/master/spec.md#id
	MetaID = "dataflow-id"
	// MetaSource is the source of the messages as a Unique Resource Identifier (URI).
	// Required.
	// https://github.com/cloudevents/spec/blob/master/spec.md#source-1
	MetaSource = "dataflow-source"
	// MetaTime is the time of the message. As meta-data, this might be different to the event-time (which might be within the message).
	// For example, it might be the last-modified time of a file, but the file itself was created at another time.
	// Optional.
	// https://github.com/cloudevents/spec/blob/master/spec.md#time
	MetaTime = "dataflow-time"
)

type Meta struct {
	Source string `json:"source" protobuf:"bytes,1,opt,name=source"`
	ID     string `json:"id" protobuf:"bytes,2,opt,name=id"`
	// UnixTime
	Time int64 `json:"time,omitempty" protobuf:"varint,3,opt,name=time"`
}

func ContextWithMeta(ctx context.Context, m Meta) context.Context {
	return context.WithValue(
		context.WithValue(
			context.WithValue(
				ctx,
				MetaSource,
				m.Source,
			),
			MetaID,
			m.ID,
		),
		MetaTime,
		m.Time,
	)
}

func MetaFromContext(ctx context.Context) (Meta, error) {
	source, ok := ctx.Value(MetaSource).(string)
	if !ok {
		return Meta{}, fmt.Errorf("failed to get source from context")
	}
	id, ok := ctx.Value(MetaID).(string)
	if !ok {
		return Meta{}, fmt.Errorf("failed to get id from context")
	}
	t, ok := ctx.Value(MetaTime).(int64)
	if !ok {
		return Meta{}, fmt.Errorf("failed to get time from context")
	}
	return Meta{
		Source: source,
		ID:     id,
		Time:   t,
	}, nil
}

func MetaInject(ctx context.Context, h http.Header) error {
	m, err := MetaFromContext(ctx)
	if err != nil {
		return err
	}
	h.Add(MetaSource, m.Source)
	h.Add(MetaID, m.ID)
	h.Add(MetaTime, time.Unix(m.Time, 0).Format(time.RFC3339))
	return nil
}

func MetaExtract(ctx context.Context, h http.Header) context.Context {
	t, _ := time.Parse(time.RFC3339, h.Get(MetaTime))
	return ContextWithMeta(ctx,
		Meta{
			Source: h.Get(MetaSource),
			ID:     h.Get(MetaID),
			Time:   t.Unix(),
		},
	)
}
