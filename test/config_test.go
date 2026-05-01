// Package tracelit_test contains black-box tests for the tracelit SDK public API.
// Tests run against exported symbols only, mirroring how a consumer would use the SDK.
package tracelit_test

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	tracelit "github.com/tracelit-ai/tracelit-go"
)

// ─────────────────────────────────────────────────────────────────────────────
// newConfig defaults (via New + WithEnabled(false))
// ─────────────────────────────────────────────────────────────────────────────

func TestConfig_Defaults(t *testing.T) {
	clearEnv(t)
	sdk, err := tracelit.New(tracelit.WithEnabled(false))
	require.NoError(t, err)
	cfg := sdk.Config()

	assert.Equal(t, "production", cfg.Environment)
	assert.Equal(t, "https://ingest.tracelit.app", cfg.Endpoint)
	assert.Equal(t, 1.0, cfg.SampleRate)
	// Enabled=false was explicitly requested — verify it was honoured.
	assert.False(t, cfg.Enabled)
}

// ─────────────────────────────────────────────────────────────────────────────
// Option precedence
// ─────────────────────────────────────────────────────────────────────────────

func TestWithAPIKey(t *testing.T) {
	clearEnv(t)
	sdk, _ := tracelit.New(tracelit.WithEnabled(false), tracelit.WithAPIKey("tl_test"))
	assert.Equal(t, "tl_test", sdk.Config().APIKey)
}

func TestWithServiceName(t *testing.T) {
	clearEnv(t)
	sdk, _ := tracelit.New(tracelit.WithEnabled(false), tracelit.WithServiceName("my-svc"))
	assert.Equal(t, "my-svc", sdk.Config().ServiceName)
}

func TestWithEnvironment(t *testing.T) {
	clearEnv(t)
	sdk, _ := tracelit.New(tracelit.WithEnabled(false), tracelit.WithEnvironment("staging"))
	assert.Equal(t, "staging", sdk.Config().Environment)
}

func TestWithEndpoint(t *testing.T) {
	clearEnv(t)
	sdk, _ := tracelit.New(tracelit.WithEnabled(false), tracelit.WithEndpoint("https://custom.example.com"))
	assert.Equal(t, "https://custom.example.com", sdk.Config().Endpoint)
}

func TestWithSampleRate(t *testing.T) {
	clearEnv(t)
	sdk, _ := tracelit.New(tracelit.WithEnabled(false), tracelit.WithSampleRate(0.5))
	assert.Equal(t, 0.5, sdk.Config().SampleRate)
}

func TestWithResourceAttributes_Merge(t *testing.T) {
	clearEnv(t)
	sdk, _ := tracelit.New(
		tracelit.WithEnabled(false),
		tracelit.WithResourceAttributes(map[string]string{"region": "eu-west", "app": "checkout"}),
	)
	cfg := sdk.Config()
	assert.Equal(t, "eu-west", cfg.ResourceAttributes["region"])
	assert.Equal(t, "checkout", cfg.ResourceAttributes["app"])
}

func TestWithResourceAttributes_AccumulatesAcrossCalls(t *testing.T) {
	clearEnv(t)
	sdk, _ := tracelit.New(
		tracelit.WithEnabled(false),
		tracelit.WithResourceAttributes(map[string]string{"a": "1"}),
		tracelit.WithResourceAttributes(map[string]string{"b": "2"}),
	)
	cfg := sdk.Config()
	assert.Equal(t, "1", cfg.ResourceAttributes["a"])
	assert.Equal(t, "2", cfg.ResourceAttributes["b"])
}

// ─────────────────────────────────────────────────────────────────────────────
// Environment variable resolution
// ─────────────────────────────────────────────────────────────────────────────

func TestEnv_AllVars(t *testing.T) {
	t.Setenv("TRACELIT_API_KEY", "env_key")
	t.Setenv("TRACELIT_SERVICE_NAME", "env_svc")
	t.Setenv("TRACELIT_ENVIRONMENT", "env_env")
	t.Setenv("TRACELIT_ENDPOINT", "https://env.endpoint")
	t.Setenv("TRACELIT_SAMPLE_RATE", "0.25")

	sdk, err := tracelit.New(tracelit.WithEnabled(false))
	require.NoError(t, err)
	cfg := sdk.Config()

	assert.Equal(t, "env_key", cfg.APIKey)
	assert.Equal(t, "env_svc", cfg.ServiceName)
	assert.Equal(t, "env_env", cfg.Environment)
	assert.Equal(t, "https://env.endpoint", cfg.Endpoint)
	assert.Equal(t, 0.25, cfg.SampleRate)
}

func TestEnv_DisabledViaEnv(t *testing.T) {
	clearEnv(t)
	t.Setenv("TRACELIT_ENABLED", "false")
	sdk, err := tracelit.New()
	require.NoError(t, err)
	assert.False(t, sdk.Config().Enabled)
}

func TestEnv_InvalidSampleRate_Ignored(t *testing.T) {
	clearEnv(t)
	t.Setenv("TRACELIT_SAMPLE_RATE", "not-a-float")
	sdk, _ := tracelit.New(tracelit.WithEnabled(false))
	assert.Equal(t, 1.0, sdk.Config().SampleRate)
}

func TestOption_OverridesEnv(t *testing.T) {
	t.Setenv("TRACELIT_API_KEY", "env_key")
	sdk, _ := tracelit.New(tracelit.WithEnabled(false), tracelit.WithAPIKey("option_key"))
	assert.Equal(t, "option_key", sdk.Config().APIKey)
}

// ─────────────────────────────────────────────────────────────────────────────
// Validation errors (via New)
// ─────────────────────────────────────────────────────────────────────────────

func TestNew_MissingAPIKey_ReturnsError(t *testing.T) {
	clearEnv(t)
	_, err := tracelit.New(tracelit.WithServiceName("svc"))
	require.Error(t, err)
	assert.Contains(t, err.Error(), "APIKey")
}

func TestNew_MissingServiceName_ReturnsError(t *testing.T) {
	clearEnv(t)
	_, err := tracelit.New(tracelit.WithAPIKey("key"))
	require.Error(t, err)
	assert.Contains(t, err.Error(), "ServiceName")
}

func TestNew_SampleRateTooLow_ReturnsError(t *testing.T) {
	clearEnv(t)
	_, err := tracelit.New(tracelit.WithAPIKey("k"), tracelit.WithServiceName("s"), tracelit.WithSampleRate(-0.1))
	require.Error(t, err)
	assert.Contains(t, err.Error(), "SampleRate")
}

func TestNew_SampleRateTooHigh_ReturnsError(t *testing.T) {
	clearEnv(t)
	_, err := tracelit.New(tracelit.WithAPIKey("k"), tracelit.WithServiceName("s"), tracelit.WithSampleRate(1.1))
	require.Error(t, err)
	assert.Contains(t, err.Error(), "SampleRate")
}

func TestNew_ValidBoundarySampleRates_NoError(t *testing.T) {
	clearEnv(t)

	// Use a local server that returns 204 so validateAPIKey passes without
	// hitting the real ingest endpoint. The OTel exporters also target this
	// server; they get 204 back and treat it as a successful export.
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusNoContent)
	}))
	defer srv.Close()

	for _, rate := range []float64{0.0, 0.5, 1.0} {
		t.Run(fmt.Sprintf("rate=%.1f", rate), func(t *testing.T) {
			sdk, err := tracelit.New(
				tracelit.WithAPIKey("k"),
				tracelit.WithServiceName("s"),
				tracelit.WithEndpoint(srv.URL),
				tracelit.WithSampleRate(rate),
			)
			require.NoError(t, err, "rate=%v should be valid", rate)
			ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
			defer cancel()
			_ = sdk.Shutdown(ctx)
		})
	}
}

// ─────────────────────────────────────────────────────────────────────────────
// Helper
// ─────────────────────────────────────────────────────────────────────────────

func clearEnv(t *testing.T) {
	t.Helper()
	for _, k := range []string{
		"TRACELIT_API_KEY", "TRACELIT_SERVICE_NAME", "TRACELIT_ENVIRONMENT",
		"TRACELIT_ENDPOINT", "TRACELIT_SAMPLE_RATE", "TRACELIT_ENABLED",
	} {
		t.Setenv(k, "")
	}
}
