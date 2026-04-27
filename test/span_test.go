// Package tracelit_test contains black-box tests for span helpers in the tracelit SDK.
package tracelit_test

import (
	"context"
	"errors"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/codes"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
	"go.opentelemetry.io/otel/sdk/trace/tracetest"
	"go.opentelemetry.io/otel/trace"

	tracelit "github.com/tracelit-ai/tracelit-go"
)

// newTestTP wires an in-memory TracerProvider and registers it globally.
// The previous global provider is restored in t.Cleanup.
func newTestTP(t *testing.T) *tracetest.InMemoryExporter {
	t.Helper()
	exp := tracetest.NewInMemoryExporter()
	tp := sdktrace.NewTracerProvider(
		sdktrace.WithSampler(sdktrace.AlwaysSample()),
		sdktrace.WithSpanProcessor(sdktrace.NewSimpleSpanProcessor(exp)),
	)
	prev := otel.GetTracerProvider()
	otel.SetTracerProvider(tp)
	t.Cleanup(func() {
		_ = tp.Shutdown(context.Background())
		otel.SetTracerProvider(prev)
		exp.Reset()
	})
	return exp
}

// kvs converts []attribute.KeyValue to map[string]string for assertions.
func kvs(attrs []attribute.KeyValue) map[string]string {
	m := make(map[string]string, len(attrs))
	for _, kv := range attrs {
		m[string(kv.Key)] = kv.Value.Emit()
	}
	return m
}

// ─────────────────────────────────────────────────────────────────────────────
// StartSpan + span kinds
// ─────────────────────────────────────────────────────────────────────────────

func TestStartSpan_CreatesValidSpan(t *testing.T) {
	exp := newTestTP(t)
	ctx, span := tracelit.StartSpan(context.Background(), "op")
	defer span.End()

	assert.True(t, span.SpanContext().IsValid())
	assert.Equal(t, span.SpanContext().SpanID(), trace.SpanFromContext(ctx).SpanContext().SpanID())

	span.End()
	require.Len(t, exp.GetSpans(), 1)
	assert.Equal(t, "op", exp.GetSpans()[0].Name)
}

func TestStartSpan_DefaultKind_Internal(t *testing.T) {
	exp := newTestTP(t)
	_, span := tracelit.StartSpan(context.Background(), "op")
	span.End()
	assert.Equal(t, trace.SpanKindInternal, exp.GetSpans()[0].SpanKind)
}

func TestStartServerSpan_KindServer(t *testing.T) {
	exp := newTestTP(t)
	_, span := tracelit.StartServerSpan(context.Background(), "handler")
	span.End()
	assert.Equal(t, trace.SpanKindServer, exp.GetSpans()[0].SpanKind)
}

func TestStartClientSpan_KindClient(t *testing.T) {
	exp := newTestTP(t)
	_, span := tracelit.StartClientSpan(context.Background(), "rpc")
	span.End()
	assert.Equal(t, trace.SpanKindClient, exp.GetSpans()[0].SpanKind)
}

func TestStartProducerSpan_KindProducer(t *testing.T) {
	exp := newTestTP(t)
	_, span := tracelit.StartProducerSpan(context.Background(), "publish")
	span.End()
	assert.Equal(t, trace.SpanKindProducer, exp.GetSpans()[0].SpanKind)
}

func TestStartConsumerSpan_KindConsumer(t *testing.T) {
	exp := newTestTP(t)
	_, span := tracelit.StartConsumerSpan(context.Background(), "consume")
	span.End()
	assert.Equal(t, trace.SpanKindConsumer, exp.GetSpans()[0].SpanKind)
}

// ─────────────────────────────────────────────────────────────────────────────
// Span options
// ─────────────────────────────────────────────────────────────────────────────

func TestWithSpanAttributes_SetAtStartTime(t *testing.T) {
	exp := newTestTP(t)
	_, span := tracelit.StartSpan(context.Background(), "op",
		tracelit.WithSpanAttributes(map[string]any{"order.id": "123"}),
	)
	span.End()
	attrs := kvs(exp.GetSpans()[0].Attributes)
	assert.Equal(t, "123", attrs["order.id"])
}

func TestWithTracerName_SetsScope(t *testing.T) {
	exp := newTestTP(t)
	_, span := tracelit.StartSpan(context.Background(), "op", tracelit.WithTracerName("my.scope"))
	span.End()
	assert.Equal(t, "my.scope", exp.GetSpans()[0].InstrumentationScope.Name)
}

func TestWithSpanLinks_AttachesLink(t *testing.T) {
	exp := newTestTP(t)
	_, src := tracelit.StartSpan(context.Background(), "src")
	lnk := trace.Link{SpanContext: src.SpanContext()}
	src.End()

	_, span := tracelit.StartSpan(context.Background(), "fan-in", tracelit.WithSpanLinks(lnk))
	span.End()

	var fanIn tracetest.SpanStub
	for _, s := range exp.GetSpans() {
		if s.Name == "fan-in" {
			fanIn = s
		}
	}
	require.Len(t, fanIn.Links, 1)
	assert.Equal(t, lnk.SpanContext.SpanID(), fanIn.Links[0].SpanContext.SpanID())
}

// ─────────────────────────────────────────────────────────────────────────────
// Parent–child tracing
// ─────────────────────────────────────────────────────────────────────────────

func TestStartSpan_ParentChild_ShareTraceID(t *testing.T) {
	exp := newTestTP(t)
	ctx, parent := tracelit.StartSpan(context.Background(), "parent")
	_, child := tracelit.StartSpan(ctx, "child")
	child.End()
	parent.End()

	stubs := exp.GetSpans()
	require.Len(t, stubs, 2)

	var parentStub, childStub tracetest.SpanStub
	for _, s := range stubs {
		switch s.Name {
		case "parent":
			parentStub = s
		case "child":
			childStub = s
		}
	}
	assert.Equal(t, parentStub.SpanContext.TraceID(), childStub.SpanContext.TraceID())
	assert.Equal(t, parentStub.SpanContext.SpanID(), childStub.Parent.SpanID())
}

// ─────────────────────────────────────────────────────────────────────────────
// SetAttribute / SetAttributes
// ─────────────────────────────────────────────────────────────────────────────

func TestSetAttribute_String(t *testing.T) {
	exp := newTestTP(t)
	_, span := tracelit.StartSpan(context.Background(), "op")
	span.SetAttribute("env", "prod")
	span.End()
	assert.Equal(t, "prod", kvs(exp.GetSpans()[0].Attributes)["env"])
}

func TestSetAttributes_MultipleTypes(t *testing.T) {
	exp := newTestTP(t)
	_, span := tracelit.StartSpan(context.Background(), "op")
	span.SetAttributes(map[string]any{
		"str":   "hello",
		"num":   int64(42),
		"flag":  true,
		"ratio": 0.95,
	})
	span.End()
	attrs := kvs(exp.GetSpans()[0].Attributes)
	assert.Equal(t, "hello", attrs["str"])
	assert.Equal(t, "42", attrs["num"])
	assert.Equal(t, "true", attrs["flag"])
}

// ─────────────────────────────────────────────────────────────────────────────
// AddEvent
// ─────────────────────────────────────────────────────────────────────────────

func TestAddEvent_NoAttrs(t *testing.T) {
	exp := newTestTP(t)
	_, span := tracelit.StartSpan(context.Background(), "op")
	span.AddEvent("payment-captured")
	span.End()
	events := exp.GetSpans()[0].Events
	require.Len(t, events, 1)
	assert.Equal(t, "payment-captured", events[0].Name)
}

func TestAddEvent_WithAttrs(t *testing.T) {
	exp := newTestTP(t)
	_, span := tracelit.StartSpan(context.Background(), "op")
	span.AddEvent("order-placed", map[string]any{"amount": 500})
	span.End()
	events := exp.GetSpans()[0].Events
	require.Len(t, events, 1)
	_, ok := kvs(events[0].Attributes)["amount"]
	assert.True(t, ok)
}

// ─────────────────────────────────────────────────────────────────────────────
// RecordError
// ─────────────────────────────────────────────────────────────────────────────

func TestRecordError_SetsErrorStatus(t *testing.T) {
	exp := newTestTP(t)
	_, span := tracelit.StartSpan(context.Background(), "op")
	span.RecordError(errors.New("db down"))
	span.End()

	stub := exp.GetSpans()[0]
	assert.Equal(t, codes.Error, stub.Status.Code)
	assert.Equal(t, "db down", stub.Status.Description)
}

func TestRecordError_AddsExceptionEventWithStack(t *testing.T) {
	exp := newTestTP(t)
	_, span := tracelit.StartSpan(context.Background(), "op")
	span.RecordError(errors.New("timeout"))
	span.End()

	events := exp.GetSpans()[0].Events
	require.Len(t, events, 1)
	assert.Equal(t, "exception", events[0].Name)

	eAttrs := kvs(events[0].Attributes)
	assert.Equal(t, "timeout", eAttrs["exception.message"])
	assert.NotEmpty(t, eAttrs["exception.type"])
	assert.Contains(t, eAttrs["exception.stacktrace"], "goroutine")
}

func TestRecordError_StackTrace_ContainsCallerFile(t *testing.T) {
	exp := newTestTP(t)
	_, span := tracelit.StartSpan(context.Background(), "op")
	span.RecordError(errors.New("err"))
	span.End()
	stack := kvs(exp.GetSpans()[0].Events[0].Attributes)["exception.stacktrace"]
	assert.Contains(t, stack, "span_test.go")
}

func TestRecordError_NilError_IsNoop(t *testing.T) {
	exp := newTestTP(t)
	_, span := tracelit.StartSpan(context.Background(), "op")
	span.RecordError(nil)
	span.End()

	stub := exp.GetSpans()[0]
	assert.NotEqual(t, codes.Error, stub.Status.Code)
	assert.Empty(t, stub.Events)
}

func TestRecordError_WithExtraAttributes(t *testing.T) {
	exp := newTestTP(t)
	_, span := tracelit.StartSpan(context.Background(), "op")
	span.RecordError(errors.New("fail"),
		tracelit.WithErrorAttributes(map[string]any{"gateway": "stripe"}),
	)
	span.End()
	assert.Equal(t, "stripe", kvs(exp.GetSpans()[0].Attributes)["gateway"])
}

// ─────────────────────────────────────────────────────────────────────────────
// RecordPanic
// ─────────────────────────────────────────────────────────────────────────────

func TestRecordPanic_Repanics_ByDefault(t *testing.T) {
	newTestTP(t)
	_, span := tracelit.StartSpan(context.Background(), "op")
	assert.Panics(t, func() {
		defer func() {
			if r := recover(); r != nil {
				span.RecordPanic(r)
			}
		}()
		panic("fire")
	})
	span.End()
}

func TestRecordPanic_WithSwallowPanic_NoRepanic(t *testing.T) {
	exp := newTestTP(t)
	_, span := tracelit.StartSpan(context.Background(), "op")
	assert.NotPanics(t, func() {
		defer func() {
			if r := recover(); r != nil {
				span.RecordPanic(r, tracelit.WithSwallowPanic())
			}
		}()
		panic("controlled")
	})
	span.End()

	stub := exp.GetSpans()[0]
	assert.Equal(t, codes.Error, stub.Status.Code)
	eAttrs := kvs(stub.Events[0].Attributes)
	assert.Equal(t, "controlled", eAttrs["exception.message"])
	assert.NotEmpty(t, eAttrs["exception.stacktrace"])
}

// ─────────────────────────────────────────────────────────────────────────────
// Context helpers: TraceID / SpanID / SpanFromContext / ContextWithSpan
// ─────────────────────────────────────────────────────────────────────────────

func TestTraceID_Valid(t *testing.T) {
	newTestTP(t)
	ctx, span := tracelit.StartSpan(context.Background(), "op")
	defer span.End()
	id := tracelit.TraceID(ctx)
	assert.Len(t, id, 32)
	assert.NotEqual(t, "00000000000000000000000000000000", id)
}

func TestSpanID_Valid(t *testing.T) {
	newTestTP(t)
	ctx, span := tracelit.StartSpan(context.Background(), "op")
	defer span.End()
	id := tracelit.SpanID(ctx)
	assert.Len(t, id, 16)
	assert.NotEqual(t, "0000000000000000", id)
}

func TestTraceID_NoActiveSpan_ReturnsEmpty(t *testing.T) {
	assert.Empty(t, tracelit.TraceID(context.Background()))
}

func TestSpanID_NoActiveSpan_ReturnsEmpty(t *testing.T) {
	assert.Empty(t, tracelit.SpanID(context.Background()))
}

func TestSpanFromContext_ReturnsActiveSpan(t *testing.T) {
	newTestTP(t)
	ctx, started := tracelit.StartSpan(context.Background(), "op")
	defer started.End()
	got := tracelit.SpanFromContext(ctx)
	assert.Equal(t, started.SpanContext().SpanID(), got.SpanContext().SpanID())
}

func TestContextWithSpan_InjectsSpan(t *testing.T) {
	newTestTP(t)
	_, span := tracelit.StartSpan(context.Background(), "op")
	defer span.End()
	ctx := tracelit.ContextWithSpan(context.Background(), span)
	extracted := trace.SpanFromContext(ctx)
	assert.Equal(t, span.SpanContext().SpanID(), extracted.SpanContext().SpanID())
}
