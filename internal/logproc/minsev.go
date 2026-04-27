// Package logproc provides log processor helpers for the Tracelit SDK.
package logproc

import (
	"context"
	"fmt"

	otellog "go.opentelemetry.io/otel/log"
	sdklog "go.opentelemetry.io/otel/sdk/log"
)

// MinSevProcessor wraps a [sdklog.Processor] and:
//  1. Implements [sdklog.FilterProcessor] so the OTel SDK can short-circuit
//     Logger.Enabled() calls for records below the minimum severity (preventing
//     the SDK from forwarding sub-threshold log records downstream).
//  2. Back-fills the SeverityText field on every emitted record when the
//     upstream bridge (e.g. otelslog) leaves it empty.
type MinSevProcessor struct {
	inner       sdklog.Processor
	minSeverity otellog.Severity
}

// New returns a MinSevProcessor that wraps inner and drops any record whose
// severity is below minSeverity.
func New(inner sdklog.Processor, minSeverity otellog.Severity) *MinSevProcessor {
	return &MinSevProcessor{inner: inner, minSeverity: minSeverity}
}

// Enabled implements [sdklog.FilterProcessor].
// Returning false here prevents the OTel SDK's logger.Enabled() from
// returning true when all registered processors are FilterProcessors — which
// in turn stops the slog bridge from constructing the record at all.
func (p *MinSevProcessor) Enabled(_ context.Context, param sdklog.EnabledParameters) bool {
	return param.Severity >= p.minSeverity
}

// OnEmit forwards the record to the inner processor after:
//   - dropping records below MinSeverity (double-check in case OnEmit is
//     called directly without a prior Enabled check), and
//   - filling in SeverityText when it is absent (the otelslog bridge does not
//     set this field, which can cause display issues in some log viewers).
func (p *MinSevProcessor) OnEmit(ctx context.Context, record *sdklog.Record) error {
	if record.Severity() < p.minSeverity {
		return nil
	}
	if record.SeverityText() == "" {
		record.SetSeverityText(severityText(record.Severity()))
	}
	return p.inner.OnEmit(ctx, record)
}

// Shutdown delegates to the inner processor.
func (p *MinSevProcessor) Shutdown(ctx context.Context) error {
	return p.inner.Shutdown(ctx)
}

// ForceFlush delegates to the inner processor.
func (p *MinSevProcessor) ForceFlush(ctx context.Context) error {
	return p.inner.ForceFlush(ctx)
}

// severityText maps an OTel Severity to its canonical string representation.
func severityText(s otellog.Severity) string {
	switch {
	case s >= otellog.SeverityFatal:
		return "FATAL"
	case s >= otellog.SeverityError:
		return "ERROR"
	case s >= otellog.SeverityWarn:
		return "WARN"
	case s >= otellog.SeverityInfo:
		return "INFO"
	case s >= otellog.SeverityDebug:
		return "DEBUG"
	default:
		return fmt.Sprintf("SEVERITY(%d)", int(s))
	}
}
