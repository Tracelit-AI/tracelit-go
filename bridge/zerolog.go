package bridge

import (
	"context"
	"encoding/json"
	"io"
	"time"

	"github.com/rs/zerolog"
	"go.opentelemetry.io/otel/log"
	"go.opentelemetry.io/otel/log/global"
)

// zerologWriter is a zerolog.LevelWriter that mirrors every zerolog event to
// the OTel log provider while also forwarding it to the underlying writer
// (so normal console/file output is preserved).
type zerologWriter struct {
	next     io.Writer
	provider log.LoggerProvider
}

// NewZerologWriter returns a zerolog.LevelWriter that:
//  1. Forwards each log line to next (e.g. os.Stderr) so existing output is unchanged.
//  2. Mirrors the log record to the Tracelit OTel log pipeline.
//
// Trace correlation: use zerolog's context API so the active span is available
// inside the log hook:
//
//	// Attach the span to the zerolog context helper:
//	log.Ctx(ctx).Info().Str("order_id", id).Msg("order created")
//
// Or set a logger on the context at request start:
//
//	logger := log.Logger.With().Logger()
//	ctx = logger.WithContext(ctx)
//
// Usage:
//
//	log.Logger = zerolog.New(bridge.NewZerologWriter(os.Stderr)).With().Timestamp().Logger()
func NewZerologWriter(next io.Writer) zerolog.LevelWriter {
	return &zerologWriter{
		next:     next,
		provider: global.GetLoggerProvider(),
	}
}

// Write implements io.Writer. zerolog calls this for log levels that don't
// match WriteLevel (in practice, zerolog always uses WriteLevel).
func (w *zerologWriter) Write(p []byte) (int, error) {
	w.emit(zerolog.NoLevel, p)
	return w.next.Write(p)
}

// WriteLevel implements zerolog.LevelWriter. It is called for every log event
// with the effective log level.
func (w *zerologWriter) WriteLevel(level zerolog.Level, p []byte) (int, error) {
	w.emit(level, p)
	return w.next.Write(p)
}

// emit converts a zerolog JSON line into an OTel log.Record and emits it.
// Trace correlation is not available here because zerolog.LevelWriter does not
// receive a per-call context. Use NewZerologWriterWithContext for
// request-scoped trace correlation.
func (w *zerologWriter) emit(level zerolog.Level, p []byte) {
	var raw map[string]any
	if err := json.Unmarshal(p, &raw); err != nil {
		return
	}

	r := log.Record{}
	r.SetTimestamp(extractTime(raw))
	r.SetSeverity(zerologLevelToOTel(level))
	r.SetSeverityText(level.String())

	if msg, ok := raw[zerolog.MessageFieldName].(string); ok {
		r.SetBody(log.StringValue(msg))
	}

	var kvs []log.KeyValue
	for k, v := range raw {
		switch k {
		case zerolog.MessageFieldName, zerolog.LevelFieldName, zerolog.TimestampFieldName:
			continue
		}
		kvs = append(kvs, log.String(k, jsonValueToString(v)))
	}

	r.AddAttributes(kvs...)

	w.provider.Logger("tracelit").Emit(context.Background(), r)
}

// zerologWriterWithContext is a variant that accepts a context resolver so
// trace IDs can be injected from the active request context.
type zerologWriterWithContext struct {
	zerologWriter
	ctxFn func() context.Context
}

// NewZerologWriterWithContext is like NewZerologWriter but calls ctxFn() on
// every log event to obtain the current request context. Use this when you
// have a reliable way to retrieve the current context (e.g. from a goroutine-
// local or a request-scoped value).
//
//	var currentCtx context.Context // set at request start
//	log.Logger = zerolog.New(bridge.NewZerologWriterWithContext(os.Stderr, func() context.Context {
//	    return currentCtx
//	}))
func NewZerologWriterWithContext(next io.Writer, ctxFn func() context.Context) zerolog.LevelWriter {
	return &zerologWriterWithContext{
		zerologWriter: zerologWriter{next: next, provider: global.GetLoggerProvider()},
		ctxFn:         ctxFn,
	}
}

func (w *zerologWriterWithContext) WriteLevel(level zerolog.Level, p []byte) (int, error) {
	var raw map[string]any
	if err := json.Unmarshal(p, &raw); err != nil {
		_, _ = w.next.Write(p)
		return len(p), nil
	}

	r := log.Record{}
	r.SetTimestamp(extractTime(raw))
	r.SetSeverity(zerologLevelToOTel(level))
	r.SetSeverityText(level.String())

	if msg, ok := raw[zerolog.MessageFieldName].(string); ok {
		r.SetBody(log.StringValue(msg))
	}

	ctx := w.ctxFn()
	if ctx == nil {
		ctx = context.Background()
	}

	var kvs []log.KeyValue
	for k, v := range raw {
		switch k {
		case zerolog.MessageFieldName, zerolog.LevelFieldName, zerolog.TimestampFieldName:
			continue
		}
		kvs = append(kvs, log.String(k, jsonValueToString(v)))
	}

	r.AddAttributes(kvs...)
	// The OTel log SDK automatically reads the active span from ctx and sets
	// the trace_id / span_id on the protobuf LogRecord's native fields, which
	// is what the Tracelit backend reads for trace correlation.
	w.provider.Logger("tracelit").Emit(ctx, r)

	return w.next.Write(p)
}

// ─────────────────────────────────────────────────────────────────────────────
// Internal helpers
// ─────────────────────────────────────────────────────────────────────────────

func zerologLevelToOTel(level zerolog.Level) log.Severity {
	switch level {
	case zerolog.TraceLevel:
		return log.SeverityTrace
	case zerolog.DebugLevel:
		return log.SeverityDebug
	case zerolog.InfoLevel:
		return log.SeverityInfo
	case zerolog.WarnLevel:
		return log.SeverityWarn
	case zerolog.ErrorLevel:
		return log.SeverityError
	case zerolog.FatalLevel:
		return log.SeverityFatal
	case zerolog.PanicLevel:
		return log.SeverityFatal4
	default:
		return log.SeverityUndefined
	}
}

func extractTime(raw map[string]any) time.Time {
	v, ok := raw[zerolog.TimestampFieldName]
	if !ok {
		return time.Now()
	}
	switch t := v.(type) {
	case string:
		parsed, err := time.Parse(time.RFC3339, t)
		if err == nil {
			return parsed
		}
	case float64:
		return time.Unix(int64(t), 0)
	}
	return time.Now()
}

func jsonValueToString(v any) string {
	switch t := v.(type) {
	case string:
		return t
	case nil:
		return ""
	default:
		b, err := json.Marshal(t)
		if err != nil {
			return ""
		}
		return string(b)
	}
}
