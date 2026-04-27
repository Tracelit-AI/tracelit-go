// Package bridge_test contains black-box tests for the tracelit bridge sub-package.
package bridge_test

import (
	"context"
	"testing"

	"github.com/sirupsen/logrus"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.opentelemetry.io/otel/log/global"
	"go.opentelemetry.io/otel/log/logtest"

	"github.com/tracelit-ai/tracelit-go/bridge"
)

func TestNewLogrusHook_EmitsRecord(t *testing.T) {
	rec := logtest.NewRecorder()
	prev := global.GetLoggerProvider()
	global.SetLoggerProvider(rec)
	defer global.SetLoggerProvider(prev)

	logger := logrus.New()
	logger.AddHook(bridge.NewLogrusHook())
	logger.SetOutput(discard{})
	logger.Info("hello from logrus")

	require.NotEmpty(t, rec.Result())
	records := rec.Result()[0].Records
	require.NotEmpty(t, records)
	assert.Equal(t, "hello from logrus", records[0].Body().AsString())
}

func TestNewLogrusHook_ErrorLevel_HasHighSeverity(t *testing.T) {
	rec := logtest.NewRecorder()
	prev := global.GetLoggerProvider()
	global.SetLoggerProvider(rec)
	defer global.SetLoggerProvider(prev)

	logger := logrus.New()
	logger.AddHook(bridge.NewLogrusHook())
	logger.SetOutput(discard{})
	logger.Error("broken")

	records := rec.Result()[0].Records
	require.NotEmpty(t, records)
	assert.GreaterOrEqual(t, int(records[0].Severity()), 17)
}

func TestNewLogrusHook_WithContext_DoesNotPanic(t *testing.T) {
	rec := logtest.NewRecorder()
	prev := global.GetLoggerProvider()
	global.SetLoggerProvider(rec)
	defer global.SetLoggerProvider(prev)

	logger := logrus.New()
	logger.AddHook(bridge.NewLogrusHook())
	logger.SetOutput(discard{})

	assert.NotPanics(t, func() {
		logger.WithContext(context.Background()).WithField("order_id", "x").Info("ctx log")
	})
}

func TestNewLogrusHook_Levels_NotEmpty(t *testing.T) {
	hook := bridge.NewLogrusHook()
	assert.NotEmpty(t, hook.Levels())
}

// discard silences logrus output during tests.
type discard struct{}

func (discard) Write(p []byte) (int, error) { return len(p), nil }
