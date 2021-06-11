from argo_dataflow import pipeline, kafka


def handler(msg, context):
    return msg


if __name__ == '__main__':
    (pipeline("104-python3-9")
     .owner('argoproj-labs')
     .describe("""This example is of the Python 3.9 handler.

[Learn about handlers](../docs/HANDLERS.md)""")
     .annotate('dataflow.argoproj.io/timeout', '2m')
     .step(
        (kafka('input-topic')
         .handler('main', handler)
         .kafka('output-topic')
         ))
     .save())
