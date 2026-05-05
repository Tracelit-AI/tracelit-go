# Tracelit SDK — tracelit-go

Go SDK for [Tracelit](https://tracelit.app) observability and monitoring.

A thin, idiomatic wrapper over the [OpenTelemetry Go SDK](https://github.com/open-telemetry/opentelemetry-go) that configures OTLP/HTTP export of **traces, metrics, and logs** to the Tracelit ingest endpoint — with deep nested span support, automatic stack trace capture on errors, and plug-and-play bridges for every major Go logger.

## Installation

```bash
go get github.com/tracelit-ai/tracelit-go
```

## Quick start

```go
package main

import (
    "context"
    "log"
    "os"

    "github.com/tracelit-ai/tracelit-go"
)

func main() {
    sdk, err := tracelit.New(
        tracelit.WithAPIKey(os.Getenv("TRACELIT_API_KEY")),
        tracelit.WithServiceName("payments-api"),
        tracelit.WithEnvironment("production"),
    )
    if err != nil {
        log.Fatal(err)
    }
    defer sdk.Shutdown(context.Background())

    // Your application code here.
}
```

Or using environment variables (no code changes needed):

```bash
export TRACELIT_API_KEY=your-api-key
export TRACELIT_SERVICE_NAME=payments-api
export TRACELIT_ENVIRONMENT=production
```

```go
if err := tracelit.Configure(); err != nil {
    log.Fatal(err)
}
defer tracelit.Shutdown(context.Background())
```

## Configuration

| Option | Env variable | Default | Description |
|--------|-------------|---------|-------------|
| `WithAPIKey(key)` | `TRACELIT_API_KEY` | — | **Required.** Your Tracelit API key |
| `WithServiceName(name)` | `TRACELIT_SERVICE_NAME` | — | **Required.** Service name shown in Tracelit |
| `WithEnvironment(env)` | `TRACELIT_ENVIRONMENT` | `production` | Deployment environment tag |
| `WithEndpoint(url)` | `TRACELIT_ENDPOINT` | `https://ingest.tracelit.app` | Override ingest URL (self-hosted only) |
| `WithSampleRate(rate)` | `TRACELIT_SAMPLE_RATE` | `1.0` | Head-based sampling rate 0.0–1.0 |
| `WithEnabled(bool)` | `TRACELIT_ENABLED` | `true` | Set `false` to disable all telemetry |
| `WithResourceAttributes(map)` | — | — | Extra resource attributes on every signal |

## Tracing

### Automatic instrumentation

Once `tracelit.New` (or `tracelit.Configure`) is called the global OTel
`TracerProvider` is set. Any library that uses the OTel API
(e.g. `otelhttp`, `otelgrpc`, `otelsql`) will automatically send spans to
Tracelit without further configuration.

### Manual spans

```go
import "github.com/tracelit-ai/tracelit-go"

func processOrder(ctx context.Context, order Order) error {
    ctx, span := tracelit.StartSpan(ctx, "process-order",
        tracelit.WithSpanAttributes(map[string]any{
            "order.id":       order.ID,
            "order.currency": order.Currency,
        }),
    )
    defer span.End()

    if err := validateOrder(ctx, order); err != nil {
        span.RecordError(err) // records error + full stack trace
        return err
    }

    // Child span — automatically a child of the span above.
    ctx, paySpan := tracelit.StartClientSpan(ctx, "charge-payment-provider")
    defer paySpan.End()

    if err := chargeCard(ctx, order); err != nil {
        paySpan.RecordError(err,
            tracelit.WithErrorAttributes(map[string]any{
                "payment.gateway": "stripe",
                "payment.amount":  order.TotalCents,
            }),
        )
        return err
    }

    span.AddEvent("payment-captured", map[string]any{"amount": order.TotalCents})
    return nil
}
```

### Span kinds

```go
// SpanKindInternal (default) — internal operation
ctx, span := tracelit.StartSpan(ctx, "validate-input")

// SpanKindServer — inbound request handler
ctx, span := tracelit.StartServerSpan(ctx, "POST /orders")

// SpanKindClient — outbound call (HTTP, gRPC, DB)
ctx, span := tracelit.StartClientSpan(ctx, "stripe.charge")

// SpanKindProducer / Consumer — message queue
ctx, span := tracelit.StartProducerSpan(ctx, "orders.created publish")
ctx, span := tracelit.StartConsumerSpan(ctx, "orders.created consume")
```

### Stack traces on errors

`RecordError` captures a full goroutine stack trace automatically:

```go
ctx, span := tracelit.StartSpan(ctx, "risky-op")
defer span.End()

if err := riskyOperation(); err != nil {
    span.RecordError(err) // sets codes.Error + attaches exception.stacktrace
    return err
}
```

Panic recovery with stack trace:

```go
ctx, span := tracelit.StartSpan(ctx, "risky-op")
defer func() {
    if r := recover(); r != nil {
        span.RecordPanic(r)           // re-panics by default
        // span.RecordPanic(r, tracelit.WithSwallowPanic()) // absorb the panic
    }
    span.End()
}()
```

### Retrieving trace/span IDs (for log correlation)

```go
traceID := tracelit.TraceID(ctx) // "4bf92f3577b34da6a3ce929d0e0e4736"
spanID  := tracelit.SpanID(ctx)  // "00f067aa0ba902b7"
```

### Passing spans across goroutines

```go
ctx, span := tracelit.StartSpan(ctx, "fan-out")
defer span.End()

// Pass ctx to goroutines — child spans automatically attach.
for _, item := range items {
    go func(item Item) {
        childCtx, child := tracelit.StartSpan(ctx, "process-item")
        defer child.End()
        processItem(childCtx, item)
    }(item)
}
```

## HTTP instrumentation

```go
import (
    "github.com/tracelit-ai/tracelit-go/middleware"
    "net/http"
)

// Server — wrap entire mux or individual handlers
mux := http.NewServeMux()
mux.HandleFunc("/api/orders", handleOrders)
http.ListenAndServe(":8080", middleware.NewHTTPHandler(mux))

// Client — wrap transport for outgoing requests
client := &http.Client{
    Transport: middleware.NewHTTPTransport(nil),
}
resp, err := client.Get("https://api.stripe.com/v1/charges")
```

## gRPC instrumentation

```go
import "github.com/tracelit-ai/tracelit-go/middleware"

// Server
grpcServer := grpc.NewServer(
    grpc.StatsHandler(middleware.NewGRPCServerHandler()),
)

// Client
conn, err := grpc.NewClient("localhost:50051",
    grpc.WithStatsHandler(middleware.NewGRPCClientHandler()),
)
```

## Logger bridges

All bridges route log records to the Tracelit log pipeline and automatically
inject `trace_id` and `span_id` from the context so logs are correlated with
traces in the Tracelit UI.

### slog (standard library)

```go
import (
    "log/slog"
    "github.com/tracelit-ai/tracelit-go/bridge"
)

// Replace the default global slog logger.
slog.SetDefault(slog.New(bridge.NewSlogHandler()))

// Logs are correlated with the active span when ctx is passed.
slog.InfoContext(ctx, "order created", "order_id", order.ID)
```

### zap (Uber)

```go
import (
    "github.com/tracelit-ai/tracelit-go/bridge"
    "go.uber.org/zap"
    "go.uber.org/zap/zapcore"
)

// Option 1: OTel-only logger
logger := bridge.NewZapLogger("payments-api")

// Option 2: tee to both stdout and OTel
stdoutLogger, _ := zap.NewProduction()
logger := zap.New(zapcore.NewTee(
    stdoutLogger.Core(),
    bridge.NewZapCore("payments-api"),
), zap.AddCaller())

logger.Info("order created", zap.String("order_id", order.ID))
```

### logrus

```go
import (
    "github.com/sirupsen/logrus"
    "github.com/tracelit-ai/tracelit-go/bridge"
)

logrus.AddHook(bridge.NewLogrusHook())

// Pass ctx to get trace correlation.
logrus.WithContext(ctx).WithField("order_id", order.ID).Info("order created")
```

### zerolog

```go
import (
    "os"
    "github.com/rs/zerolog"
    "github.com/rs/zerolog/log"
    "github.com/tracelit-ai/tracelit-go/bridge"
)

// Replace the global zerolog logger.
log.Logger = zerolog.New(bridge.NewZerologWriter(os.Stderr)).
    With().Timestamp().Logger()

// Use log.Ctx(ctx) to attach trace correlation.
log.Ctx(ctx).Info().Str("order_id", order.ID).Msg("order created")
```

## Metrics

```go
// Named counter
requests, err := tracelit.Counter("http.server.requests",
    metric.WithDescription("Total HTTP requests"),
    metric.WithUnit("{request}"),
)
requests.Add(ctx, 1, metric.WithAttributes(
    attribute.String("method", r.Method),
    attribute.Int("status", code),
))

// Histogram
latency, err := tracelit.Histogram("http.server.duration",
    metric.WithUnit("ms"),
)
latency.Record(ctx, durationMs)
```

Runtime and process metrics (goroutines, GC, memory) are exported automatically
every 60 seconds — no extra code required.

## Sampling

```go
// Keep 10% of traces. Error spans are always exported regardless.
sdk, err := tracelit.New(
    tracelit.WithAPIKey(os.Getenv("TRACELIT_API_KEY")),
    tracelit.WithServiceName("high-traffic-api"),
    tracelit.WithSampleRate(0.1),
)
```

## Disabling telemetry

```go
// Disable in test environments without removing the SDK.
sdk, err := tracelit.New(
    tracelit.WithAPIKey(os.Getenv("TRACELIT_API_KEY")),
    tracelit.WithServiceName("my-service"),
    tracelit.WithEnabled(false),
)
// Or: TRACELIT_ENABLED=false
```

## Complete example

```go
package main

import (
    "context"
    "log/slog"
    "net/http"
    "os"
    "os/signal"
    "syscall"

    "github.com/tracelit-ai/tracelit-go"
    "github.com/tracelit-ai/tracelit-go/bridge"
    "github.com/tracelit-ai/tracelit-go/middleware"
)

func main() {
    sdk, err := tracelit.New(
        tracelit.WithAPIKey(os.Getenv("TRACELIT_API_KEY")),
        tracelit.WithServiceName("my-api"),
        tracelit.WithEnvironment("production"),
        tracelit.WithSampleRate(0.2),
    )
    if err != nil {
        slog.Error("tracelit init failed", "error", err)
        os.Exit(1)
    }
    defer sdk.Shutdown(context.Background())

    // Route all slog output through Tracelit.
    slog.SetDefault(slog.New(bridge.NewSlogHandler()))

    mux := http.NewServeMux()
    mux.HandleFunc("POST /orders", func(w http.ResponseWriter, r *http.Request) {
        ctx, span := tracelit.StartServerSpan(r.Context(), "create-order")
        defer span.End()

        slog.InfoContext(ctx, "creating order")

        if err := processOrder(ctx); err != nil {
            span.RecordError(err)
            http.Error(w, "internal error", http.StatusInternalServerError)
            return
        }
        w.WriteHeader(http.StatusCreated)
    })

    srv := &http.Server{
        Addr:    ":8080",
        Handler: middleware.NewHTTPHandler(mux),
    }

    quit := make(chan os.Signal, 1)
    signal.Notify(quit, syscall.SIGTERM, syscall.SIGINT)
    go func() { _ = srv.ListenAndServe() }()
    <-quit
    _ = srv.Shutdown(context.Background())
}
```

## Changelog

See the [release history](https://docs.tracelit.io/changelog) on the Tracelit docs.

## License

MIT — see [LICENSE](./LICENSE).
