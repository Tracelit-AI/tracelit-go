package bridge

import (
	"go.opentelemetry.io/contrib/bridges/otelzap"
	"go.opentelemetry.io/otel/log/global"
	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"
)

// NewZapCore returns a zapcore.Core that forwards every zap log record to the
// Tracelit (OTel) log pipeline.
//
// The name argument is the instrumentation scope name — typically the package
// import path or component name (e.g. "github.com/acme/payments").
//
// Typical usage — build a new zap logger backed entirely by OTel:
//
//	logger := zap.New(bridge.NewZapCore("my-service"), zap.AddCaller())
//
// Or tee to both OTel and stdout using zap.NewTee:
//
//	stdoutCore, _ := zap.NewDevelopment()
//	logger := zap.New(zapcore.NewTee(
//	    stdoutCore.Core(),
//	    bridge.NewZapCore("my-service"),
//	))
//
// The bridge reads the context from zap fields when you use zap.Any("ctx", ctx)
// or call logger.With(zap.Any("ctx", ctx)). For automatic trace correlation,
// pass the request context via a context-aware logger:
//
//	logger.InfoContext(ctx, "request handled", zap.String("order_id", id))
func NewZapCore(name string, opts ...otelzap.Option) zapcore.Core {
	return otelzap.NewCore(name,
		append([]otelzap.Option{
			otelzap.WithLoggerProvider(global.GetLoggerProvider()),
		}, opts...)...,
	)
}

// NewZapLogger is a convenience constructor that creates a fully configured
// zap.Logger using NewZapCore. It adds the caller information and stack traces
// on error level by default.
func NewZapLogger(name string, opts ...otelzap.Option) *zap.Logger {
	return zap.New(NewZapCore(name, opts...), zap.AddCaller(), zap.AddStacktrace(zap.ErrorLevel))
}
