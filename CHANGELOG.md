# Changelog

All notable changes to this project will be documented in this file.

The format is based on [Keep a Changelog](https://keepachangelog.com/en/1.1.0/),
and this project adheres to [Semantic Versioning](https://semver.org/spec/v2.0.0.html).

> **Note:** Release entries are inserted automatically by the release workflow.
> Do not add release entries manually — run `./release.sh` locally instead.

---

## [0.1.1] - 2026-04-27

- fix: update module path to github.com/tracelit-ai/tracelit-go
- chore: update CHANGELOG for v0.1.1
- chore: bump version to 0.1.1
- Refactor HTTP middleware for Go version compatibility. Introduced separate implementations of PatternMiddleware for Go 1.23 and earlier, ensuring proper route tagging and metrics handling based on the Go version. Updated documentation to reflect changes.
- Publishing Go SDK

[0.1.1]: https://github.com/Tracelit-AI/tracelit-go/commits/v0.1.1


## [0.1.1] - 2026-04-27

- chore: bump version to 0.1.1
- Refactor HTTP middleware for Go version compatibility. Introduced separate implementations of PatternMiddleware for Go 1.23 and earlier, ensuring proper route tagging and metrics handling based on the Go version. Updated documentation to reflect changes.
- Publishing Go SDK

[0.1.1]: https://github.com/Tracelit-AI/tracelit-go/commits/v0.1.1


## [0.1.0] - 2024-01-01

### Added

- Initial release of the Tracelit Go SDK.
- OTLP/HTTP exporters for traces, metrics, and logs.
- Logger bridges for slog, zap, logrus, and zerolog.
- HTTP and gRPC middleware.
- Automatic runtime and process metrics.
- Head-based sampling with error-always semantics.

[0.1.0]: https://github.com/tracelit-ai/tracelit-go/releases/tag/v0.1.0
