// Package tracelit provides a Go SDK for Tracelit observability and monitoring.
// It is a thin wrapper over the OpenTelemetry Go SDK that configures OTLP/HTTP
// exporters for traces, metrics, and logs, and sends them to the Tracelit ingest
// endpoint with the appropriate authentication headers.
//
// Quick start:
//
//	sdk, err := tracelit.New(
//	    tracelit.WithAPIKey("tl_live_abc123"),
//	    tracelit.WithServiceName("payments-api"),
//	)
//	if err != nil {
//	    log.Fatal(err)
//	}
//	defer sdk.Shutdown(context.Background())
//
// Or using the global API (mirrors the Ruby SDK):
//
//	if err := tracelit.Configure(
//	    tracelit.WithAPIKey("tl_live_abc123"),
//	    tracelit.WithServiceName("payments-api"),
//	); err != nil {
//	    log.Fatal(err)
//	}
//	defer tracelit.Shutdown(context.Background())
package tracelit

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"net/http"
	"sync"
	"time"

	"go.opentelemetry.io/otel"
	otellog "go.opentelemetry.io/otel/log"
	"go.opentelemetry.io/otel/log/global"
	"go.opentelemetry.io/otel/metric"
	"go.opentelemetry.io/otel/sdk/log"
	sdkmetric "go.opentelemetry.io/otel/sdk/metric"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
	"go.opentelemetry.io/otel/trace"
)

// Version is the current Tracelit Go SDK version.
const Version = "0.1.1"

// SDK is the central object for the Tracelit Go SDK. It holds all configured
// OTel providers and must be shut down gracefully on application exit.
//
// Create one with New() or use the package-level Configure/Shutdown helpers
// for a global singleton pattern.
type SDK struct {
	config         Config
	tracerProvider *sdktrace.TracerProvider
	meterProvider  *sdkmetric.MeterProvider
	loggerProvider *log.LoggerProvider
	traceExporter  sdktrace.SpanExporter
	stopPollers    context.CancelFunc
}

// New creates and initialises a new Tracelit SDK instance.
// It validates configuration, sets up all three OTel signal pipelines
// (traces, metrics, logs), starts background metric pollers, and registers
// the providers as the global OTel providers.
//
// Call Shutdown on the returned SDK before your process exits to flush
// all pending telemetry.
func New(opts ...Option) (*SDK, error) {
	cfg := newConfig(opts)

	if !cfg.Enabled {
		return &SDK{config: cfg, stopPollers: func() {}}, nil
	}

	if err := cfg.validate(); err != nil {
		return nil, err
	}

	if err := validateAPIKey(cfg); err != nil {
		return nil, err
	}

	ctx := context.Background()

	tp, exp, err := setupTraces(ctx, cfg)
	if err != nil {
		return nil, err
	}

	mp, err := setupMetrics(ctx, cfg)
	if err != nil {
		return nil, fmt.Errorf("tracelit: metrics setup: %w", err)
	}

	lp, err := setupLogs(ctx, cfg)
	if err != nil {
		return nil, fmt.Errorf("tracelit: log setup: %w", err)
	}

	pollCtx, cancel := context.WithCancel(context.Background())

	sdk := &SDK{
		config:         cfg,
		tracerProvider: tp,
		meterProvider:  mp,
		loggerProvider: lp,
		traceExporter:  exp,
		stopPollers:    cancel,
	}

	startRuntimePollers(pollCtx, mp)

	return sdk, nil
}

// Config returns a copy of the configuration that was used to initialise the SDK.
// Useful for inspecting resolved values (environment defaults, option overrides).
func (s *SDK) Config() Config {
	return s.config
}

// Shutdown gracefully flushes all pending telemetry and shuts down every OTel
// provider. Pass a context with a deadline to bound the shutdown duration.
// Errors from each provider are joined and returned.
func (s *SDK) Shutdown(ctx context.Context) error {
	s.stopPollers()

	var errs []error
	if s.tracerProvider != nil {
		errs = append(errs, s.tracerProvider.Shutdown(ctx))
	}
	if s.meterProvider != nil {
		errs = append(errs, s.meterProvider.Shutdown(ctx))
	}
	if s.loggerProvider != nil {
		errs = append(errs, s.loggerProvider.Shutdown(ctx))
	}
	return errors.Join(errs...)
}

// Tracer returns an OpenTelemetry Tracer scoped to the given instrumentation
// scope name (typically the package or component name).
func (s *SDK) Tracer(name string, opts ...trace.TracerOption) trace.Tracer {
	return otel.GetTracerProvider().Tracer(name, opts...)
}

// Meter returns an OpenTelemetry Meter scoped to the given instrumentation scope.
func (s *SDK) Meter(name string, opts ...metric.MeterOption) metric.Meter {
	return otel.GetMeterProvider().Meter(name, opts...)
}

// Logger returns an OpenTelemetry Logger scoped to the given instrumentation scope.
func (s *SDK) Logger(name string, opts ...otellog.LoggerOption) otellog.Logger {
	return global.GetLoggerProvider().Logger(name, opts...)
}

// ─────────────────────────────────────────────────────────────────────────────
// Startup validation
// ─────────────────────────────────────────────────────────────────────────────

// validateAPIKey sends an empty POST /v1/logs to the ingest endpoint as a
// lightweight credential check before setting up all three OTel pipelines.
//
//   - 204 No Content → key is valid, proceed.
//   - 401 Unauthorized → key is invalid / workspace disabled; return an error
//     so the operator is immediately informed instead of silently dropping data.
//   - Any other error (5xx, network, timeout) → proceed anyway; the backend
//     may be temporarily unreachable and the exporters will retry on their own.
func validateAPIKey(cfg Config) error {
	client := &http.Client{Timeout: 10 * time.Second}
	req, err := http.NewRequest(http.MethodPost, cfg.Endpoint+"/v1/logs", bytes.NewReader(nil))
	if err != nil {
		return nil // malformed endpoint will surface when the exporter first flushes
	}
	req.Header.Set("Authorization", "Bearer "+cfg.APIKey)
	req.Header.Set("X-Service-Name", cfg.ServiceName)
	req.Header.Set("X-Environment", cfg.Environment)
	req.Header.Set("Content-Type", "application/x-protobuf")

	resp, err := client.Do(req)
	if err != nil {
		return nil // network unavailable — proceed and let exporters retry
	}
	defer resp.Body.Close()

	if resp.StatusCode == http.StatusUnauthorized {
		return fmt.Errorf(
			"tracelit: API key rejected (401 Unauthorized) — verify TRACELIT_API_KEY and that the workspace has observability enabled",
		)
	}
	return nil
}

// ─────────────────────────────────────────────────────────────────────────────
// Global singleton API
// ─────────────────────────────────────────────────────────────────────────────

var (
	globalMu  sync.Mutex
	globalSDK *SDK
)

// Configure initialises the global Tracelit SDK. It is equivalent to calling
// New and storing the result as the global SDK. This mirrors the Ruby SDK's
// Tracelit.configure + Tracelit.start! pattern.
//
// Call Shutdown (package-level) before your process exits.
func Configure(opts ...Option) error {
	sdk, err := New(opts...)
	if err != nil {
		return err
	}
	globalMu.Lock()
	globalSDK = sdk
	globalMu.Unlock()
	return nil
}

// Shutdown flushes and shuts down the global SDK.
func Shutdown(ctx context.Context) error {
	globalMu.Lock()
	sdk := globalSDK
	globalMu.Unlock()
	if sdk == nil {
		return nil
	}
	return sdk.Shutdown(ctx)
}

// Tracer returns an OpenTelemetry Tracer from the global provider.
func Tracer(name string, opts ...trace.TracerOption) trace.Tracer {
	return otel.GetTracerProvider().Tracer(name, opts...)
}

// Meter returns an OpenTelemetry Meter from the global provider.
func Meter(name string, opts ...metric.MeterOption) metric.Meter {
	return otel.GetMeterProvider().Meter(name, opts...)
}

// Logger returns an OpenTelemetry Logger from the global provider.
func Logger(name string, opts ...otellog.LoggerOption) otellog.Logger {
	return global.GetLoggerProvider().Logger(name, opts...)
}
