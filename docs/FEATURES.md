# Features

A list of features.

See [Wikipedia](https://en.wikipedia.org/wiki/Software_release_life_cycle#Stages_of_development) for an explanation of
alpha/beta/stable.

| Name  | Alpha | Beta | Stable |
|---|---|---|---|
| API | | v0.0.59 | |
| [Automatic pipeline garbage collection](GC.md) | v0.0.59 | v0.0.60 | |
| Auto-scaling | | v0.0.59 | |
| Cat step | | v0.0.59 | |
| Container step | | v0.0.59 | |
| [Code step](CODE.md) | v0.0.59 | v0.0.70 | |
| Container killer | | v0.0.59 | |
| Container step | v0.0.59 | v0.0.70 | |
| Cron source | v0.0.59 | |
| Dedupe step | v0.0.59 | || |
| Expand step | v0.0.59 | v0.0.70 | |
| Expression based scaling | v0.0.90 | | |
| Filter step | v0.0.59 | v0.0.70 | |
| FMEA tests | | v0.0.59 | |
| [Generator step](STEPS.md#Generator-step) | v0.0.59 | | |
| Graceful step termination | v0.0.59 | | |
| Group step | v0.0.59 | | |
| [Git step](GIT.md) | v0.0.59 | v0.0.70 | |
| Golang SDK | v0.0.59 | v0.0.70 | |
| Golang runtime | v0.0.59 | v0.0.70 | |
| Kubernetes manifests | | v0.0.59 | |
| HPA support | v0.0.59 | v0.0.71 | |
| Java runtime | v0.0.59 | v0.0.70 | |
| HTTP sink | v0.0.59 | | |
| HTTP source | v0.0.59 | | |
| Kafka sink | v0.0.59 | | |
| Kafka source | v0.0.59 | | |
| Log sink | |  v0.0.59 |  |
| Map step | v0.0.59 | v0.0.70 | |
| NATS Streaming sink | v0.0.59 | | |
| NATS Streaming source | v0.0.59 | | |
| NodeJS SDK | v0.0.78 | | |
| NodeJS runtime | v0.0.84 | | |
| Non-terminating pipelines | | v0.0.59 | |
| [Prometheus metrics](METRICS.md) | | v0.0.59 | |
| Python SDK | | v0.0.59 | |
| Python runtime | v0.0.59 | v0.0.70 | |
| Scale-to-zero (aka "peeking") | | v0.0.70 | |
| S3 source | v0.0.74 | | |
| S3 sink | v0.0.75 | | |
| Stress tests | | v0.0.59 | |
| Terminating pipelines | v0.0.59 | v0.0.70 | |
| Terminating steps | v0.0.59 | v0.0.70 | |
| User interface | | v0.0.59 | |
| Volume source | v0.0.91 | | |

Common gating criteria to get from alpha to beta:

* Unit, end-to-end, FMEA and stress tests (as applicable).
* Used in production.
* DSL and SDK.
* Examples and documentation.

And from beta to stable:

* Stable API.
* Provide in production.
* Adopted by multiple users.
