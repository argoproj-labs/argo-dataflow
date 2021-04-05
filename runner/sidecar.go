package main

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io/ioutil"
	"net/http"
	"os"
	"reflect"
	"strconv"
	"time"

	"github.com/Shopify/sarama"
	"github.com/nats-io/stan.go"
	apierr "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	runtimeutil "k8s.io/apimachinery/pkg/util/runtime"

	dfv1 "github.com/argoproj-labs/argo-dataflow/api/v1alpha1"
)

var (
	updateInterval = 15 * time.Second
	config         = sarama.NewConfig()
	replica        = 0
	pipelineName   = os.Getenv(dfv1.EnvPipelineName)
	namespace      = os.Getenv(dfv1.EnvNamespace)
	spec           = &dfv1.StepSpec{}
	status         = &dfv1.StepStatus{
		SourceStatues: dfv1.SourceStatuses{},
		SinkStatues:   dfv1.SinkStatuses{},
	}
)

func Sidecar(ctx context.Context) error {

	if err := unmarshallSpec(); err != nil {
		return err
	}

	if err := enrichSpec(ctx); err != nil {
		return err
	}

	if v, err := strconv.Atoi(os.Getenv(dfv1.EnvReplica)); err != nil {
		return err
	} else {
		replica = v
	}
	log.WithValues("stepName", spec.Name, "pipelineName", pipelineName, "replica", replica).Info("config")

	config.ClientID = dfv1.CtrSidecar

	toSink, err := connectSink()
	if err != nil {
		return err
	}

	connectOut(toSink)

	toMain, err := connectTo()
	if err != nil {
		return err
	}

	if err := connectSources(ctx, toMain); err != nil {
		return err
	}

	go func() {
		defer runtimeutil.HandleCrash(runtimeutil.PanicHandlers...)
		lastStatus := status.DeepCopy()
		for {
			if !reflect.DeepEqual(lastStatus, status) {
				patch := dfv1.Json(&dfv1.Step{Status: status})
				log.Info("patching step status (sinks/sources)", "patch", patch)
				if _, err := dynamicInterface.
					Resource(dfv1.StepsGroupVersionResource).
					Namespace(namespace).
					Patch(
						ctx,
						pipelineName+"-"+spec.Name,
						types.MergePatchType,
						[]byte(patch),
						metav1.PatchOptions{},
						"status",
					); err != nil {
					log.Error(err, "failed to patch step status")
				}
				// once we're reported pending, it possible we won't get anymore messages for a while, so the value
				// we have will be wrong
				for i, s := range status.SourceStatues {
					s.Pending = 0
					status.SourceStatues[i] = s
				}
				lastStatus = status.DeepCopy()
			}
			time.Sleep(updateInterval)
		}
	}()
	log.Info("ready")
	<-ctx.Done()
	log.Info("done")
	return nil
}

func enrichSpec(ctx context.Context) error {
	secrets := kubernetesInterface.CoreV1().Secrets(namespace)
	for i, source := range spec.Sources {
		if s := source.STAN; s != nil {
			secret, err := secrets.Get(ctx, "dataflow-stan-"+dfv1.StringOr(s.Name, "default"), metav1.GetOptions{})
			if err != nil {
				if !apierr.IsNotFound(err) {
					return err
				}
			} else {
				s.NATSURL = dfv1.StringOr(s.NATSURL, string(secret.Data["natsUrl"]))
				s.ClusterID = dfv1.StringOr(s.ClusterID, string(secret.Data["clusterId"]))
				s.SubjectPrefix = dfv1.SubjectPrefixOr(s.SubjectPrefix, dfv1.SubjectPrefix(secret.Data["subjectPrefix"]))
			}
			switch s.SubjectPrefix {
			case dfv1.SubjectPrefixNamespaceName:
				s.Subject = fmt.Sprintf("%s.%s", namespace, s.Subject)
			case dfv1.SubjectPrefixNamespacedPipelineName:
				s.Subject = fmt.Sprintf("%s.%s.%s", namespace, pipelineName, s.Subject)
			}
			source.STAN = s
		} else if k := source.Kafka; k != nil {
			secret, err := secrets.Get(ctx, "dataflow-kafka-"+dfv1.StringOr(k.Name, "default"), metav1.GetOptions{})
			if err != nil {
				if !apierr.IsNotFound(err) {
					return err
				}
			} else {
				k.URL = dfv1.StringOr(k.URL, string(secret.Data["url"]))
			}
			source.Kafka = k
		}
		spec.Sources[i] = source
	}

	for i, sink := range spec.Sinks {
		if s := sink.STAN; s != nil {
			secret, err := secrets.Get(ctx, "dataflow-stan-"+dfv1.StringOr(s.Name, "default"), metav1.GetOptions{})
			if err != nil {
				if !apierr.IsNotFound(err) {
					return err
				}
			} else {
				s.NATSURL = dfv1.StringOr(s.NATSURL, string(secret.Data["natsUrl"]))
				s.ClusterID = dfv1.StringOr(s.ClusterID, string(secret.Data["clusterId"]))
				s.SubjectPrefix = dfv1.SubjectPrefixOr(s.SubjectPrefix, dfv1.SubjectPrefix(secret.Data["subjectPrefix"]))
			}
			switch s.SubjectPrefix {
			case dfv1.SubjectPrefixNamespaceName:
				s.Subject = fmt.Sprintf("%s.%s", namespace, s.Subject)
			case dfv1.SubjectPrefixNamespacedPipelineName:
				s.Subject = fmt.Sprintf("%s.%s.%s", namespace, pipelineName, s.Subject)
			}
			sink.STAN = s
		} else if k := sink.Kafka; k != nil {
			secret, err := secrets.Get(ctx, "dataflow-kafka-"+dfv1.StringOr(k.Name, "default"), metav1.GetOptions{})
			if err != nil {
				if !apierr.IsNotFound(err) {
					return err
				}
			} else {
				k.URL = string(secret.Data["url"])
			}
			sink.Kafka = k
		}
		spec.Sinks[i] = sink
	}

	return nil
}

func unmarshallSpec() error {
	if err := json.Unmarshal([]byte(os.Getenv(dfv1.EnvStepSpec)), spec); err != nil {
		return fmt.Errorf("failed to unmarshall spec: %w", err)
	}
	return nil
}

func connectSources(ctx context.Context, toMain func([]byte) error) error {
	for i, source := range spec.Sources {
		if s := source.STAN; s != nil {
			clientID := fmt.Sprintf("%s-%s-%d-source-%d", pipelineName, spec.Name, replica, i)
			log.Info("connecting to source", "type", "stan", "url", s.NATSURL, "clusterID", s.ClusterID, "clientID", clientID, "subject", s.Subject)
			sc, err := stan.Connect(s.ClusterID, clientID, stan.NatsURL(s.NATSURL))
			if err != nil {
				return fmt.Errorf("failed to connect to stan url=%s clusterID=%s clientID=%s subject=%s: %w", s.NATSURL, s.ClusterID, clientID, s.Subject, err)
			}
			closers = append(closers, sc.Close)
			if sub, err := sc.QueueSubscribe(s.Subject, fmt.Sprintf("%s-%s", pipelineName, spec.Name), func(m *stan.Msg) {
				log.Info("◷ stan →", "m", short(m.Data))
				status.SourceStatues.Set(source.Name, replica, short(m.Data))
				if err := toMain(m.Data); err != nil {
					log.Error(err, "failed to send message from stan to main")
				} else {
					debug.Info("✔ stan → ", "subject", s.Subject)
				}
			}, stan.DeliverAllAvailable(), stan.DurableName(clientID)); err != nil {
				return fmt.Errorf("failed to subscribe: %w", err)
			} else {
				closers = append(closers, sub.Close)
				go func() {
					defer runtimeutil.HandleCrash(runtimeutil.PanicHandlers...)
					for {
						if pending, _, err := sub.Pending(); err != nil {
							log.Error(err, "failed to get pending", "subject", s.Subject)
						} else {
							debug.Info("setting pending", "subject", s.Subject, "pending", pending)
							status.SourceStatues.SetPending(source.Name, uint64(pending))
						}
						time.Sleep(updateInterval)
					}
				}()
			}
		} else if k := source.Kafka; k != nil {
			log.Info("connecting kafka source", "type", "kafka", "url", k.URL, "topic", k.Topic)
			client, err := sarama.NewClient([]string{k.URL}, config) // I am not giving any configuration
			if err != nil {
				return fmt.Errorf("failed to create kafka client: %w", err)
			}
			closers = append(closers, client.Close)
			group, err := sarama.NewConsumerGroup([]string{k.URL}, pipelineName+"-"+spec.Name, config)
			if err != nil {
				return fmt.Errorf("failed to create kafka consumer group: %w", err)
			}
			closers = append(closers, group.Close)
			handler := &handler{source.Name, toMain, 0}
			go func() {
				defer runtimeutil.HandleCrash(runtimeutil.PanicHandlers...)
				if err := group.Consume(ctx, []string{k.Topic}, handler); err != nil {
					log.Error(err, "failed to create kafka consumer")
				}
			}()
			go func() {
				defer runtimeutil.HandleCrash(runtimeutil.PanicHandlers...)
				for {
					newestOffset, err := client.GetOffset(k.Topic, 0, sarama.OffsetNewest)
					if err != nil {
						log.Error(err, "failed to get offset", "topic", k.Topic)
					} else {
						pending := uint64(newestOffset - handler.offset)
						debug.Info("setting pending", "type", "kafka", "topic", k.Topic, "pending", pending, "newestOffset", newestOffset, "offset", handler.offset)
						status.SourceStatues.SetPending(source.Name, pending)
					}
					time.Sleep(updateInterval)
				}
			}()
		} else {
			return fmt.Errorf("source misconfigured")
		}
	}
	return nil
}

func connectTo() (func([]byte) error, error) {
	in := spec.GetIn()
	if in == nil {
		log.Info("no in interface configured")
		return func(i []byte) error {
			return fmt.Errorf("no in interface configured")
		}, nil
	} else if in.FIFO {
		log.Info("opened input FIFO")
		fifo, err := os.OpenFile(dfv1.PathFIFOIn, os.O_WRONLY, os.ModeNamedPipe)
		if err != nil {
			return nil, fmt.Errorf("failed to open input FIFO: %w", err)
		}
		closers = append(closers, fifo.Close)
		return func(data []byte) error {
			debug.Info("◷ source → fifo")
			if _, err := fifo.Write(data); err != nil {
				return fmt.Errorf("failed to write message from source to main via FIFO: %w", err)
			}
			if _, err := fifo.Write([]byte("\n")); err != nil {
				return fmt.Errorf("failed to write new line from source to main via FIFO: %w", err)
			}
			debug.Info("✔ source → fifo")
			return nil
		}, nil
	} else if in.HTTP != nil {
		log.Info("HTTP in interface configured")
		log.Info("waiting for HTTP in interface to be ready")
		for {
			if resp, err := http.Get("http://localhost:8080/ready"); err == nil && resp.StatusCode == 200 {
				log.Info("HTTP in interface ready")
				break
			}
			time.Sleep(3 * time.Second)
		}
		return func(data []byte) error {
			debug.Info("◷ source → http")
			resp, err := http.Post("http://localhost:8080/messages", "application/json", bytes.NewBuffer(data))
			if err != nil {
				return fmt.Errorf("failed to sent message from source to main via HTTP: %w", err)
			}
			if resp.StatusCode >= 300 {
				return fmt.Errorf("failed to sent message from source to main via HTTP: %s", resp.Status)
			}
			debug.Info("✔ source → http")
			return nil
		}, nil
	} else {
		return nil, fmt.Errorf("in interface misconfigured")
	}
}

func connectOut(toSink func([]byte) error) {
	log.Info("FIFO out interface configured")
	go func() {
		defer runtimeutil.HandleCrash(runtimeutil.PanicHandlers...)
		err := func() error {
			fifo, err := os.OpenFile(dfv1.PathFIFOOut, os.O_RDONLY, os.ModeNamedPipe)
			if err != nil {
				return fmt.Errorf("failed to open output FIFO: %w", err)
			}
			defer fifo.Close()
			log.Info("opened output FIFO")
			scanner := bufio.NewScanner(fifo)
			for scanner.Scan() {
				debug.Info("◷ fifo → sink")
				if err := toSink(scanner.Bytes()); err != nil {
					return fmt.Errorf("failed to send message from main to sink: %w", err)
				}
				debug.Info("✔ fifo → sink")
			}
			if err = scanner.Err(); err != nil {
				return fmt.Errorf("scanner error: %w", err)
			}
			return nil
		}()
		if err != nil {
			log.Error(err, "failed to received message from FIFO")
			os.Exit(1)
		}
	}()
	log.Info("HTTP out interface configured")
	http.HandleFunc("/messages", func(w http.ResponseWriter, r *http.Request) {
		data, err := ioutil.ReadAll(r.Body)
		if err != nil {
			log.Error(err, "failed to read message body from main via HTTP")
			w.WriteHeader(500)
			return
		}
		debug.Info("◷ http → sink")
		if err := toSink(data); err != nil {
			log.Error(err, "failed to send message from main to sink")
			w.WriteHeader(500)
			return
		}
		debug.Info("✔ http → sink")
		w.WriteHeader(200)
	})
	go func() {
		defer runtimeutil.HandleCrash(runtimeutil.PanicHandlers...)
		log.Info("starting HTTP server")
		err := http.ListenAndServe(":3569", nil)
		if err != nil {
			log.Error(err, "failed to listen-and-server")
			os.Exit(1)
		}
	}()
}

func connectSink() (func([]byte) error, error) {
	var toSinks []func([]byte) error
	for i, sink := range spec.Sinks {
		if s := sink.STAN; s != nil {
			clientID := fmt.Sprintf("%s-%s-%d-sink-%d", pipelineName, spec.Name, replica, i)
			log.Info("connecting sink", "type", "stan", "url", s.NATSURL, "clusterID", s.ClusterID, "clientID", clientID, "subject", s.Subject)
			sc, err := stan.Connect(s.ClusterID, clientID, stan.NatsURL(s.NATSURL))
			if err != nil {
				return nil, fmt.Errorf("failed to connect to stan url=%s clusterID=%s clientID=%s subject=%s: %w", s.NATSURL, s.ClusterID, clientID, s.Subject, err)
			}
			closers = append(closers, sc.Close)
			toSinks = append(toSinks, func(m []byte) error {
				status.SinkStatues.Set(sink.Name, replica, short(m))
				log.Info("◷ → stan", "subject", s.Subject, "m", short(m))
				return sc.Publish(s.Subject, m)
			})
		} else if k := sink.Kafka; k != nil {
			log.Info("connecting sink", "type", "kafka", "url", k.URL, "topic", k.Topic)
			config.Producer.Return.Successes = true
			producer, err := sarama.NewSyncProducer([]string{k.URL}, config)
			if err != nil {
				return nil, fmt.Errorf("failed to create kafka producer: %w", err)
			}
			closers = append(closers, producer.Close)
			toSinks = append(toSinks, func(m []byte) error {
				status.SinkStatues.Set(sink.Name, replica, short(m))
				log.Info("◷ → kafka", "topic", k.Topic, "m", short(m))
				_, _, err := producer.SendMessage(&sarama.ProducerMessage{
					Topic: k.Topic,
					Value: sarama.ByteEncoder(m),
				})
				return err
			})
		} else {
			return nil, fmt.Errorf("sink misconfigured")
		}
	}
	return func(m []byte) error {
		for _, s := range toSinks {
			if err := s(m); err != nil {
				return err
			}
		}
		return nil
	}, nil
}
