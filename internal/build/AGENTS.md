# internal/build/ — APK Build Pipeline

## Purpose

Orchestrates the full APK build from a `BuildRequest` through: icon processing, splash screen, manifest generation, Java source generation (via `codegen`), javac compilation, DEX conversion (d8), resource linking (aapt2), ZIP packaging, zipalign, and APK signing (apksigner).

## Ownership

- `build.go` — `Builder.Build()`: the single build entry point; handles both debug (APK) and release (APK + AAB) modes
- `aab.go` — `buildAAB()`: AAB pipeline — aapt2 proto-format link, bundle module assembly, bundletool, jarsigner
- `registry.go` — `Registry`: concurrent-safe in-memory store of build records with log channels and artifact lists
- `build_test.go` — Integration tests: `TestRealAPKBuild`, `TestTransparentNavBarRealBuild`, utility tests
- `aab_build_test.go` — `TestReleaseAABBuild`: full release pipeline test (auto-skips if bundletool/jarsigner absent)
- `navbar_build_test.go` — Dedicated transparent nav-bar full-pipeline test

## Local Contracts

- `Builder` holds `BaseDir`, `*config.Resolver`, `*Registry`, and a `sync.Once` keystore path
- `Build()` runs synchronously in a goroutine; it communicates status via `Record.LogCh` (buffered channel, 50 slots) and final state via `Record.Status` / `Record.APKName` / `Record.Err`
- Build failures signal via `panic("build-fail")`, caught by the deferred recover in `Build()`
- All build work happens in `output/build_<id>/` and is cleaned up on completion
- Keystore handling: uses embedded debug keystore by default; accepts base64-encoded custom keystore from `BuildRequest.KeystoreBase64`
- The build pipeline is sequential (no parallelism) — each step depends on the previous step's output
- `Registry` is safe for concurrent access via `sync.RWMutex`

## Pipeline Steps (in order)

1. Process app icon (base64 decode → PNG compress → aapt2 compile)
2. Process splash image (if enabled)
3. Write `styles.xml` and compile via aapt2
4. Generate `AndroidManifest.xml` (via `codegen.GenAndroidManifest`; cleartext/debug flags driven by `BuildMode`)
5. Write web assets: `index.html`, asset files (with shim injection for `.html` files)
6. Generate Java sources (WebViewActivity, ChromeClient, helpers — via `codegen` package; `WebDebug=false` in release)
7. Compile Java → `.class` files (javac)
8. Convert `.class` → `classes.dex` (d8)
9. Link resources + manifest → unsigned APK (aapt2; dynamic `--version-code`, `--min-sdk-version`, `--target-sdk-version`)
10. Add `classes.dex` to APK (zip)
11. Align APK (zipalign)
12. Sign APK (apksigner; release requires custom keystore)
13. **Debug:** copy `<SafeName>.apk` → `output/`; set `rec.Artifacts=[{apk}]`
14. **Release:** copy `<SafeName>-release.apk` → `output/`; call `buildAAB()` → `<SafeName>.aab`; set `rec.Artifacts=[{aab},{apk}]`

## Work Guidance

- When adding build features that affect the Java source, the corresponding codegen function must be updated in `internal/codegen/`
- New build options in `BuildRequest` must be plumbed through both the server request parsing and the `Builder.Build()` method
- Pipeline debugging: the `logf` function writes to both stdout and the `Record.LogCh` channel for SSE streaming
- Custom keystore signing requires `KeystorePass` and `KeyAlias`; `KeyPass` defaults to `KeystorePass` if empty

## Verification

| Check | Command |
|-------|---------|
| Full pipeline (debug APK) | `go test -v -run TestRealAPKBuild ./internal/build` |
| Full pipeline (release AAB) | `go test -v -run TestReleaseAABBuild ./internal/build` |
| Transparent nav bar build | `go test -v -run TestTransparentNavBarRealBuild ./internal/build` |
| Build step presence | `go test -v -run TestBuildPipelineSteps ./internal/build` |
| AGENTS.md self-check | `go test -v -run TestBuildCommandExists ./internal/build` |
