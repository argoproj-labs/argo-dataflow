package kafka

import (
	"context"
	"fmt"
	"sync"
	"time"

	dfv1 "github.com/argoproj-labs/argo-dataflow/api/v1alpha1"
	"github.com/argoproj-labs/argo-dataflow/runner/sidecar/monitor"
	sharedkafka "github.com/argoproj-labs/argo-dataflow/runner/sidecar/shared/kafka"
	"github.com/argoproj-labs/argo-dataflow/runner/sidecar/source"
	sharedutil "github.com/argoproj-labs/argo-dataflow/shared/util"
	"github.com/confluentinc/confluent-kafka-go/kafka"
	"github.com/go-logr/logr"
	"github.com/opentracing/opentracing-go"
	"k8s.io/apimachinery/pkg/util/wait"
	corev1 "k8s.io/client-go/kubernetes/typed/core/v1"
)

type kafkaSource struct {
	logger     logr.Logger
	sourceName string
	sourceURN  string
	mntr       monitor.Interface
	consumer   *kafka.Consumer
	topic      string
	wg         *sync.WaitGroup
	channels   map[int32]chan *kafka.Message
	process    source.Process
}

func New(ctx context.Context, secretInterface corev1.SecretInterface, mntr monitor.Interface, consumerGroupID, sourceName, sourceURN string, replica int, x dfv1.KafkaSource, process source.Process) (source.Interface, error) {
	logger := sharedutil.NewLogger().WithValues("source", sourceName)
	config, err := sharedkafka.GetConfig(ctx, secretInterface, x.KafkaConfig)
	if err != nil {
		return nil, err
	}
	config["group.id"] = consumerGroupID
	config["group.instance.id"] = fmt.Sprintf("%s/%d", consumerGroupID, replica)
	config["enable.auto.commit"] = false
	config["enable.auto.offset.store"] = false
	if x.StartOffset == "First" {
		config["auto.offset.reset"] = "earliest"
	} else {
		config["auto.offset.reset"] = "latest"
	}
	logger.Info("Kafka config", "config", sharedutil.MustJSON(sharedkafka.RedactConfigMap(config)))
	// https://github.com/confluentinc/confluent-kafka-go/blob/master/examples/consumer_example/consumer_example.go
	consumer, err := kafka.NewConsumer(&config)
	if err != nil {
		return nil, err
	}

	s := &kafkaSource{
		logger:     logger,
		mntr:       mntr,
		sourceName: sourceName,
		sourceURN:  sourceURN,
		consumer:   consumer,
		topic:      x.Topic,
		channels:   map[int32]chan *kafka.Message{}, // partition -> messages
		wg:         &sync.WaitGroup{},
		process:    process,
	}

	if err = consumer.Subscribe(x.Topic, func(consumer *kafka.Consumer, event kafka.Event) error {
		return s.rebalanced(ctx, event)
	}); err != nil {
		return nil, err
	}

	go wait.JitterUntilWithContext(ctx, s.startPollLoop, 3*time.Second, 1.2, true)

	return s, nil
}

func (s *kafkaSource) processMessage(ctx context.Context, msg *kafka.Message) error {
	span, ctx := opentracing.StartSpanFromContext(ctx, fmt.Sprintf("kafka-source-%s", s.sourceName))
	defer span.Finish()
	return s.process(
		dfv1.ContextWithMeta(
			ctx,
			dfv1.Meta{
				Source: s.sourceURN,
				ID:     fmt.Sprintf("%d-%d", msg.TopicPartition.Partition, msg.TopicPartition.Offset),
				Time:   msg.Timestamp.Unix(),
			},
		),
		msg.Value,
	)
}

func (s *kafkaSource) assignedPartition(ctx context.Context, partition int32) {
	logger := s.logger.WithValues("partition", partition)
	if _, ok := s.channels[partition]; !ok {
		logger.Info("assigned partition")
		s.mntr.AssignedPartition(ctx, s.sourceURN, partition)
		s.channels[partition] = make(chan *kafka.Message, 256)
		go wait.JitterUntilWithContext(ctx, func(ctx context.Context) {
			s.consumePartition(ctx, partition)
		}, 3*time.Second, 1.2, true)
	}
}

func (s *kafkaSource) revokedPartition(ctx context.Context, partition int32) {
	if _, ok := s.channels[partition]; ok {
		s.logger.Info("revoked partition", "partition", partition)
		close(s.channels[partition])
		delete(s.channels, partition)
	}
	s.mntr.RevokedPartition(ctx, s.sourceURN, partition)
}

func (s *kafkaSource) startPollLoop(ctx context.Context) {
	s.logger.Info("starting poll loop")
	for {
		// shutdown will be blocked for the amount of time we specify here
		ev := s.consumer.Poll(5 * 1000)
		select {
		case <-ctx.Done():
			return
		default:
			switch e := ev.(type) {
			case *kafka.Message:
				func() {
					defer func() {
						// Fact 1 - if you send a message on a closed channel, you get a panic.
						// Fact 2 - it is impossible to know if a channel is close in Golang.
						// we need to recover any panic, so we don't pollute the logs
						if r := recover(); r != nil {
							s.logger.Info("recovered from panic while queuing message", "recover", fmt.Sprint(r))
						}
					}()
					s.channels[e.TopicPartition.Partition] <- e
				}()
			case kafka.Error:
				s.logger.Error(fmt.Errorf("%v", e), "poll error")
			case nil:
				// noop
			default:
				s.logger.Info("ignored event", "event", ev)
			}
		}
	}
}

func (s *kafkaSource) Close() error {
	s.logger.Info("closing partition channels")
	for key, ch := range s.channels {
		delete(s.channels, key)
		close(ch)
	}
	s.logger.Info("waiting for partition consumers to finish")
	s.wg.Wait()
	s.logger.Info("closing consumer")
	return s.consumer.Close()
}

func (s *kafkaSource) GetPending(context.Context) (uint64, error) {
	// TODO - only works for assigned partitions
	toppars, err := s.consumer.Assignment()
	if err != nil {
		return 0, err
	}
	toppars, err = s.consumer.Committed(toppars, 3*1000)
	if err != nil {
		return 0, err
	}
	var low, high int64
	var pending int64
	for _, t := range toppars {
		low, high, err = s.consumer.QueryWatermarkOffsets(*t.Topic, t.Partition, 3*1000)
		if err != nil {
			return 0, err
		}
		offset := int64(t.Offset)
		if t.Offset == kafka.OffsetInvalid {
			offset = low
		}
		if d := high - offset; d > 0 {
			pending += d
		}
	}
	return uint64(pending), nil
}

func (s *kafkaSource) rebalanced(ctx context.Context, event kafka.Event) error {
	s.logger.Info("re-balance", "event", event.String())
	switch e := event.(type) {
	case kafka.AssignedPartitions:
		for _, p := range e.Partitions {
			s.assignedPartition(ctx, p.Partition)
		}
	case kafka.RevokedPartitions:
		for _, p := range e.Partitions {
			s.revokedPartition(ctx, p.Partition)
		}
	}
	return nil
}

func (s *kafkaSource) consumePartition(ctx context.Context, partition int32) {
	logger := s.logger.WithValues("partition", partition)
	logger.Info("consuming partition")
	s.wg.Add(1)
	defer func() {
		logger.Info("done consuming partition")
		s.wg.Done()
	}()
	for msg := range s.channels[partition] {
		offset := int64(msg.TopicPartition.Offset)
		logger := logger.WithValues("offset", offset)
		if !s.mntr.Accept(s.sourceName, s.sourceURN, partition, offset) {
			logger.Info("not accepting message")
		} else if err := s.processMessage(ctx, msg); err != nil {
			logger.Error(err, "failed to process message")
		} else {
			s.mntr.Mark(s.sourceURN, partition, offset)
			if _, err := s.consumer.CommitMessage(msg); err != nil {
				logger.Error(err, "failed to commit message")
			}
		}
	}
}
