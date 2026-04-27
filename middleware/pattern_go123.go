//go:build go1.23

package middleware

import (
	"net/http"
	"strings"

	"go.opentelemetry.io/contrib/instrumentation/net/http/otelhttp"
	semconv "go.opentelemetry.io/otel/semconv/v1.26.0"
	"go.opentelemetry.io/otel/trace"
)

// PatternMiddleware must be applied to individual route handlers (not the whole
// mux). Once the Go 1.23 ServeMux populates r.Pattern it:
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

// patternToRoute strips the optional HTTP-method prefix from a Go 1.22+
// ServeMux pattern. "GET /products/{id}" → "/products/{id}"
func patternToRoute(pattern string) string {
	if idx := strings.Index(pattern, " "); idx >= 0 {
		return pattern[idx+1:]
	}
	return pattern
}
