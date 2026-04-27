// Package middleware provides OpenTelemetry instrumentation wrappers for
// gRPC servers and clients.
//
// # Server usage
//
//	grpcServer := grpc.NewServer(
//	    grpc.StatsHandler(middleware.NewGRPCServerHandler()),
//	)
//
// # Client usage
//
//	conn, err := grpc.NewClient("localhost:50051",
//	    grpc.WithStatsHandler(middleware.NewGRPCClientHandler()),
//	)
package middleware

import (
	"go.opentelemetry.io/contrib/instrumentation/google.golang.org/grpc/otelgrpc"
	"google.golang.org/grpc"
	"google.golang.org/grpc/stats"
)

// NewGRPCServerHandler returns a gRPC stats.Handler that traces every inbound
// unary and streaming RPC call. Use it with grpc.StatsHandler:
//
//	grpc.NewServer(grpc.StatsHandler(middleware.NewGRPCServerHandler()))
func NewGRPCServerHandler(opts ...otelgrpc.Option) stats.Handler {
	return otelgrpc.NewServerHandler(opts...)
}

// NewGRPCClientHandler returns a gRPC stats.Handler that traces every outbound
// unary and streaming RPC call. Use it with grpc.WithStatsHandler:
//
//	grpc.NewClient(addr, grpc.WithStatsHandler(middleware.NewGRPCClientHandler()))
func NewGRPCClientHandler(opts ...otelgrpc.Option) stats.Handler {
	return otelgrpc.NewClientHandler(opts...)
}

// UnaryServerInterceptor returns a gRPC UnaryServerInterceptor that creates a
// server span for each unary RPC. Prefer NewGRPCServerHandler for new code;
// this interceptor is provided for compatibility with codebases that already
// use the interceptor pattern.
func UnaryServerInterceptor(opts ...otelgrpc.Option) grpc.UnaryServerInterceptor {
	return otelgrpc.UnaryServerInterceptor(opts...) //nolint:staticcheck
}

// StreamServerInterceptor returns a gRPC StreamServerInterceptor that creates
// a server span for each streaming RPC.
func StreamServerInterceptor(opts ...otelgrpc.Option) grpc.StreamServerInterceptor {
	return otelgrpc.StreamServerInterceptor(opts...) //nolint:staticcheck
}

// UnaryClientInterceptor returns a gRPC UnaryClientInterceptor that creates a
// client span for each outbound unary RPC call.
func UnaryClientInterceptor(opts ...otelgrpc.Option) grpc.UnaryClientInterceptor {
	return otelgrpc.UnaryClientInterceptor(opts...) //nolint:staticcheck
}

// StreamClientInterceptor returns a gRPC StreamClientInterceptor that creates
// a client span for each outbound streaming RPC call.
func StreamClientInterceptor(opts ...otelgrpc.Option) grpc.StreamClientInterceptor {
	return otelgrpc.StreamClientInterceptor(opts...) //nolint:staticcheck
}
