from argo_dataflow import pipeline, kafka

if __name__ == '__main__':
    (pipeline("103-autoscaling")
     .owner('argoproj-labs')
     .describe("""This is an example of having multiple replicas for a single step.

Replicas are automatically scaled up and down depending on the number of messages pending processing.

The ratio is defined as the number of pending messages per replica:

```
replicas = pending / ratio
```

The number of replicas will not scale beyond the min/max bounds (except when *peeking*, see below):

```
min <= replicas <= max
```

* `min` is used as the initial number of replicas.
* If `ratio` is undefined no scaling can occur; `max` is meaningless.
* If `ratio` is defined but `max` is not, the step may scale to infinity.
* If `max` and `ratio` are undefined, then the number of replicas is `min`.
* In this example, because the ratio is 1000, if 2000 messages pending, two replicas will be started.
* To prevent scaling up and down repeatedly - scale up or down occurs a maximum of once a minute.
* The same message will not be send to two different replicas.

### Scale-To-Zero and Peeking

You can scale to zero by setting `minReplicas: 0`. The number of replicas will start at zero, and periodically be scaled
to 1  so it can "peek" the the message queue. The number of pending messages is measured and the target number
of replicas re-calculated.""")
     .step(
        (kafka('input-topic')
         .cat('main')
         .scale(0, 4, 1000)
         .kafka('output-topic'))
    )
     .save())
