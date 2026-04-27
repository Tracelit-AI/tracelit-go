package processor_test

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.opentelemetry.io/otel/codes"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
	"go.opentelemetry.io/otel/sdk/trace/tracetest"

	"github.com/tracelit/tracelit-go/internal/processor"
	"github.com/tracelit/tracelit-go/internal/sampler"
)

func TestNewErrorSpan_NonErrorSpan_NotExported(t *testing.T) {
	exp := tracetest.NewInMemoryExporter()
	tp := sdktrace.NewTracerProvider(
		sdktrace.WithSampler(sdktrace.AlwaysSample()),
		sdktrace.WithSpanProcessor(processor.NewErrorSpan(exp)),
	)
	defer tp.Shutdown(context.Background())

	_, span := tp.Tracer("test").Start(context.Background(), "ok")
	span.End()

	assert.Empty(t, exp.GetSpans(), "non-error span must not be exported by ErrorSpanProcessor")
}

func TestNewErrorSpan_SampledErrorSpan_NotExported(t *testing.T) {
	exp := tracetest.NewInMemoryExporter()
	tp := sdktrace.NewTracerProvider(
		sdktrace.WithSampler(sdktrace.AlwaysSample()),
		sdktrace.WithSpanProcessor(processor.NewErrorSpan(exp)),
	)
	defer tp.Shutdown(context.Background())

	_, span := tp.Tracer("test").Start(context.Background(), "err-sampled")
	span.SetStatus(codes.Error, "oops")
	span.End()

	assert.Empty(t, exp.GetSpans(),
		"sampled error span must NOT be re-exported by ErrorSpanProcessor (BatchSpanProcessor handles it)")
}

func TestNewErrorSpan_UnsampledErrorSpan_IsExported(t *testing.T) {
	exp := tracetest.NewInMemoryExporter()
	tp := sdktrace.NewTracerProvider(
		sdktrace.WithSampler(sampler.NewErrorAlways(sdktrace.TraceIDRatioBased(0))),
		sdktrace.WithSpanProcessor(processor.NewErrorSpan(exp)),
	)
	defer tp.Shutdown(context.Background())

	_, span := tp.Tracer("test").Start(context.Background(), "err-unsampled")
	span.SetStatus(codes.Error, "unsampled error")
	span.End()

	stubs := exp.GetSpans()
	require.Len(t, stubs, 1, "unsampled error span must be exported by ErrorSpanProcessor")
	assert.Equal(t, "err-unsampled", stubs[0].Name)
	assert.Equal(t, codes.Error, stubs[0].Status.Code)
}

func TestNewErrorSpan_UnsampledNonError_NotExported(t *testing.T) {
	exp := tracetest.NewInMemoryExporter()
	tp := sdktrace.NewTracerProvider(
		sdktrace.WithSampler(sampler.NewErrorAlways(sdktrace.TraceIDRatioBased(0))),
		sdktrace.WithSpanProcessor(processor.NewErrorSpan(exp)),
	)
	defer tp.Shutdown(context.Background())

	_, span := tp.Tracer("test").Start(context.Background(), "ok-unsampled")
	span.End()

	assert.Empty(t, exp.GetSpans(), "unsampled non-error span must not be exported")
}

func TestNewErrorSpan_Shutdown_NoError(t *testing.T) {
	assert.NoError(t, processor.NewErrorSpan(tracetest.NewInMemoryExporter()).Shutdown(context.Background()))
}

func TestNewErrorSpan_ForceFlush_NoError(t *testing.T) {
	assert.NoError(t, processor.NewErrorSpan(tracetest.NewInMemoryExporter()).ForceFlush(context.Background()))
}

// ─────────────────────────────────────────────────────────────────────────────
// Full pipeline: ErrorAlwaysSampler + ErrorSpanProcessor wired together
// ─────────────────────────────────────────────────────────────────────────────

func TestPipeline_UnsampledError_ExportedOnceByErrorProcessor(t *testing.T) {
	batchExp := tracetest.NewInMemoryExporter()
	errorExp := tracetest.NewInMemoryExporter()

	tp := sdktrace.NewTracerProvider(
		sdktrace.WithSampler(sampler.NewErrorAlways(sdktrace.TraceIDRatioBased(0))),
		sdktrace.WithSpanProcessor(sdktrace.NewSimpleSpanProcessor(batchExp)),
		sdktrace.WithSpanProcessor(processor.NewErrorSpan(errorExp)),
	)
	defer tp.Shutdown(context.Background())

	_, span := tp.Tracer("test").Start(context.Background(), "pipeline-err")
	span.SetStatus(codes.Error, "fail")
	span.End()

	assert.Empty(t, batchExp.GetSpans(), "BatchSpanProcessor must not export RECORD_ONLY spans")
	require.Len(t, errorExp.GetSpans(), 1, "ErrorSpanProcessor must export unsampled error exactly once")
}

func TestPipeline_SampledError_OnlyInBatch(t *testing.T) {
	batchExp := tracetest.NewInMemoryExporter()
	errorExp := tracetest.NewInMemoryExporter()

	tp := sdktrace.NewTracerProvider(
		sdktrace.WithSampler(sdktrace.AlwaysSample()),
		sdktrace.WithSpanProcessor(sdktrace.NewSimpleSpanProcessor(batchExp)),
		sdktrace.WithSpanProcessor(processor.NewErrorSpan(errorExp)),
	)
	defer tp.Shutdown(context.Background())

	_, span := tp.Tracer("test").Start(context.Background(), "sampled-err")
	span.SetStatus(codes.Error, "fail")
	span.End()

	require.Len(t, batchExp.GetSpans(), 1, "BatchSpanProcessor must export sampled error spans")
	assert.Empty(t, errorExp.GetSpans(), "ErrorSpanProcessor must skip sampled error spans")
}
