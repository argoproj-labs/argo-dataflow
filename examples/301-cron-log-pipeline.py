from dsls.python import pipeline, cron

if __name__ == '__main__':
    (pipeline("cron-log")
     .describe("""This example uses a cron source and a log sink.

## Cron

You can format dates using a "layout":

https://golang.org/pkg/time/#Time.Format

By default, the layout is RFC3339.

* Cron sources are **unreliable**. Messages will not be sent when a pod is not running, which can happen at any time in Kubernetes.
* Cron sources must not be scaled to zero.

## Log

This logs the message.
""")
     .step(
        (cron('*/3 * * * * *', layout='15:04:05')
         .cat('main')
         .log())
    ).dump())