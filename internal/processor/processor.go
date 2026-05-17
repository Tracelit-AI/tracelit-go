// Package processor provides the custom OTel SpanProcessor used by the Tracelit SDK.
// It is an internal implementation detail and must not be imported by consumers.
package processor

import (
	"context"
	"sync"
	"time"

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
	queue    chan sdktrace.ReadOnlySpan
	done     chan struct{}
	stopOnce sync.Once
	wg       sync.WaitGroup
}

// NewErrorSpan returns a SpanProcessor that synchronously exports any span
// whose status is Error and whose trace was not head-sampled.
func NewErrorSpan(exp sdktrace.SpanExporter) sdktrace.SpanProcessor {
	p := &errorSpanProcessor{
		exporter: exp,
		queue:    make(chan sdktrace.ReadOnlySpan, 512),
		done:     make(chan struct{}),
	}
	p.wg.Add(1)
	go func() {
		defer p.wg.Done()
		p.run()
	}()
	return p
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
	// Unsampled error: enqueue for background export. Never block request
	// goroutines on network retries / exporter latency.
	select {
	case <-p.done:
		return
	case p.queue <- span:
	default:
		// Queue full — drop to protect caller latency.
	}
}

func (p *errorSpanProcessor) run() {
	for {
		select {
		case <-p.done:
			return
		case span := <-p.queue:
			_ = p.exporter.ExportSpans(context.Background(), []sdktrace.ReadOnlySpan{span})
		}
	}
}

func (p *errorSpanProcessor) Shutdown(_ context.Context) error {
	p.stopOnce.Do(func() {
		close(p.done)
	})
	p.wg.Wait()
	return nil
}

func (p *errorSpanProcessor) ForceFlush(ctx context.Context) error {
	// Best effort queue drain without blocking indefinitely.
	deadline, hasDeadline := ctx.Deadline()
	for {
		if len(p.queue) == 0 {
			return nil
		}
		if hasDeadline && time.Now().After(deadline) {
			return ctx.Err()
		}
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(10 * time.Millisecond):
		}
	}
}
