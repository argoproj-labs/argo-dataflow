from argo_dataflow import pipeline, kafka


def handler(msg):
    return msg


if __name__ == '__main__':
    (pipeline("104-java16")
     .owner('argoproj-labs')
     .describe("""This example is of the Java 16 handler.

[Learn about handlers](../docs/HANDLERS.md)""")
     .step(
        (kafka('input-topic')
         .code(code="""import java.util.Map;

public class Handler {
    public static byte[] Handle(byte[] msg, Map<String,String> context) throws Exception {
        return msg;
    }
}""", runtime='java16')
         .kafka('output-topic')
         ))
     .save())
