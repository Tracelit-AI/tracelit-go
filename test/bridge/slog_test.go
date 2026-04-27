// Package bridge_test contains black-box tests for the tracelit bridge sub-package.
package bridge_test

import (
	"context"
	"log/slog"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.opentelemetry.io/otel/log/global"
	"go.opentelemetry.io/otel/log/logtest"

	"github.com/tracelit/tracelit-go/bridge"
)

func TestNewSlogHandler_EmitsRecord(t *testing.T) {
	rec, restore := installRecorder(t)
	defer restore()

	logger := slog.New(bridge.NewSlogHandler())
	logger.InfoContext(context.Background(), "hello from slog")

	require.NotEmpty(t, rec.Result())
	records := rec.Result()[0].Records
	require.NotEmpty(t, records)
	assert.Equal(t, "hello from slog", records[0].Body().AsString())
}

func TestNewSlogHandler_ErrorLevel_HasHighSeverity(t *testing.T) {
	rec, restore := installRecorder(t)
	defer restore()

	slog.New(bridge.NewSlogHandler()).ErrorContext(context.Background(), "broken")

	records := rec.Result()[0].Records
	require.NotEmpty(t, records)
	assert.GreaterOrEqual(t, int(records[0].Severity()), 17, "ERROR must map to OTel severity >= 17")
}

func TestNewSlogHandler_WithAttrs_RecordBodyCorrect(t *testing.T) {
	rec, restore := installRecorder(t)
	defer restore()

	slog.New(bridge.NewSlogHandler()).
		InfoContext(context.Background(), "order created", "order_id", "ord-42")

	require.NotEmpty(t, rec.Result()[0].Records)
	assert.Equal(t, "order created", rec.Result()[0].Records[0].Body().AsString())
}

// installRecorder replaces the global log provider with a logtest.Recorder and
// returns both the recorder and a restore function.
func installRecorder(t *testing.T) (*logtest.Recorder, func()) {
	t.Helper()
	rec := logtest.NewRecorder()
	prev := global.GetLoggerProvider()
	global.SetLoggerProvider(rec)
	return rec, func() { global.SetLoggerProvider(prev) }
}
