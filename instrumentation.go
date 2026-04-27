package tracelit

import (
	"context"
	"fmt"
	"time"

	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/exporters/otlp/otlplog/otlploghttp"
	"go.opentelemetry.io/otel/exporters/otlp/otlpmetric/otlpmetrichttp"
	"go.opentelemetry.io/otel/exporters/otlp/otlptrace/otlptracehttp"
	"go.opentelemetry.io/otel/log/global"
	"go.opentelemetry.io/otel/propagation"
	sdklog "go.opentelemetry.io/otel/sdk/log"
	sdkmetric "go.opentelemetry.io/otel/sdk/metric"
	"go.opentelemetry.io/otel/sdk/metric/metricdata"
	"go.opentelemetry.io/otel/sdk/resource"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
	semconv "go.opentelemetry.io/otel/semconv/v1.26.0"

	"github.com/tracelit-ai/tracelit-go/internal/logproc"
	"github.com/tracelit-ai/tracelit-go/internal/processor"
	"github.com/tracelit-ai/tracelit-go/internal/sampler"
)

// exporterRetry is the shared retry policy applied to every OTLP exporter.
// The backend never returns 429 and has no deduplication, so we only retry
// on 5xx / network errors (the OTel exporter already honours this).
// 3 attempts (2 retries): 0 s → 1 s → ~2 s, all within MaxElapsedTime.
var exporterRetry = struct {
	Enabled         bool
	InitialInterval time.Duration
	MaxInterval     time.Duration
	MaxElapsedTime  time.Duration
}{
	Enabled:         true,
	InitialInterval: 1 * time.Second,
	MaxInterval:     30 * time.Second,
	MaxElapsedTime:  90 * time.Second,
}

// setupTraces configures the OTel TracerProvider with OTLP/HTTP export and
// returns the provider along with the underlying exporter (shared with
// ErrorSpanProcessor so it can export unsampled error spans).
func setupTraces(ctx context.Context, cfg Config) (*sdktrace.TracerProvider, sdktrace.SpanExporter, error) {
	exp, err := otlptracehttp.New(ctx,
		otlptracehttp.WithEndpointURL(cfg.Endpoint+"/v1/traces"),
		otlptracehttp.WithHeaders(cfg.headers()),
		otlptracehttp.WithCompression(otlptracehttp.GzipCompression),
		otlptracehttp.WithRetry(otlptracehttp.RetryConfig{
			Enabled:         exporterRetry.Enabled,
			InitialInterval: exporterRetry.InitialInterval,
			MaxInterval:     exporterRetry.MaxInterval,
			MaxElapsedTime:  exporterRetry.MaxElapsedTime,
		}),
	)
	if err != nil {
		return nil, nil, fmt.Errorf("tracelit: creating trace exporter: %w", err)
	}

	res, err := buildResource(ctx, cfg)
	if err != nil {
		return nil, nil, err
	}

	tp := sdktrace.NewTracerProvider(
		sdktrace.WithResource(res),
		sdktrace.WithSampler(sampler.Choose(cfg.SampleRate)),
		sdktrace.WithBatcher(exp),
		sdktrace.WithSpanProcessor(processor.NewErrorSpan(exp)),
	)

	otel.SetTracerProvider(tp)
	otel.SetTextMapPropagator(propagation.NewCompositeTextMapPropagator(
		propagation.TraceContext{},
		propagation.Baggage{},
	))

	return tp, exp, nil
}

// setupMetrics configures the OTel MeterProvider with a 60-second periodic reader.
// Delta temporality is requested so the backend receives per-interval deltas and
// does not need to perform cumulative→delta conversion server-side.
func setupMetrics(ctx context.Context, cfg Config) (*sdkmetric.MeterProvider, error) {
	exp, err := otlpmetrichttp.New(ctx,
		otlpmetrichttp.WithEndpointURL(cfg.Endpoint+"/v1/metrics"),
		otlpmetrichttp.WithHeaders(cfg.headers()),
		otlpmetrichttp.WithCompression(otlpmetrichttp.GzipCompression),
		otlpmetrichttp.WithRetry(otlpmetrichttp.RetryConfig{
			Enabled:         exporterRetry.Enabled,
			InitialInterval: exporterRetry.InitialInterval,
			MaxInterval:     exporterRetry.MaxInterval,
			MaxElapsedTime:  exporterRetry.MaxElapsedTime,
		}),
		// Request delta temporality for all additive instruments (counters,
		// histograms).  Gauges report instantaneous values and are unaffected.
		otlpmetrichttp.WithTemporalitySelector(func(k sdkmetric.InstrumentKind) metricdata.Temporality {
			switch k {
			case sdkmetric.InstrumentKindObservableGauge,
				sdkmetric.InstrumentKindGauge:
				// Gauges represent a current reading — cumulative is the only
				// meaningful option; delta would double-count on the backend.
				return metricdata.CumulativeTemporality
			default:
				return metricdata.DeltaTemporality
			}
		}),
	)
	if err != nil {
		return nil, fmt.Errorf("tracelit: creating metric exporter: %w", err)
	}

	res, err := buildResource(ctx, cfg)
	if err != nil {
		return nil, err
	}

	mp := sdkmetric.NewMeterProvider(
		sdkmetric.WithResource(res),
		sdkmetric.WithReader(sdkmetric.NewPeriodicReader(exp,
			sdkmetric.WithInterval(metricsExportInterval),
		)),
	)

	otel.SetMeterProvider(mp)
	return mp, nil
}

// setupLogs configures the OTel LoggerProvider with OTLP/HTTP batch export.
func setupLogs(ctx context.Context, cfg Config) (*sdklog.LoggerProvider, error) {
	exp, err := otlploghttp.New(ctx,
		otlploghttp.WithEndpointURL(cfg.Endpoint+"/v1/logs"),
		otlploghttp.WithHeaders(cfg.headers()),
		otlploghttp.WithCompression(otlploghttp.GzipCompression),
		otlploghttp.WithRetry(otlploghttp.RetryConfig{
			Enabled:         exporterRetry.Enabled,
			InitialInterval: exporterRetry.InitialInterval,
			MaxInterval:     exporterRetry.MaxInterval,
			MaxElapsedTime:  exporterRetry.MaxElapsedTime,
		}),
	)
	if err != nil {
		return nil, fmt.Errorf("tracelit: creating log exporter: %w", err)
	}

	res, err := buildResource(ctx, cfg)
	if err != nil {
		return nil, err
	}

	lp := sdklog.NewLoggerProvider(
		sdklog.WithResource(res),
		// Wrap the batch processor with MinSevProcessor which:
		//  (a) implements FilterProcessor so Logger.Enabled() correctly
		//      honours the configured minimum severity, and
		//  (b) back-fills SeverityText on every record (the otelslog bridge
		//      leaves this field empty, breaking severity display in some UIs).
		sdklog.WithProcessor(logproc.New(sdklog.NewBatchProcessor(exp), cfg.MinLogLevel)),
	)

	global.SetLoggerProvider(lp)
	return lp, nil
}

// buildResource constructs the OTel resource attached to every signal.
func buildResource(ctx context.Context, cfg Config) (*resource.Resource, error) {
	attrs := []attribute.KeyValue{
		semconv.ServiceName(cfg.ServiceName),
		semconv.DeploymentEnvironment(cfg.Environment),
		attribute.String("telemetry.sdk.language", "go"),
		attribute.String("telemetry.sdk.name", "tracelit"),
		attribute.String("telemetry.sdk.version", Version),
	}

	for k, v := range cfg.ResourceAttributes {
		attrs = append(attrs, attribute.String(k, v))
	}

	res, err := resource.New(ctx,
		resource.WithAttributes(attrs...),
		resource.WithProcess(),
		resource.WithHost(),
	)
	if err != nil {
		return nil, fmt.Errorf("tracelit: building resource: %w", err)
	}
	return res, nil
}

