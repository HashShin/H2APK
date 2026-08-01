# internal/codegen/ — Code Generation

## Purpose

Generates all Android source files and the AndroidManifest.xml for the APK. Also injects JavaScript shims into user-supplied HTML to polyfill browser APIs (clipboard, notifications, share, speech synthesis) through the Java-JavaScript bridge helpers.

## Ownership

| File | Responsibility |
|------|---------------|
| `codegen.go` | Java source generators: `WebViewActivity`, `H2AChromeClient`, `SplashActivity`, `PullIndicator`, `PullListener`, `ClipboardHelper`, `FileHelper`, `ShareHelper`, `TTSHelper`, `NotificationHelper`, `PaddingClient` |
| `manifest.go` | `AndroidManifest.xml` generator with conditional permissions and splash/launcher activity wiring |
| `shims.go` | JavaScript polyfills injected into HTML: clipboard (`H2AClip`), notifications (`H2A`), share (`H2AShare`), speech synthesis (`H2ATTS`) |
| `codegen_test.go` | Unit tests for manifest generation (incl. `AllowCleartext`), `WebDebug`, shim scripts, regex patterns |

## Local Contracts

- All generated Java source uses package `com.h2a` — this is hardcoded and assumed by the build pipeline
- `WebViewActivityParams` is the single parameter struct for the main activity template; every build option that affects the activity Java source must be a field here. `WebDebug bool` controls `setWebContentsDebuggingEnabled` (false in release mode)
- `fmt.Sprintf` templates use positional `%s`, `%t`, `%d` verbs — the argument order in the `Sprintf` call must match the format string exactly. Mismatches cause `go vet` failures
- Shims are injected before `</body>`, `</html>`, or `</head>` (in that order of preference)
- `WrapHTML()` is used for local HTML builds; URL-mode builds skip HTML wrapping
- `InjectShims()` is called on each `.html`/`.htm` asset file for local builds
- The `PaddingClient` generator has three variants: minimal, asset-loader, and ad-blocking (+ optional AdGuard DNS)
- `GenChromeClientSrc` has two variants: full permission-handling (camera/mic/geo) vs geo-only stub

## Work Guidance

### Adding a new Java helper class
1. Write the generator function in `codegen.go` (e.g. `GenFooHelperSrc() string`)
2. Call `util.WriteFile` in `build.go`'s Java source generation section
3. Add the `.java` file to the `javacFiles` slice in `build.go`
4. Add a unit test in `codegen_test.go` verifying the output is non-empty and contains the expected class declaration

### Modifying the WebViewActivity template
1. Add new fields to `WebViewActivityParams` in `codegen.go`
2. Add corresponding `%s`/`%t`/`%d` placeholders in the `GenWebViewActivitySrc` format string
3. Update the `fmt.Sprintf` argument list to match — verify with `go vet ./...`
4. Populate the new params in `build.go` before calling `GenWebViewActivitySrc`

### Template safety
- Every `fmt.Sprintf` call in this package is checked by `go vet` for format/argument consistency
- The `cmd/h2apk/vet_test.go` `TestGoVetReturnsNoFatalErrors` test specifically scans for format-string mismatches
- After any template change, run `go vet ./...` immediately

## Verification

| Check | Command |
|-------|---------|
| Unit tests | `go test ./internal/codegen/...` |
| Manifest generation | `TestGenAndroidManifest` |
| Shim scripts | `TestShimScriptGeneration` |
| PaddingClient variants | `TestPaddingClientGeneration` |
| Template format safety | `go vet ./internal/codegen/...` |
