package tracelit

import (
	"context"
	"fmt"
	"reflect"
	"runtime"

	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/codes"
	"go.opentelemetry.io/otel/trace"
)

// ─────────────────────────────────────────────────────────────────────────────
// Span wrapper
// ─────────────────────────────────────────────────────────────────────────────

// Span wraps the OTel trace.Span with idiomatic helpers for deep tracing,
// error recording, and stack trace capture.
type Span struct {
	trace.Span
}

// End finishes the span. It is safe to call multiple times (idempotent via OTel).
// Typical usage:
//
//	ctx, span := tracelit.StartSpan(ctx, "operation")
//	defer span.End()
func (s *Span) End() {
	s.Span.End()
}

// SetAttribute sets a single attribute on the span. value is converted to an
// OTel attribute using the same type mapping as SetAttributes.
func (s *Span) SetAttribute(key string, value any) {
	s.Span.SetAttributes(toAttribute(key, value))
}

// SetAttributes sets multiple attributes from a map. Keys are attribute names;
// values are converted to OTel types (string, bool, int64, float64, or their
// slice equivalents). Any unrecognised type is formatted with fmt.Sprintf.
func (s *Span) SetAttributes(attrs map[string]any) {
	kvs := make([]attribute.KeyValue, 0, len(attrs))
	for k, v := range attrs {
		kvs = append(kvs, toAttribute(k, v))
	}
	s.Span.SetAttributes(kvs...)
}

// AddEvent records a named event on the span's timeline.
// An optional single map[string]any may be passed as the event's attributes.
func (s *Span) AddEvent(name string, attrs ...map[string]any) {
	if len(attrs) == 0 {
		s.Span.AddEvent(name)
		return
	}
	kvs := make([]attribute.KeyValue, 0, len(attrs[0]))
	for k, v := range attrs[0] {
		kvs = append(kvs, toAttribute(k, v))
	}
	s.Span.AddEvent(name, trace.WithAttributes(kvs...))
}

// RecordError marks the span as errored (status = Error), records the error as
// a span event following the OTel exception semantic conventions, and attaches
// a full Go stack trace as the "exception.stacktrace" attribute.
//
// If err is nil, this is a no-op.
func (s *Span) RecordError(err error, opts ...RecordErrorOption) {
	if err == nil {
		return
	}

	cfg := defaultRecordErrorConfig()
	for _, o := range opts {
		o(&cfg)
	}

	stack := captureStack(cfg.stackDepth)

	s.Span.SetStatus(codes.Error, err.Error())
	s.Span.AddEvent("exception", trace.WithAttributes(
		attribute.String("exception.type", errorTypeName(err)),
		attribute.String("exception.message", err.Error()),
		attribute.String("exception.stacktrace", stack),
	))

	if cfg.extraAttrs != nil {
		kvs := make([]attribute.KeyValue, 0, len(cfg.extraAttrs))
		for k, v := range cfg.extraAttrs {
			kvs = append(kvs, toAttribute(k, v))
		}
		s.Span.SetAttributes(kvs...)
	}
}

// RecordPanic is designed to be called inside a deferred recover() block.
// It converts the recovered panic value to an error representation, records the
// full stack trace, marks the span as errored, and then either re-panics or
// swallows the panic depending on the WithSwallowPanic option.
//
// Example:
//
//	ctx, span := tracelit.StartSpan(ctx, "risky-op")
//	defer func() {
//	    if r := recover(); r != nil {
//	        span.RecordPanic(r)  // re-panics by default
//	    }
//	    span.End()
//	}()
func (s *Span) RecordPanic(recovered any, opts ...RecordErrorOption) {
	cfg := defaultRecordErrorConfig()
	for _, o := range opts {
		o(&cfg)
	}

	stack := captureStack(cfg.stackDepth)
	msg := fmt.Sprintf("%+v", recovered)

	s.Span.SetStatus(codes.Error, "panic: "+msg)
	s.Span.AddEvent("exception", trace.WithAttributes(
		attribute.String("exception.type", "panic"),
		attribute.String("exception.message", msg),
		attribute.String("exception.stacktrace", stack),
	))

	if !cfg.swallowPanic {
		panic(recovered)
	}
}

// ─────────────────────────────────────────────────────────────────────────────
// RecordError options
// ─────────────────────────────────────────────────────────────────────────────

type recordErrorConfig struct {
	stackDepth   int
	swallowPanic bool
	extraAttrs   map[string]any
}

func defaultRecordErrorConfig() recordErrorConfig {
	return recordErrorConfig{stackDepth: 4096}
}

// RecordErrorOption configures the behaviour of RecordError / RecordPanic.
type RecordErrorOption func(*recordErrorConfig)

// WithStackDepth sets the maximum byte length of the captured stack trace.
// Default is 4096 bytes which is sufficient for most call stacks.
func WithStackDepth(depth int) RecordErrorOption {
	return func(c *recordErrorConfig) { c.stackDepth = depth }
}

// WithSwallowPanic causes RecordPanic to absorb the panic instead of re-panicking.
func WithSwallowPanic() RecordErrorOption {
	return func(c *recordErrorConfig) { c.swallowPanic = true }
}

// WithErrorAttributes attaches extra span attributes when recording an error.
func WithErrorAttributes(attrs map[string]any) RecordErrorOption {
	return func(c *recordErrorConfig) { c.extraAttrs = attrs }
}

// ─────────────────────────────────────────────────────────────────────────────
// Span start helpers
// ─────────────────────────────────────────────────────────────────────────────

// SpanOption configures a span at start time.
type SpanOption func(*spanConfig)

type spanConfig struct {
	kind     trace.SpanKind
	attrs    map[string]any
	links    []trace.Link
	tracer   string
}

func defaultSpanConfig() spanConfig {
	return spanConfig{kind: trace.SpanKindInternal}
}

// WithSpanKind sets the span kind (Internal, Server, Client, Producer, Consumer).
func WithSpanKind(kind trace.SpanKind) SpanOption {
	return func(c *spanConfig) { c.kind = kind }
}

// WithSpanAttributes sets attributes on the span at start time.
func WithSpanAttributes(attrs map[string]any) SpanOption {
	return func(c *spanConfig) { c.attrs = attrs }
}

// WithSpanLinks adds links to other spans (e.g. for fan-in operations).
func WithSpanLinks(links ...trace.Link) SpanOption {
	return func(c *spanConfig) { c.links = append(c.links, links...) }
}

// WithTracerName sets the instrumentation scope name used to retrieve the tracer.
// Defaults to "tracelit".
func WithTracerName(name string) SpanOption {
	return func(c *spanConfig) { c.tracer = name }
}

// StartSpan starts a new child span derived from ctx. The span is automatically
// made a child of any span already in ctx. Returns the updated context
// (containing the new span) and a *Span wrapper.
//
// Always call span.End() when the operation is complete, typically via defer:
//
//	ctx, span := tracelit.StartSpan(ctx, "process-order")
//	defer span.End()
func StartSpan(ctx context.Context, name string, opts ...SpanOption) (context.Context, *Span) {
	cfg := defaultSpanConfig()
	for _, o := range opts {
		o(&cfg)
	}

	tracerName := cfg.tracer
	if tracerName == "" {
		tracerName = "tracelit"
	}

	startOpts := []trace.SpanStartOption{
		trace.WithSpanKind(cfg.kind),
	}
	if len(cfg.links) > 0 {
		startOpts = append(startOpts, trace.WithLinks(cfg.links...))
	}
	if len(cfg.attrs) > 0 {
		kvs := make([]attribute.KeyValue, 0, len(cfg.attrs))
		for k, v := range cfg.attrs {
			kvs = append(kvs, toAttribute(k, v))
		}
		startOpts = append(startOpts, trace.WithAttributes(kvs...))
	}

	tracer := otel.GetTracerProvider().Tracer(tracerName)
	ctx, rawSpan := tracer.Start(ctx, name, startOpts...)
	return ctx, &Span{Span: rawSpan}
}

// StartServerSpan is a convenience wrapper for StartSpan with SpanKindServer.
// Use for the entry point of inbound requests (HTTP handlers, gRPC server methods).
func StartServerSpan(ctx context.Context, name string, opts ...SpanOption) (context.Context, *Span) {
	return StartSpan(ctx, name, append(opts, WithSpanKind(trace.SpanKindServer))...)
}

// StartClientSpan is a convenience wrapper for StartSpan with SpanKindClient.
// Use for outbound calls (HTTP client requests, RPC calls, DB queries).
func StartClientSpan(ctx context.Context, name string, opts ...SpanOption) (context.Context, *Span) {
	return StartSpan(ctx, name, append(opts, WithSpanKind(trace.SpanKindClient))...)
}

// StartProducerSpan is a convenience wrapper for message-queue publish operations.
func StartProducerSpan(ctx context.Context, name string, opts ...SpanOption) (context.Context, *Span) {
	return StartSpan(ctx, name, append(opts, WithSpanKind(trace.SpanKindProducer))...)
}

// StartConsumerSpan is a convenience wrapper for message-queue consume operations.
func StartConsumerSpan(ctx context.Context, name string, opts ...SpanOption) (context.Context, *Span) {
	return StartSpan(ctx, name, append(opts, WithSpanKind(trace.SpanKindConsumer))...)
}

// ─────────────────────────────────────────────────────────────────────────────
// Context helpers
// ─────────────────────────────────────────────────────────────────────────────

// SpanFromContext extracts the active span from ctx and returns it as a
// *Span wrapper. If there is no active span a noop span is returned.
func SpanFromContext(ctx context.Context) *Span {
	return &Span{Span: trace.SpanFromContext(ctx)}
}

// ContextWithSpan returns a copy of ctx containing span. Use this to pass a
// span to a goroutine that needs to create child spans.
func ContextWithSpan(ctx context.Context, span *Span) context.Context {
	return trace.ContextWithSpan(ctx, span.Span)
}

// TraceID returns the hex-encoded trace ID of the active span in ctx.
// Returns an empty string when there is no active span.
func TraceID(ctx context.Context) string {
	sc := trace.SpanFromContext(ctx).SpanContext()
	if !sc.IsValid() {
		return ""
	}
	return sc.TraceID().String()
}

// SpanID returns the hex-encoded span ID of the active span in ctx.
// Returns an empty string when there is no active span.
func SpanID(ctx context.Context) string {
	sc := trace.SpanFromContext(ctx).SpanContext()
	if !sc.IsValid() {
		return ""
	}
	return sc.SpanID().String()
}

// ─────────────────────────────────────────────────────────────────────────────
// Internal helpers
// ─────────────────────────────────────────────────────────────────────────────

// captureStack returns a string containing the current goroutine's call stack.
// maxBytes controls the maximum buffer size; 4096 is enough for most stacks.
func captureStack(maxBytes int) string {
	buf := make([]byte, maxBytes)
	n := runtime.Stack(buf, false)
	return string(buf[:n])
}

// errorTypeName returns the fully-qualified type name of err, following the OTel
// exception.type semantic convention.
func errorTypeName(err error) string {
	t := reflect.TypeOf(err)
	if t == nil {
		return "error"
	}
	if t.Kind() == reflect.Ptr {
		t = t.Elem()
	}
	if pkg := t.PkgPath(); pkg != "" {
		return pkg + "." + t.Name()
	}
	return t.Name()
}

// toAttribute converts a key/value pair to an OTel attribute.KeyValue.
// Supported types: string, bool, int, int32, int64, float32, float64, and
// their slice variants. All other types fall back to fmt.Sprintf("%v", v).
func toAttribute(key string, value any) attribute.KeyValue {
	switch v := value.(type) {
	case string:
		return attribute.String(key, v)
	case bool:
		return attribute.Bool(key, v)
	case int:
		return attribute.Int(key, v)
	case int32:
		return attribute.Int64(key, int64(v))
	case int64:
		return attribute.Int64(key, v)
	case float32:
		return attribute.Float64(key, float64(v))
	case float64:
		return attribute.Float64(key, v)
	case []string:
		return attribute.StringSlice(key, v)
	case []bool:
		return attribute.BoolSlice(key, v)
	case []int:
		return attribute.IntSlice(key, v)
	case []int64:
		return attribute.Int64Slice(key, v)
	case []float64:
		return attribute.Float64Slice(key, v)
	default:
		return attribute.String(key, fmt.Sprintf("%v", v))
	}
}
