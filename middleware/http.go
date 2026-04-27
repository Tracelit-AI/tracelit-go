// Package middleware provides OpenTelemetry instrumentation wrappers for
// net/http servers and clients. It is a thin layer over the official
// go.opentelemetry.io/contrib/instrumentation/net/http/otelhttp package that
// applies Tracelit-friendly defaults and exposes a simplified API.
//
// # Server instrumentation
//
//	mux := http.NewServeMux()
//	mux.HandleFunc("/api/orders", handleOrders)
//
//	// Wrap the entire mux — every route is automatically traced.
//	http.ListenAndServe(":8080", middleware.NewHTTPHandler(mux))
//
// # Route tagging (Go 1.23+ ServeMux)
//
//	mux.Handle("GET /products/{id}",
//	    middleware.PatternMiddleware(http.HandlerFunc(handleGetProduct)))
//
// PatternMiddleware reads r.Pattern (the matched route template, available
// since Go 1.23) after the ServeMux has done its routing and back-fills the
// span name and http.route attribute so every trace shows the template
// ("/products/{id}") instead of the concrete path ("/products/42").
// On Go 1.21 and 1.22 the function is a no-op pass-through.
//
// # Client instrumentation
//
//	client := &http.Client{
//	    Transport: middleware.NewHTTPTransport(nil), // nil uses http.DefaultTransport
//	}
//	resp, err := client.Get("https://api.example.com/items")
package middleware

import (
	"net/http"

	"go.opentelemetry.io/contrib/instrumentation/net/http/otelhttp"
)

// NewHTTPHandler wraps handler with OpenTelemetry tracing and HTTP server
// metrics. Each inbound request gets a server span named after the HTTP
// method and path (e.g. "GET /products/42").
//
// For route-template span names (e.g. "GET /products/{id}") additionally wrap
// each individual route handler with [PatternMiddleware].
//
// Additional otelhttp.Option values can be passed to customise span naming,
// attribute filters, propagation format, etc.
func NewHTTPHandler(handler http.Handler, opts ...otelhttp.Option) http.Handler {
	defaults := []otelhttp.Option{
		otelhttp.WithSpanNameFormatter(func(_ string, r *http.Request) string {
			return r.Method + " " + r.URL.Path
		}),
	}
	return otelhttp.NewHandler(handler, "", append(defaults, opts...)...)
}

// NewHTTPTransport wraps base (or http.DefaultTransport when base is nil) with
// OpenTelemetry tracing for outbound HTTP requests. Inject this into an
// http.Client to trace every outgoing call.
//
// Example:
//
//	client := &http.Client{Transport: middleware.NewHTTPTransport(nil)}
func NewHTTPTransport(base http.RoundTripper, opts ...otelhttp.Option) http.RoundTripper {
	return otelhttp.NewTransport(base, opts...)
}
