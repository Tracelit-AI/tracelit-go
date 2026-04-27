// Package bridge_test contains black-box tests for the tracelit bridge sub-package.
package bridge_test

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.opentelemetry.io/otel/log/global"
	"go.opentelemetry.io/otel/log/logtest"
	"go.uber.org/zap"

	"github.com/tracelit/tracelit-go/bridge"
)

func TestNewZapCore_EmitsRecord(t *testing.T) {
	rec := logtest.NewRecorder()
	prev := global.GetLoggerProvider()
	global.SetLoggerProvider(rec)
	defer global.SetLoggerProvider(prev)

	zap.New(bridge.NewZapCore("svc")).Info("hello from zap")

	require.NotEmpty(t, rec.Result())
	records := rec.Result()[0].Records
	require.NotEmpty(t, records)
	assert.Equal(t, "hello from zap", records[0].Body().AsString())
}

func TestNewZapCore_ErrorLevel_HasHighSeverity(t *testing.T) {
	rec := logtest.NewRecorder()
	prev := global.GetLoggerProvider()
	global.SetLoggerProvider(rec)
	defer global.SetLoggerProvider(prev)

	zap.New(bridge.NewZapCore("svc")).Error("broken")

	records := rec.Result()[0].Records
	require.NotEmpty(t, records)
	assert.GreaterOrEqual(t, int(records[0].Severity()), 17)
}

func TestNewZapLogger_DoesNotPanic(t *testing.T) {
	rec := logtest.NewRecorder()
	prev := global.GetLoggerProvider()
	global.SetLoggerProvider(rec)
	defer global.SetLoggerProvider(prev)

	assert.NotPanics(t, func() {
		bridge.NewZapLogger("svc").Info("test", zap.String("k", "v"))
	})
}
