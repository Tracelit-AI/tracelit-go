// Package processor provides the custom OTel SpanProcessor used by the Tracelit SDK.
// It is an internal implementation detail and must not be imported by consumers.
package processor

import (
	"context"

	"go.opentelemetry.io/otel/codes"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
)

// errorSpanProcessor exports error spans that were not sampled by the head
// sampler (i.e. their TraceFlags do not have the Sampled bit set). This
// mirrors the Ruby SDK's ErrorSpanProcessor and ensures that errors are
// always visible in Tracelit regardless of the configured sample rate.
//
// Sampled error spans are handled by the BatchSpanProcessor; this processor
// skips them to avoid double-export.
type errorSpanProcessor struct {
	exporter sdktrace.SpanExporter
}

// NewErrorSpan returns a SpanProcessor that synchronously exports any span
// whose status is Error and whose trace was not head-sampled.
func NewErrorSpan(exp sdktrace.SpanExporter) sdktrace.SpanProcessor {
	return &errorSpanProcessor{exporter: exp}
}

func (p *errorSpanProcessor) OnStart(_ context.Context, _ sdktrace.ReadWriteSpan) {}

func (p *errorSpanProcessor) OnEnd(span sdktrace.ReadOnlySpan) {
	if span.Status().Code != codes.Error {
		return
	}
	if span.SpanContext().TraceFlags().IsSampled() {
		// Already sampled — BatchSpanProcessor will export it.
		return
	}
	// Unsampled error: export synchronously. Errors are swallowed to ensure
	// they never propagate to application code.
	_ = p.exporter.ExportSpans(context.Background(), []sdktrace.ReadOnlySpan{span})
}

func (p *errorSpanProcessor) Shutdown(_ context.Context) error  { return nil }
func (p *errorSpanProcessor) ForceFlush(_ context.Context) error { return nil }
