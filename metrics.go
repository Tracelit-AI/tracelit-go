package tracelit

import (
	"context"
	"os"
	"os/exec"
	"runtime"
	"strconv"
	"strings"
	"sync/atomic"
	"time"

	"go.opentelemetry.io/otel/metric"
	sdkmetric "go.opentelemetry.io/otel/sdk/metric"
)

const metricsExportInterval = 60 * time.Second

// startRuntimePollers registers observable gauges for Go runtime and process
// metrics on the provided MeterProvider. Metric values are read lazily by the
// OTel SDK on each export cycle — no background goroutines are needed for the
// runtime metrics. RSS and CPU pollers run as daemon goroutines and store their
// latest readings atomically for the observable gauge callbacks to pick up.
func startRuntimePollers(ctx context.Context, mp *sdkmetric.MeterProvider) {
	m := mp.Meter("tracelit.runtime",
		metric.WithInstrumentationVersion(Version),
	)
	registerRuntimeMetrics(m)
	registerProcessMetrics(ctx, m)
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

// ─── Process RSS + CPU pollers ────────────────────────────────────────────────
//
// These two atomic values are updated by background goroutines every 30/60 s
// and read by the observable gauge callbacks on each OTel export cycle.
var (
	processRSSKiB  atomic.Int64 // VmRSS in KiB; 0 = not yet sampled
	processCPUPct1 atomic.Int64 // CPU % × 10 (e.g. 45 = 4.5 %); 0 = not ready
)

// registerProcessMetrics registers process.memory.rss and
// process.runtime.cpu.usage as observable gauges and starts the background
// goroutines that keep the atomic values current.
//
// These names are the same ones queried by QueryServiceSummary in the API so
// the Memory and CPU widgets on the service dashboard show real data.
func registerProcessMetrics(ctx context.Context, m metric.Meter) {
	rssGauge, _ := m.Float64ObservableGauge(
		"process.memory.rss",
		metric.WithDescription("Process resident set size (RSS)"),
		metric.WithUnit("MB"),
	)
	cpuGauge, _ := m.Float64ObservableGauge(
		"process.runtime.cpu.usage",
		metric.WithDescription("Process CPU utilisation percentage"),
		metric.WithUnit("%"),
	)

	_, _ = m.RegisterCallback(func(_ context.Context, o metric.Observer) error {
		if kb := processRSSKiB.Load(); kb > 0 {
			o.ObserveFloat64(rssGauge, float64(kb)/1024.0)
		}
		if cpuX10 := processCPUPct1.Load(); cpuX10 > 0 {
			o.ObserveFloat64(cpuGauge, float64(cpuX10)/10.0)
		}
		return nil
	}, rssGauge, cpuGauge)

	// RSS poller — every 60 s. Prime the first value immediately.
	go func() {
		sampleRSS()
		ticker := time.NewTicker(60 * time.Second)
		defer ticker.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
				sampleRSS()
			}
		}
	}()

	// CPU poller — every 30 s. First tick initialises the baseline.
	go func() {
		ticker := time.NewTicker(30 * time.Second)
		defer ticker.Stop()
		var lastJiffies int64
		var lastWall time.Time
		for {
			select {
			case <-ctx.Done():
				return
			case t := <-ticker.C:
				cur := readCPUJiffies()
				if lastWall.IsZero() || cur == 0 {
					lastJiffies = cur
					lastWall = t
					continue
				}
				elapsed := t.Sub(lastWall).Seconds()
				delta := cur - lastJiffies
				lastJiffies = cur
				lastWall = t
				if elapsed > 0 && delta >= 0 {
					// Jiffies at 100 Hz → cpu_seconds = delta / 100
					// cpu% = (cpu_seconds / elapsed_seconds) * 100
					pct := (float64(delta) / 100.0) / elapsed * 100.0
					if pct > 100 {
						pct = 100
					}
					processCPUPct1.Store(int64(pct * 10))
				}
			}
		}
	}()
}

// sampleRSS reads the process's resident set size and stores it in
// processRSSKiB. Uses /proc/self/status on Linux; falls back to `ps` on
// macOS/BSD.
func sampleRSS() {
	// Linux: /proc/self/status contains "VmRSS: <N> kB"
	if data, err := os.ReadFile("/proc/self/status"); err == nil {
		for _, line := range strings.Split(string(data), "\n") {
			if strings.HasPrefix(line, "VmRSS:") {
				fields := strings.Fields(line)
				if len(fields) >= 2 {
					if kb, err := strconv.ParseInt(fields[1], 10, 64); err == nil && kb > 0 {
						processRSSKiB.Store(kb)
						return
					}
				}
			}
		}
	}
	// macOS/BSD fallback: `ps -o rss= -p <pid>` prints RSS in KiB.
	out, err := exec.Command("ps", "-o", "rss=", "-p", strconv.Itoa(os.Getpid())).Output()
	if err == nil {
		if kb, err := strconv.ParseInt(strings.TrimSpace(string(out)), 10, 64); err == nil && kb > 0 {
			processRSSKiB.Store(kb)
		}
	}
}

// readCPUJiffies returns the cumulative CPU time for this process in jiffies
// (clock ticks, assumed 100 Hz) from /proc/self/stat. Returns 0 on non-Linux
// systems or on any parse error — the CPU poller treats 0 as "no data".
func readCPUJiffies() int64 {
	data, err := os.ReadFile("/proc/self/stat")
	if err != nil {
		return 0
	}
	// Format: pid (comm) state ppid pgrp ... utime stime ...
	// comm can contain spaces and parentheses so find the last ')'.
	s := string(data)
	idx := strings.LastIndex(s, ")")
	if idx < 0 {
		return 0
	}
	// Fields after ')': state(0) ppid(1) pgrp(2) session(3) tty(4) tpgid(5)
	// flags(6) minflt(7) cminflt(8) majflt(9) cmajflt(10) utime(11) stime(12)
	fields := strings.Fields(s[idx+1:])
	if len(fields) < 13 {
		return 0
	}
	utime, err1 := strconv.ParseInt(fields[11], 10, 64)
	stime, err2 := strconv.ParseInt(fields[12], 10, 64)
	if err1 != nil || err2 != nil {
		return 0
	}
	return utime + stime
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

