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
// # Route tagging (Go 1.22+ ServeMux)
//
//	mux.Handle("GET /products/{id}",
//	    middleware.PatternMiddleware(http.HandlerFunc(handleGetProduct)))
//
// PatternMiddleware reads r.Pattern (the matched route template) after the
// ServeMux has done its routing and back-fills the span name and http.route
// attribute so every trace shows the template ("/products/{id}") instead of
// the concrete path ("/products/42").
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
	"strings"

	"go.opentelemetry.io/contrib/instrumentation/net/http/otelhttp"
	semconv "go.opentelemetry.io/otel/semconv/v1.26.0"
	"go.opentelemetry.io/otel/trace"
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

// PatternMiddleware is a thin wrapper that must be applied to individual route
// handlers (not the whole mux). After the Go 1.22 ServeMux sets r.Pattern on
// the request it:
//
//  1. Renames the current OTel span to "<METHOD> <route>" (e.g. "GET /products/{id}")
//  2. Sets the http.route span attribute to the route template ("/products/{id}")
//  3. Adds the http.route attribute to the request's metrics labeler so that
//     the otelhttp HTTP server duration/size histograms are broken out per route.
//
// Example:
//
//	mux.Handle("GET /products/{id}",
//	    middleware.PatternMiddleware(http.HandlerFunc(s.handleGetProduct)))
func PatternMiddleware(h http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Pattern != "" {
			route := patternToRoute(r.Pattern)
			attr := semconv.HTTPRoute(route)

			span := trace.SpanFromContext(r.Context())
			span.SetName(r.Method + " " + route)
			span.SetAttributes(attr)

			labeler, _ := otelhttp.LabelerFromContext(r.Context())
			labeler.Add(attr)
		}
		h.ServeHTTP(w, r)
	})
}

// patternToRoute strips the optional HTTP-method prefix from a Go 1.22
// ServeMux pattern.  "GET /products/{id}" → "/products/{id}"
func patternToRoute(pattern string) string {
	if idx := strings.Index(pattern, " "); idx >= 0 {
		return pattern[idx+1:]
	}
	return pattern
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
