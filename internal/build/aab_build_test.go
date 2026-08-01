package build

import (
	"encoding/base64"
	"os"
	"os/exec"
	"path/filepath"
	"testing"

	"h2apk/internal/assets"
	"h2apk/internal/config"
	"h2apk/internal/types"
)

// TestReleaseAABBuild exercises the full release pipeline (AAB + signed APK).
// It auto-skips if bundletool.jar, jarsigner, or any other required build tool
// is absent — mirroring the skip behaviour of TestRealAPKBuild.
func TestReleaseAABBuild(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping AAB integration test in short mode")
	}

	root := repoRoot()
	bundletoolJar := filepath.Join(root, "tools", "bundletool.jar")

	// Required PATH tools
	for _, tool := range []string{"javac", "aapt2", "zipalign", "zip", "jarsigner"} {
		if _, err := exec.LookPath(tool); err != nil {
			t.Skipf("Tool %s not found — skipping AAB build test", tool)
		}
	}

	// Required JARs
	for _, jar := range []string{
		filepath.Join(root, "tools", "android.jar"),
		filepath.Join(root, "tools", "d8.jar"),
		filepath.Join(root, "tools", "apksigner.jar"),
	} {
		if _, err := os.Stat(jar); os.IsNotExist(err) {
			t.Skipf("%s not found", jar)
		}
	}

	if _, err := os.Stat(bundletoolJar); os.IsNotExist(err) {
		t.Skip("tools/bundletool.jar not found — skipping AAB build test")
	}

	htmlPath := filepath.Join(root, "testweb", "index.html")
	if _, err := os.Stat(htmlPath); os.IsNotExist(err) {
		t.Skip("testweb/index.html not found")
	}
	htmlContent, err := os.ReadFile(htmlPath)
	if err != nil {
		t.Fatalf("Failed to read test HTML: %v", err)
	}

	// Use root as BaseDir so the Resolver can find tools/ and config.json.
	reg := NewRegistry()
	cfg := &config.Resolver{BaseDir: root}
	builder := &Builder{BaseDir: root, Cfg: cfg, Reg: reg}

	// Use the embedded debug keystore (password h2ah2a, alias h2a).
	ksB64 := base64.StdEncoding.EncodeToString(assets.Keystore)

	req := types.BuildRequest{
		AppName:        "AABTest",
		PackageName:    "com.h2a.aabtest",
		HTML:           string(htmlContent),
		BuildMode:      "release",
		VersionCode:    1,
		KeystoreBase64: ksB64,
		KeystorePass:   "h2ah2a",
		KeyAlias:       "h2a",
		AllowCleartext: false,
	}

	id := "aab-test-001"
	rec := reg.Create(id)
	go builder.Build(id, req, false)

	// Drain the log channel; Build closes it on completion.
	for range rec.LogCh {
	}

	if rec.Status != "done" {
		t.Fatalf("Release build failed: %s\nLog:\n%s", rec.Err, rec.Log)
	}

	// Verify at least one .aab artifact was produced.
	aabFound := false
	for _, a := range rec.Artifacts {
		p := filepath.Join(root, "output", a.Name)
		if _, err := os.Stat(p); err == nil {
			// Clean up test output file.
			os.Remove(p)
			if a.Kind == "aab" {
				aabFound = true
			}
		}
	}
	if !aabFound {
		t.Errorf("No .aab artifact produced; artifacts: %+v", rec.Artifacts)
	}
}
