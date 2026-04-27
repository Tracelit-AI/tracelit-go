// Package tracelit_test contains black-box tests for the tracelit SDK public API.
package tracelit_test

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	tracelit "github.com/tracelit/tracelit-go"
)

// ─────────────────────────────────────────────────────────────────────────────
// New — disabled SDK
// ─────────────────────────────────────────────────────────────────────────────

func TestNew_Disabled_ReturnsSDKWithoutError(t *testing.T) {
	sdk, err := tracelit.New(tracelit.WithEnabled(false))
	require.NoError(t, err)
	require.NotNil(t, sdk)
	assert.False(t, sdk.Config().Enabled)
	assert.NoError(t, sdk.Shutdown(context.Background()))
}

// ─────────────────────────────────────────────────────────────────────────────
// Configure / Shutdown global
// ─────────────────────────────────────────────────────────────────────────────

func TestConfigure_MissingAPIKey_ReturnsError(t *testing.T) {
	clearEnv(t)
	err := tracelit.Configure(tracelit.WithServiceName("svc"))
	require.Error(t, err)
	assert.Contains(t, err.Error(), "APIKey")
}

func TestShutdown_WhenNotConfigured_NoError(t *testing.T) {
	// Ensure Shutdown does not panic or error when no SDK is configured.
	assert.NoError(t, tracelit.Shutdown(context.Background()))
}

// ─────────────────────────────────────────────────────────────────────────────
// SDK.Tracer / SDK.Meter / SDK.Logger — constructors must not panic
// ─────────────────────────────────────────────────────────────────────────────

func TestSDK_Tracer_DoesNotPanic(t *testing.T) {
	sdk, _ := tracelit.New(tracelit.WithEnabled(false))
	assert.NotPanics(t, func() {
		_ = sdk.Tracer("test")
	})
}

func TestSDK_Meter_DoesNotPanic(t *testing.T) {
	sdk, _ := tracelit.New(tracelit.WithEnabled(false))
	assert.NotPanics(t, func() {
		_ = sdk.Meter("test")
	})
}

func TestSDK_Logger_DoesNotPanic(t *testing.T) {
	sdk, _ := tracelit.New(tracelit.WithEnabled(false))
	assert.NotPanics(t, func() {
		_ = sdk.Logger("test")
	})
}

// ─────────────────────────────────────────────────────────────────────────────
// Version constant
// ─────────────────────────────────────────────────────────────────────────────

func TestVersion_NotEmpty(t *testing.T) {
	assert.NotEmpty(t, tracelit.Version)
}
