package sampler_test

import (
	"testing"

	"github.com/stretchr/testify/assert"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
	"github.com/tracelit/tracelit-go/internal/sampler"
)

func TestNewErrorAlways_Description(t *testing.T) {
	inner := sdktrace.TraceIDRatioBased(0.5)
	s := sampler.NewErrorAlways(inner)
	desc := s.Description()
	assert.Contains(t, desc, "ErrorAlwaysSampler")
	assert.Contains(t, desc, inner.Description())
}

func TestNewErrorAlways_PromotesDrop_to_RecordOnly(t *testing.T) {
	inner := sdktrace.TraceIDRatioBased(0) // always drops
	s := sampler.NewErrorAlways(inner)
	result := s.ShouldSample(sdktrace.SamplingParameters{})
	assert.Equal(t, sdktrace.RecordOnly, result.Decision,
		"DROP from inner sampler must be promoted to RECORD_ONLY")
}

func TestNewErrorAlways_LeavesRecordAndSample_Unchanged(t *testing.T) {
	s := sampler.NewErrorAlways(sdktrace.AlwaysSample())
	result := s.ShouldSample(sdktrace.SamplingParameters{})
	assert.Equal(t, sdktrace.RecordAndSample, result.Decision)
}

func TestNewErrorAlways_LeavesRecordOnly_Unchanged(t *testing.T) {
	s := sampler.NewErrorAlways(recordOnlySampler{})
	result := s.ShouldSample(sdktrace.SamplingParameters{})
	assert.Equal(t, sdktrace.RecordOnly, result.Decision)
}

func TestChoose_Rate1_AlwaysSample(t *testing.T) {
	s := sampler.Choose(1.0)
	assert.Equal(t, sdktrace.AlwaysSample().Description(), s.Description())
}

func TestChoose_RateOver1_AlwaysSample(t *testing.T) {
	s := sampler.Choose(1.5)
	assert.Equal(t, sdktrace.AlwaysSample().Description(), s.Description())
}

func TestChoose_RateUnder1_WrapsWithErrorAlways(t *testing.T) {
	s := sampler.Choose(0.5)
	assert.Contains(t, s.Description(), "ErrorAlwaysSampler")
}

func TestChoose_Rate0_WrapsWithErrorAlways(t *testing.T) {
	s := sampler.Choose(0.0)
	assert.Contains(t, s.Description(), "ErrorAlwaysSampler")
}

// recordOnlySampler is a test-only sampler that always returns RecordOnly.
type recordOnlySampler struct{}

func (recordOnlySampler) ShouldSample(_ sdktrace.SamplingParameters) sdktrace.SamplingResult {
	return sdktrace.SamplingResult{Decision: sdktrace.RecordOnly}
}
func (recordOnlySampler) Description() string { return "RecordOnly" }
