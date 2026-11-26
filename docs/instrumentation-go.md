# Capture an instrumented Go application

Tracelet does not add tracing to arbitrary code. Your application must create and end OpenTelemetry spans, configure an OTLP/HTTP Protobuf trace exporter, and shut its SDK down with a bounded flush deadline. Existing exporter configuration remains effective unless you explicitly request `--override-otel`.

From this source checkout, build the CLI and bundled sample:

```sh
make build example
./dist/tracelet init
./dist/tracelet run -- ./dist/tracelet-go-api
```

In a second terminal:

```sh
curl http://127.0.0.1:8080/orders/123
curl http://127.0.0.1:8080/orders/error
```

The sample creates an HTTP root, authentication and slow lookup spans, an event, a client span and a local downstream server span with propagated W3C context. The error route records an exception and error status. Source attributes are explicitly added to custom spans. Sample request bodies are synthetic and are not captured. Select a different sample port using `--addr 127.0.0.1:8081` after the child executable.

Stop with Ctrl+C. Tracelet forwards the signal to the child process group, keeps ingestion available during configured shutdown grace, then drains the receiver and closes storage. The sample flushes the SDK within three seconds. A second interrupt or expired grace forces process-tree termination; telemetry that the application has not exported may be lost. Normal child exits retain their exit code. Browser inspection is added by subsequent planned tasks.

The required `--` separates Tracelet options from child arguments. For an already-instrumented application:

```sh
./dist/tracelet run -- /absolute/path/to/your-app --your-argument
```

When no explicit exporter endpoint or protocol exists, Tracelet injects the trace-specific HTTP endpoint and `http/protobuf`. Nonempty generic or trace-specific endpoint/protocol settings are preserved, with notices that never echo credentials. Explicit trace settings take precedence over generic ones. Empty values count as unset. Existing service name/resource service metadata and SDK disable/exporter/sampler settings are respected. To intentionally redirect trace endpoint and protocol:

```sh
./dist/tracelet run --override-otel -- /absolute/path/to/your-app
```

Override changes only the child environment's trace endpoint/protocol. It does not edit your shell, source, SDK options, collector configuration, sampler, or tracing enablement. Hardcoded exporter options in application code can still override environment settings. Use `OTEL_SERVICE_NAME` for a deliberate service name; otherwise Tracelet defaults it to the configured project name if resource attributes do not already supply one.

The sample uses the pinned Go SDK in `go.mod`. It manually instruments `net/http`, uses batch export with gzip, and shows explicit shutdown. Install/build dependencies require network access once; the running sample, Tracelet receiver and SQLite store need no external service. macOS lifecycle tests pass; Windows/Linux native lifecycle checks remain release gates.
