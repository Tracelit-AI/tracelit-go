package bridge

import (
	"go.opentelemetry.io/contrib/bridges/otellogrus"
	"go.opentelemetry.io/otel/log/global"
	"github.com/sirupsen/logrus"
)

// NewLogrusHook returns a logrus.Hook that forwards every log entry to the
// Tracelit (OTel) log pipeline. Install it globally or on a specific logger:
//
//	// Global (affects all logrus output in the process):
//	logrus.AddHook(bridge.NewLogrusHook())
//
//	// Per-logger:
//	logger := logrus.New()
//	logger.AddHook(bridge.NewLogrusHook())
//
// Trace correlation: logrus does not carry a context per-log-call by default.
// Use logrus.WithContext to attach the active context so the hook can extract
// the trace and span IDs:
//
//	logrus.WithContext(ctx).WithField("order_id", id).Info("order created")
//
// The hook fires on all levels. To restrict it to specific levels, wrap the
// hook in a custom logrus.Hook that overrides the Levels() method.
func NewLogrusHook(opts ...otellogrus.Option) logrus.Hook {
	return otellogrus.NewHook("tracelit",
		append([]otellogrus.Option{
			otellogrus.WithLoggerProvider(global.GetLoggerProvider()),
		}, opts...)...,
	)
}
