//go:build !go1.23

package middleware

import "net/http"

// PatternMiddleware is a no-op pass-through on Go versions before 1.23.
// (*http.Request).Pattern was introduced in Go 1.23; upgrade your toolchain
// to get route-template span names and per-route metrics.
func PatternMiddleware(h http.Handler) http.Handler {
	return h
}
