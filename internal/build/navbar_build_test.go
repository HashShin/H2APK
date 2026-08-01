package build

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"h2apk/internal/config"
	"h2apk/internal/types"
)

// TestTransparentNavBarRealBuild runs the FULL build pipeline (javac -> d8 ->
// aapt2 -> zip -> zipalign -> apksigner) with TransparentNavBar enabled to prove
// the generated setNavigationBarColor / setNavigationBarContrastEnforced /
// SYSTEM_UI_FLAG_LAYOUT_HIDE_NAVIGATION code actually compiles against android.jar.
func TestTransparentNavBarRealBuild(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping full APK build in short mode")
	}
	root := repoRoot()
	for _, tool := range []string{"javac", "aapt2", "zipalign", "apksigner"} {
		checkTool(t, tool)
	}
	for _, jar := range []string{"android.jar", "d8.jar", "apksigner.jar"} {
		if _, err := os.Stat(filepath.Join(root, "tools", jar)); os.IsNotExist(err) {
			t.Skipf("tools/%s not found", jar)
		}
	}

	baseDir, err := os.MkdirTemp("", "h2apk-navbar-build-*")
	if err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(baseDir)

	// The Builder resolves tools relative to BaseDir; symlink the repo tools in.
	if err := os.Symlink(filepath.Join(root, "tools"), filepath.Join(baseDir, "tools")); err != nil {
		t.Fatalf("symlink tools: %v", err)
	}

	cfg := &config.Resolver{BaseDir: baseDir}
	reg := NewRegistry()
	b := &Builder{BaseDir: baseDir, Cfg: cfg, Reg: reg}

	req := types.BuildRequest{
		AppName:           "NavBarTest",
		PackageName:       "com.h2a.navbartest",
		HTML:              "<html><body style='background:#123456'><h1>hi</h1></body></html>",
		ThemeColor:        "#1B1B1F",
		TransparentNavBar: true,
	}

	id := "navbartest"
	reg.Create(id)
	b.Build(id, req, false)

	rec, _ := reg.Get(id)
	if rec.Status != "done" {
		t.Fatalf("build failed: status=%q err=%q\n--- log ---\n%s", rec.Status, rec.Err, rec.Log)
	}
	apk := filepath.Join(baseDir, "output", rec.APKName)
	fi, err := os.Stat(apk)
	if err != nil {
		t.Fatalf("APK not produced: %v", err)
	}
	if fi.Size() == 0 {
		t.Fatal("APK is empty")
	}
	if !strings.HasSuffix(rec.APKName, ".apk") {
		t.Fatalf("unexpected APK name: %s", rec.APKName)
	}
	t.Logf("built transparent-nav APK: %s (%d bytes)", rec.APKName, fi.Size())
}
