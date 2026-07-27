# H2APK — AI Agent Guide

## Testing Phase

Always run tests before committing changes:

```bash
# Build the binary
go build -o h2apk ./cmd/h2apk

# Run static analysis (catches format string mismatches, etc.)
go vet ./...

# Run all tests
# - Skips full APK build (requires tools: javac, aapt2, d8, zipalign, apksigner)
go test ./...

# Run tests in short mode (faster, skips integration)
go test -short ./...

# Run tests with verbose output
go test -v ./...
```

### Required Checks

All three commands must pass with **no errors**:

| Command | Purpose |
|---------|---------|
| `go build -o h2apk ./cmd/h2apk` | Verify compilation succeeds |
| `go vet ./...` | Catch fmt.Sprintf mismatches, unreachable code, etc. |
| `go test ./...` | Run all unit + integration tests |

If `go vet` reports format string errors like `%s has arg bool`, the template argument list is out of sync and must be fixed before proceeding.

### APK Build Test

To validate the **full APK build pipeline** (Java compile → DEX → aapt2 → zip → zipalign → sign):

```bash
# Run APK build test (requires build tools installed)
go test -v -run TestRealAPKBuild ./internal/build

# Skip if tools not available (uses skip)
go test -run TestRealAPKBuild ./internal/build
```

This test:
1. Reads `testweb/index.html` as source content
2. Generates Java source for WebViewActivity
3. Compiles Java to `.class` files (javac)
4. Compiles to DEX (d8)
5. Links resources (aapt2)
6. Packages APK (zip)
7. Aligns APK (zipalign)
8. Validates the final APK is a valid ZIP with correct structure

**Required tools:** javac, aapt2, zipalign, zip, java (with tools/d8.jar, tools/android.jar, tools/apksigner.jar)

If tools are missing, the test automatically skips.
