# H2APK — DOX Root

## Purpose

H2APK converts HTML content or URLs into standalone Android APK or AAB files. It provides a web UI for configuration and a REST API for programmatic builds. Two build modes are supported:
- **Debug** — produces a debug-signed `.apk` (default).
- **Play Store Release** — produces a signed `.aab` (for Play Console upload) plus a `-release.apk` (for sideload testing). Requires a custom keystore and `tools/bundletool.jar`.

## Ownership

- **Module:** `h2apk` (Go 1.21, zero external dependencies)
- **Entry points:** `main.go` (thin shim) and `cmd/h2apk/main.go` (full CLI server)
- **Repository root** owns: `go.mod`, `config.json`, build scripts, tool JARs, keystore, test fixtures

## Local Contracts

- The project uses only the Go standard library — no third-party modules
- `go:embed` embeds the web UI (`internal/assets/static/index.html`), payment QR images, and the debug keystore at compile time
- Build tool JARs (d8, apksigner, android.jar) are resolved from `tools/` → `config.json` → `$ANDROID_HOME` fallback chain at runtime
- All build artifact directories (`output/`, `tmp/`) are `.gitignore`'d

## Work Guidance

### Build & Test

```bash
# Compile
go build -o h2apk ./cmd/h2apk

# Static analysis
go vet ./...

# All tests (integration tests auto-skip if build tools are missing)
go test ./...

# Short mode (unit tests only)
go test -short ./...
```

All three (`go build`, `go vet`, `go test ./...`) must pass with zero errors before any commit.

If `go vet` reports format-string mismatches like `%s has arg bool`, the `fmt.Sprintf` argument list in the affected codegen template is out of sync and must be fixed immediately.

### APK Build Validation

```bash
go test -v -run TestRealAPKBuild ./internal/build
```

This exercises the full pipeline end-to-end using `testweb/index.html` as source content. Skips automatically if `javac`, `aapt2`, `zipalign`, or the tool JARs are missing.

### Dependency Checks

Required runtime tools: `javac`, `java`, `jarsigner`, `aapt2`, `zipalign`, `zip`  
Required JARs: `tools/d8.jar`, `tools/android.jar`, `tools/apksigner.jar`  
Optional JARs: `tools/bundletool.jar` (release builds only — place in `tools/` or set `bundletool_jar` in `config.json`)

The server prints a dependency table at startup. Missing optional tools are flagged but do not block debug builds.

## Verification

| Check | Command |
|-------|---------|
| Compilation | `go build -o h2apk ./cmd/h2apk` |
| Static analysis | `go vet ./...` |
| Unit tests | `go test -short ./...` |
| Full test suite | `go test ./...` |
| APK build pipeline | `go test -v -run TestRealAPKBuild ./internal/build` |
| AAB build pipeline | `go test -v -run TestReleaseAABBuild ./internal/build` |

## User Preferences

- Always run tests before committing changes
- Use English for all communication
- No speculative features or abstractions
- Match existing code style; do not refactor adjacent code
- The `go vet` check in `cmd/h2apk/vet_test.go` is the canonical guardrail — treat it as a self-test of build quality
- Never add `Co-authored-by:` trailers to git commits. Commits must have a single human author

## Child DOX Index

| Path | Purpose |
|------|---------|
| `internal/` | Core library packages: server, build pipeline, code generation, config, types, utilities |
| `internal/build/` | APK build pipeline: javac, d8, aapt2, zip, zipalign, apksigner orchestration |
| `internal/codegen/` | Java source code generation, AndroidManifest.xml, JS shim injection |
