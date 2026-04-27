// Package sampler provides the custom OTel sampler used by the Tracelit SDK.
// It is an internal implementation detail and must not be imported by consumers.
package sampler

import sdktrace "go.opentelemetry.io/otel/sdk/trace"

// errorAlwaysSampler wraps an inner sampler and promotes DROP decisions to
// RECORD_ONLY so that the ErrorSpanProcessor can still observe every span
// and export those that have an error status, even when the trace is not
// sampled. Sampled spans are left unchanged and flow through BatchSpanProcessor.
type errorAlwaysSampler struct {
	wrapped sdktrace.Sampler
}

// NewErrorAlways returns a sampler that delegates to inner for the sampling
// decision but promotes DROP to RECORD_ONLY, ensuring error spans are always
// observable by ErrorSpanProcessor.
func NewErrorAlways(inner sdktrace.Sampler) sdktrace.Sampler {
	return errorAlwaysSampler{wrapped: inner}
}

func (s errorAlwaysSampler) ShouldSample(p sdktrace.SamplingParameters) sdktrace.SamplingResult {
	result := s.wrapped.ShouldSample(p)
	if result.Decision == sdktrace.Drop {
		result.Decision = sdktrace.RecordOnly
	}
	return result
}

func (s errorAlwaysSampler) Description() string {
	return "ErrorAlwaysSampler{" + s.wrapped.Description() + "}"
}

// Choose returns the appropriate sampler for rate:
//   - rate >= 1.0 → AlwaysSample (no wrapping needed, all errors are exported anyway)
//   - rate < 1.0  → ratio sampler wrapped with ErrorAlwaysSampler so unsampled
//     error spans are still recorded and exportable
func Choose(rate float64) sdktrace.Sampler {
	if rate >= 1.0 {
		return sdktrace.AlwaysSample()
	}
	return NewErrorAlways(sdktrace.ParentBased(sdktrace.TraceIDRatioBased(rate)))
}
