package tracelit

import (
	"fmt"
	"os"
	"strconv"

	otellog "go.opentelemetry.io/otel/log"
)

const (
	defaultEndpoint    = "https://ingest.tracelit.app"
	defaultEnvironment = "production"
	defaultSampleRate  = 1.0
)

// Config holds all configuration for the Tracelit SDK.
type Config struct {
	// APIKey is your Tracelit API key. Required.
	// Env: TRACELIT_API_KEY
	APIKey string

	// ServiceName is the name of your service as it appears in Tracelit. Required.
	// Env: TRACELIT_SERVICE_NAME
	ServiceName string

	// Environment tag — production, staging, development, etc.
	// Env: TRACELIT_ENVIRONMENT (default: "production")
	Environment string

	// Endpoint is the full base URL of the Tracelit ingest endpoint.
	// Signal-specific paths (/v1/traces, /v1/metrics, /v1/logs) are appended automatically.
	// Env: TRACELIT_ENDPOINT (default: "https://ingest.tracelit.app")
	Endpoint string

	// SampleRate controls head-based trace sampling (0.0–1.0).
	// 1.0 keeps all traces; 0.1 keeps 10%. Errors are always exported regardless.
	// Env: TRACELIT_SAMPLE_RATE (default: 1.0)
	SampleRate float64

	// Enabled controls whether the SDK is active. Set to false to disable all
	// telemetry without removing the SDK (useful for test environments).
	// Env: TRACELIT_ENABLED (default: true)
	Enabled bool

	// ResourceAttributes are additional attributes merged into every span, log,
	// and metric as resource-level metadata. Keys and values must be strings.
	ResourceAttributes map[string]string

	// MinLogLevel is the minimum OTel severity that will be forwarded to
	// Tracelit. Records below this level are dropped before export.
	// Defaults to SeverityInfo (i.e. Debug logs are suppressed unless you
	// explicitly lower the threshold with WithMinLogLevel).
	// Env: TRACELIT_MIN_LOG_LEVEL  (values: "debug", "info", "warn", "error")
	MinLogLevel otellog.Severity
}

// Option is a functional option for configuring the SDK.
type Option func(*Config)

// WithAPIKey sets the Tracelit API key.
func WithAPIKey(key string) Option {
	return func(c *Config) { c.APIKey = key }
}

// WithServiceName sets the service name.
func WithServiceName(name string) Option {
	return func(c *Config) { c.ServiceName = name }
}

// WithEnvironment sets the deployment environment tag (e.g. "production", "staging").
func WithEnvironment(env string) Option {
	return func(c *Config) { c.Environment = env }
}

// WithEndpoint overrides the Tracelit ingest base URL.
// Only needed when self-hosting; the default is https://ingest.tracelit.app.
func WithEndpoint(endpoint string) Option {
	return func(c *Config) { c.Endpoint = endpoint }
}

// WithSampleRate sets the head-based sampling rate (0.0–1.0).
// Error spans are always exported regardless of this value.
func WithSampleRate(rate float64) Option {
	return func(c *Config) { c.SampleRate = rate }
}

// WithEnabled explicitly enables or disables all telemetry.
func WithEnabled(enabled bool) Option {
	return func(c *Config) { c.Enabled = enabled }
}

// WithMinLogLevel sets the minimum log severity forwarded to Tracelit.
// Records below this level are silently dropped at the processor layer.
// The default is otellog.SeverityInfo.
// Use otellog.SeverityDebug to forward debug-level logs as well.
func WithMinLogLevel(level otellog.Severity) Option {
	return func(c *Config) { c.MinLogLevel = level }
}

// WithResourceAttributes merges the given key/value pairs into every span,
// log, and metric as resource-level attributes.
func WithResourceAttributes(attrs map[string]string) Option {
	return func(c *Config) {
		for k, v := range attrs {
			c.ResourceAttributes[k] = v
		}
	}
}

// newConfig returns a Config pre-populated from environment variables.
// Explicit Option values override environment variables.
func newConfig(opts []Option) Config {
	cfg := Config{
		Environment:        defaultEnvironment,
		Endpoint:           defaultEndpoint,
		SampleRate:         defaultSampleRate,
		Enabled:            true,
		MinLogLevel:        otellog.SeverityInfo,
		ResourceAttributes: make(map[string]string),
	}

	cfg.applyEnv()

	for _, opt := range opts {
		opt(&cfg)
	}

	return cfg
}

// applyEnv reads Tracelit environment variables into the config.
// Values already set by a previous Option call are not overwritten here
// because applyEnv is called before options are applied in newConfig.
func (c *Config) applyEnv() {
	if v := os.Getenv("TRACELIT_API_KEY"); v != "" {
		c.APIKey = v
	}
	if v := os.Getenv("TRACELIT_SERVICE_NAME"); v != "" {
		c.ServiceName = v
	}
	if v := os.Getenv("TRACELIT_ENVIRONMENT"); v != "" {
		c.Environment = v
	}
	if v := os.Getenv("TRACELIT_ENDPOINT"); v != "" {
		c.Endpoint = v
	}
	if v := os.Getenv("TRACELIT_SAMPLE_RATE"); v != "" {
		if f, err := strconv.ParseFloat(v, 64); err == nil {
			c.SampleRate = f
		}
	}
	if v := os.Getenv("TRACELIT_ENABLED"); v == "false" {
		c.Enabled = false
	}
	if v := os.Getenv("TRACELIT_MIN_LOG_LEVEL"); v != "" {
		c.MinLogLevel = parseSeverity(v)
	}
}

// validate returns an error if the configuration is incomplete or invalid.
func (c *Config) validate() error {
	if c.APIKey == "" {
		return fmt.Errorf("tracelit: APIKey is required (set TRACELIT_API_KEY or use WithAPIKey)")
	}
	if c.ServiceName == "" {
		return fmt.Errorf("tracelit: ServiceName is required (set TRACELIT_SERVICE_NAME or use WithServiceName)")
	}
	if c.SampleRate < 0.0 || c.SampleRate > 1.0 {
		return fmt.Errorf("tracelit: SampleRate must be between 0.0 and 1.0, got %v", c.SampleRate)
	}
	return nil
}

// headers returns the common authentication and routing headers used
// by every OTLP exporter.
func (c *Config) headers() map[string]string {
	return map[string]string{
		"Authorization":  "Bearer " + c.APIKey,
		"X-Service-Name": c.ServiceName,
		"X-Environment":  c.Environment,
	}
}

// parseSeverity converts a string severity label to an OTel Severity value.
// Accepted values (case-insensitive): "debug", "info", "warn", "error".
// Unknown values return SeverityInfo as a safe default.
func parseSeverity(s string) otellog.Severity {
	switch s {
	case "debug", "DEBUG":
		return otellog.SeverityDebug
	case "warn", "WARN", "warning", "WARNING":
		return otellog.SeverityWarn
	case "error", "ERROR":
		return otellog.SeverityError
	default:
		return otellog.SeverityInfo
	}
}
