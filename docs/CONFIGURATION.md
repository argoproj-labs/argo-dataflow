# Configuration

How to configure Dataflow.

## Sources and Sinks

The configuration for a source or sink can be set as follows:

* *Inline* - by provide all configuration in the pipeline's manifest.
* *Named* - by naming source or sink provider in the manifest.
* *Default* - by not naming or inlining.

Inline:

```
source:
  kafka:
    url: kafka-0.broker:9092
    topic: my-topic
```

Named:

```
source:
  kafka:
    name: my-kafka
    topic: my-topic
```

Configuration will be taken from `secret/dataflow-kafka-${name}`, in this example `secret/dataflow-kafka-my-kafka`.

* [Example Kafka secret](https://raw.githubusercontent.com/argoproj-labs/argo-dataflow/main/examples/dataflow-kafka-default-secret.yaml)
* [Example NATS Streaming (STAN) secret](https://raw.githubusercontent.com/argoproj-labs/argo-dataflow/main/examples/dataflow-stan-default-secret.yaml).

Default:

```
source:
  kafka:
    topic: my-topic
```

Configuration will be taken from `secret/dataflow-kafka-default`.

