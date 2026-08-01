# internal/ — Core Library

## Purpose

The `internal/` tree contains all H2APK library packages. Each sub-package is a sealed responsibility boundary importable only by code within the `h2apk` module (Go `internal` visibility rule).

## Ownership

- `app/` — Server startup: dependency check, route registration, port binding, browser open
- `server/` — HTTP handlers: `POST /api/build`, `GET /api/status/{id}`, `GET /api/download/{id}`, `GET /api/log/{id}` (SSE)
- `config/` — Tool-path resolver: `tools/` → `config.json` → `$ANDROID_HOME` fallback chain
- `types/` — Shared data types: `Config`, `BuildRequest`, `BuildInfo`
- `util/` — Stateless helpers: PNG compression, ID generation, package-name sanitization, XML escaping, JSON response writing
- `assets/` — Compile-time embedded web UI, payment QR images, debug keystore

## Local Contracts

- All packages are zero-dependency (stdlib only)
- `types/` owns the canonical struct definitions; all other packages import `types`, never redefine them
- `util/` functions must be pure/stateless — no global mutable state
- `assets/` files are embedded via `//go:embed` at compile time; do not read them from disk at runtime
- `config.Resolver` loads `config.json` lazily once and caches the result
- The `app.Run()` function in `app/` mirrors `cmd/h2apk/main.go` but is the importable variant for `main.go`

## Work Guidance

- New shared types go in `types/`, never duplicated across packages
- New HTTP endpoints go in `server/` via `RegisterRoutes`
- New utility functions that are used by multiple packages go in `util/`
- Single-use helpers stay in the calling package

## Verification

- Unit tests in `util/` and type-level tests execute via `go test ./internal/...`
- Server handlers are tested indirectly through the full build pipeline tests in `internal/build/`
- `cmd/h2apk/vet_test.go` runs `go vet ./...` which covers all `internal/` packages

## Child DOX Index

| Path | Purpose |
|------|---------|
| `build/` | APK build pipeline: javac → d8 → aapt2 → zip → zipalign → apksigner |
| `codegen/` | Java source generation, AndroidManifest.xml, JS shim injection |
