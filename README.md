<p align="center"><strong>HTML → APK. One click.</strong></p>

<p align="center">Paste HTML or a URL.  Get back an Android app.<br>No Android Studio.  No Gradle.  No XML.</p>

<p align="center">
  <a href="#install">Install</a> ·
  <a href="#how-it-works">How it works</a> ·
  <a href="#features">Features</a> ·
  <a href="#structure">Structure</a>
</p>

---

H2APK is a single-binary Go server that converts HTML content or website URLs into signed, installable Android APKs.  It generates the Java, compiles it, packages it, and signs it.

## Install

### 1. Run setup

```bash
git clone https://github.com/gitlawb/h2apk
cd h2apk
./setup.sh
```

`setup.sh` detects your OS (Termux, Debian/Ubuntu, Arch, Fedora) and installs everything: Go, JDK, Android SDK build-tools, plus `android.jar`, `d8.jar`, and `apksigner.jar` into `tools/`.

### 2. Build and run

```bash
go build -o h2a main.go
./h2a
```

Open `http://localhost:8080`.

On first launch the server runs a dependency check — any missing tools are reported before you start a build.

## How it works

1. You fill in the form — app name, HTML or URL, icon, optional settings.
2. The Go server writes an `AndroidManifest.xml`, generates Java sources for a WebView activity, compiles them with `javac`, converts the bytecode to DEX with `d8`, packages everything with `aapt2`, zipaligns, and signs it with an embedded debug keystore.
3. You download the APK.  The build log streams live over SSE.

<details>
<summary>Build pipeline in detail</summary>

```
HTML/URL → AndroidManifest → Java sources → javac → d8 (dex) → aapt2 pack → zip add dex → zipalign → apksigner
```
</details>

## Features

**Three input modes**
- **HTML** — write HTML, CSS, and JS directly in the editor.  In-page preview included.
- **Website URL** — wrap any website as a standalone app.
- **Upload** — drop `.html`, `.css`, `.js` files from your project.

**App customization**
- App name, package ID, version
- Custom icon (PNG upload)
- Splash screen with fade/slide animation, configurable duration, image, and background color
- Theme color picker
- Transparent navigation bar

**WebView options**
- Pull-to-refresh with a custom animated indicator
- Pinch-to-zoom (both legacy and modern Android APIs)
- Hide scrollbars
- Disable copy/long-press

**Privacy & blocking** (URL mode)
- Domain blocklist (50+ ad/tracking domains)
- AdGuard DNS blocking (150+ additional domains)
- JavaScript injection to block `window.open`, fingerprinting, and ad elements
- Cookie blocking, password saving disabled, geolocation disabled
- App redirect prevention (blocks `intent://`, `market://`, etc.)

**Build experience**
- Live build log streaming (SSE)
- Build status indicator (building → done/failed)
- Terminal-style log with persistent output
- Direct APK download link

## Structure

```
H2APK/
  main.go              — Entire backend: HTTP server, build pipeline, Java codegen
  static/index.html     — Web UI (embedded at build time)
  keystore/              — Debug signing key (embedded at build time)
  tools/                — Downloaded by setup.sh (gitignored)
  scripts/testbuild.sh  — Test script (builds URL + HTML APK via curl)
  output/               — Generated APKs
```

Everything is a single Go binary.  The HTML UI and keystore are embedded with `//go:embed`.

## API

| Endpoint | Method | Description |
|----------|--------|-------------|
| `/` | GET | Web UI |
| `/api/build` | POST | Start a build (JSON body) |
| `/api/status/{id}` | GET | Poll build status |
| `/api/log/{id}` | GET | SSE log stream |
| `/api/download/{id}` | GET | Download finished APK |

## Config

Set `PORT` to change from the default `8080`:

```bash
PORT=3000 ./h2a
```

## Test build

```bash
./scripts/testbuild.sh myapp
```

Builds two APKs — one from a URL, one from inline HTML — and confirms both succeed.

## License

MIT
