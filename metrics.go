package tracelit

import (
	"context"
	"runtime"
	"time"

	"go.opentelemetry.io/otel/metric"
	sdkmetric "go.opentelemetry.io/otel/sdk/metric"
)

const metricsExportInterval = 60 * time.Second

// startRuntimePollers registers observable gauges for Go runtime and process
// metrics on the provided MeterProvider. Metric values are read lazily by the
// OTel SDK on each export cycle — no background goroutines are needed.
func startRuntimePollers(_ context.Context, mp *sdkmetric.MeterProvider) {
	m := mp.Meter("tracelit.runtime",
		metric.WithInstrumentationVersion(Version),
	)
	registerRuntimeMetrics(m)
}

// registerRuntimeMetrics registers all Go runtime observable gauges on m.
func registerRuntimeMetrics(m metric.Meter) {
	heapAlloc, _ := m.Int64ObservableGauge(
		"process.runtime.heap_alloc_bytes",
		metric.WithDescription("Live heap allocation in bytes"),
		metric.WithUnit("By"),
	)
	heapSys, _ := m.Int64ObservableGauge(
		"process.runtime.heap_sys_bytes",
		metric.WithDescription("Heap memory obtained from the OS"),
		metric.WithUnit("By"),
	)
	heapObjects, _ := m.Int64ObservableGauge(
		"process.runtime.heap_objects",
		metric.WithDescription("Number of allocated heap objects"),
	)
	stackSys, _ := m.Int64ObservableGauge(
		"process.runtime.stack_sys_bytes",
		metric.WithDescription("Stack memory obtained from the OS"),
		metric.WithUnit("By"),
	)
	goroutines, _ := m.Int64ObservableGauge(
		"process.runtime.goroutines",
		metric.WithDescription("Current number of goroutines"),
	)
	gcCount, _ := m.Int64ObservableGauge(
		"process.runtime.gc_count",
		metric.WithDescription("Total number of completed GC cycles"),
	)
	gcPauseNs, _ := m.Int64ObservableGauge(
		"process.runtime.gc_pause_ns",
		metric.WithDescription("Most recent GC stop-the-world pause duration"),
		metric.WithUnit("ns"),
	)
	gcHeapGoal, _ := m.Int64ObservableGauge(
		"process.runtime.gc_heap_goal_bytes",
		metric.WithDescription("Heap size goal for the next GC cycle"),
		metric.WithUnit("By"),
	)
	allSys, _ := m.Int64ObservableGauge(
		"process.runtime.sys_bytes",
		metric.WithDescription("Total OS memory used by the Go runtime"),
		metric.WithUnit("By"),
	)

	_, _ = m.RegisterCallback(func(_ context.Context, o metric.Observer) error {
		var ms runtime.MemStats
		runtime.ReadMemStats(&ms)

		o.ObserveInt64(heapAlloc, int64(ms.HeapAlloc))
		o.ObserveInt64(heapSys, int64(ms.HeapSys))
		o.ObserveInt64(heapObjects, int64(ms.HeapObjects))
		o.ObserveInt64(stackSys, int64(ms.StackSys))
		o.ObserveInt64(goroutines, int64(runtime.NumGoroutine()))
		o.ObserveInt64(gcCount, int64(ms.NumGC))
		o.ObserveInt64(allSys, int64(ms.Sys))
		if ms.NumGC > 0 {
			o.ObserveInt64(gcPauseNs, int64(ms.PauseNs[(ms.NumGC+255)%256]))
		}
		o.ObserveInt64(gcHeapGoal, int64(ms.NextGC))
		return nil
	},
		heapAlloc, heapSys, heapObjects, stackSys,
		goroutines, gcCount, allSys, gcPauseNs, gcHeapGoal,
	)
}

// ─────────────────────────────────────────────────────────────────────────────
// Metric convenience helpers (global meter)
// ─────────────────────────────────────────────────────────────────────────────

// Counter creates a named Int64Counter on the global meter.
// Use for monotonically increasing values (request counts, error counts, etc.).
func Counter(name string, opts ...metric.Int64CounterOption) (metric.Int64Counter, error) {
	return Meter("tracelit").Int64Counter(name, opts...)
}

// Histogram creates a named Float64Histogram on the global meter.
// Use for distributions (request durations, payload sizes, etc.).
func Histogram(name string, opts ...metric.Float64HistogramOption) (metric.Float64Histogram, error) {
	return Meter("tracelit").Float64Histogram(name, opts...)
}
