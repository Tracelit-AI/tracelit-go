package bridge

import (
	"log/slog"

	"go.opentelemetry.io/contrib/bridges/otelslog"
	"go.opentelemetry.io/otel/log/global"
)

// NewSlogHandler returns a slog.Handler that forwards every log record to the
// Tracelit (OTel) log pipeline. The handler reads the active trace and span ID
// from the slog record's context and attaches them automatically.
//
// Typical usage — replace the default slog logger:
//
//	slog.SetDefault(slog.New(bridge.NewSlogHandler()))
//
// To retain output to stderr alongside OTel export, use slog.NewJSONHandler
// as a secondary handler in a fan-out handler, or simply write the same log
// record to both.
//
// The handler respects the OTel log level mapping:
//
//	slog.LevelDebug → OTel Debug
//	slog.LevelInfo  → OTel Info
//	slog.LevelWarn  → OTel Warn
//	slog.LevelError → OTel Error
func NewSlogHandler(opts ...otelslog.Option) slog.Handler {
	return otelslog.NewHandler("tracelit",
		append([]otelslog.Option{
			otelslog.WithLoggerProvider(global.GetLoggerProvider()),
		}, opts...)...,
	)
}
