// Package bridge provides adapters that route log records from popular Go
// logging libraries (slog, zap, logrus, zerolog) to the OpenTelemetry log
// pipeline configured by the Tracelit SDK.
//
// Every bridge automatically injects the active trace ID and span ID from the
// context into each log record, so logs appear correlated with traces in the
// Tracelit UI.
//
// # Choosing a bridge
//
//   - Standard library slog  → bridge.NewSlogHandler
//   - Uber zap               → bridge.NewZapCore
//   - Logrus                 → bridge.NewLogrusHook
//   - zerolog                → bridge.NewZerologWriter
//
// # Example — slog
//
//	slog.SetDefault(slog.New(bridge.NewSlogHandler()))
//
// # Example — zap
//
//	logger := zap.New(bridge.NewZapCore("my-service"))
//
// # Example — logrus
//
//	logrus.AddHook(bridge.NewLogrusHook())
//
// # Example — zerolog
//
//	log.Logger = zerolog.New(bridge.NewZerologWriter(os.Stderr))
package bridge
