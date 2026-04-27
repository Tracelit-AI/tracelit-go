// Package bridge_test contains black-box tests for the tracelit bridge sub-package.
package bridge_test

import (
	"bytes"
	"context"
	"testing"

	"github.com/rs/zerolog"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.opentelemetry.io/otel/log"
	"go.opentelemetry.io/otel/log/global"
	"go.opentelemetry.io/otel/log/logtest"

	"github.com/tracelit/tracelit-go/bridge"
)

// ─────────────────────────────────────────────────────────────────────────────
// NewZerologWriter
// ─────────────────────────────────────────────────────────────────────────────

func TestNewZerologWriter_EmitsRecord(t *testing.T) {
	rec := logtest.NewRecorder()
	prev := global.GetLoggerProvider()
	global.SetLoggerProvider(rec)
	defer global.SetLoggerProvider(prev)

	var buf bytes.Buffer
	logger := zerolog.New(bridge.NewZerologWriter(&buf)).With().Timestamp().Logger()
	logger.Info().Msg("hello from zerolog")

	require.NotEmpty(t, rec.Result())
	records := rec.Result()[0].Records
	require.NotEmpty(t, records)
	assert.Equal(t, "hello from zerolog", records[0].Body().AsString())
}

func TestNewZerologWriter_ForwardsToNextWriter(t *testing.T) {
	rec := logtest.NewRecorder()
	prev := global.GetLoggerProvider()
	global.SetLoggerProvider(rec)
	defer global.SetLoggerProvider(prev)

	var buf bytes.Buffer
	fwdLogger := zerolog.New(bridge.NewZerologWriter(&buf))
	fwdLogger.Info().Msg("forwarded")

	assert.Contains(t, buf.String(), "forwarded",
		"log line must also be written to the underlying writer")
}

func TestNewZerologWriter_LevelMapping(t *testing.T) {
	levels := []struct {
		level   zerolog.Level
		minOTel int
	}{
		{zerolog.DebugLevel, int(log.SeverityDebug)},
		{zerolog.InfoLevel, int(log.SeverityInfo)},
		{zerolog.WarnLevel, int(log.SeverityWarn)},
		{zerolog.ErrorLevel, int(log.SeverityError)},
	}
	for _, tc := range levels {
		t.Run(tc.level.String(), func(t *testing.T) {
			rec := logtest.NewRecorder()
			prev := global.GetLoggerProvider()
			global.SetLoggerProvider(rec)
			defer global.SetLoggerProvider(prev)

			var buf bytes.Buffer
			logger := zerolog.New(bridge.NewZerologWriter(&buf))

			switch tc.level {
			case zerolog.DebugLevel:
				logger.Debug().Msg("test")
			case zerolog.InfoLevel:
				logger.Info().Msg("test")
			case zerolog.WarnLevel:
				logger.Warn().Msg("test")
			case zerolog.ErrorLevel:
				logger.Error().Msg("test")
			}

			require.NotEmpty(t, rec.Result())
			records := rec.Result()[0].Records
			require.NotEmpty(t, records)
			assert.GreaterOrEqual(t, int(records[0].Severity()), tc.minOTel)
		})
	}
}

func TestNewZerologWriter_InvalidJSON_DoesNotPanic(t *testing.T) {
	rec := logtest.NewRecorder()
	prev := global.GetLoggerProvider()
	global.SetLoggerProvider(rec)
	defer global.SetLoggerProvider(prev)

	var buf bytes.Buffer
	writer := bridge.NewZerologWriter(&buf)
	assert.NotPanics(t, func() {
		_, _ = writer.WriteLevel(zerolog.InfoLevel, []byte("not-json"))
	})
}

func TestNewZerologWriter_ExtraFields_CapturedAsAttributes(t *testing.T) {
	rec := logtest.NewRecorder()
	prev := global.GetLoggerProvider()
	global.SetLoggerProvider(rec)
	defer global.SetLoggerProvider(prev)

	var buf bytes.Buffer
	extraLogger := zerolog.New(bridge.NewZerologWriter(&buf))
	extraLogger.Info().Str("order_id", "ord-999").Msg("checkout")

	records := rec.Result()[0].Records
	require.NotEmpty(t, records)

	found := false
	records[0].WalkAttributes(func(kv log.KeyValue) bool {
		if kv.Key == "order_id" {
			found = true
			assert.Equal(t, "ord-999", kv.Value.AsString())
			return false
		}
		return true
	})
	assert.True(t, found, "order_id must appear as OTel log attribute")
}

// ─────────────────────────────────────────────────────────────────────────────
// NewZerologWriterWithContext
// ─────────────────────────────────────────────────────────────────────────────

func TestNewZerologWriterWithContext_EmitsRecord(t *testing.T) {
	rec := logtest.NewRecorder()
	prev := global.GetLoggerProvider()
	global.SetLoggerProvider(rec)
	defer global.SetLoggerProvider(prev)

	var buf bytes.Buffer
	writer := bridge.NewZerologWriterWithContext(&buf, func() context.Context { return context.Background() })
	ctxLogger := zerolog.New(writer)
	ctxLogger.Info().Msg("with context")

	require.NotEmpty(t, rec.Result()[0].Records)
	assert.Equal(t, "with context", rec.Result()[0].Records[0].Body().AsString())
}

func TestNewZerologWriterWithContext_NilContextFallback_DoesNotPanic(t *testing.T) {
	rec := logtest.NewRecorder()
	prev := global.GetLoggerProvider()
	global.SetLoggerProvider(rec)
	defer global.SetLoggerProvider(prev)

	var buf bytes.Buffer
	writer := bridge.NewZerologWriterWithContext(&buf, func() context.Context { return nil })
	assert.NotPanics(t, func() {
		nilCtxLogger := zerolog.New(writer)
		nilCtxLogger.Info().Msg("nil ctx fallback")
	})
}
