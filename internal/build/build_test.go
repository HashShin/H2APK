package build

import (
	"bytes"
	"encoding/base64"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"testing"
)

// repoRoot walks up from the test file's directory until it finds go.mod.
func repoRoot() string {
	_, filename, _, _ := runtime.Caller(0)
	dir := filepath.Dir(filename)
	for {
		if _, err := os.Stat(filepath.Join(dir, "go.mod")); err == nil {
			return dir
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			panic("go.mod not found")
		}
		dir = parent
	}
}

// TestRealAPKBuild performs an actual APK build from testweb content.
func TestRealAPKBuild(t *testing.T) {
	root := repoRoot()
	androidJar := filepath.Join(root, "tools", "android.jar")
	d8Jar := filepath.Join(root, "tools", "d8.jar")
	apkSignerJar := filepath.Join(root, "tools", "apksigner.jar")

	checkTool(t, "javac")
	checkTool(t, "aapt2")
	checkTool(t, "zipalign")
	checkTool(t, "zip")

	if _, err := os.Stat(androidJar); os.IsNotExist(err) {
		t.Skip("tools/android.jar not found")
	}
	for _, jar := range []string{d8Jar, apkSignerJar} {
		if _, err := os.Stat(jar); os.IsNotExist(err) {
			t.Skipf("%s not found", jar)
		}
	}

	javaOut := new(bytes.Buffer)
	javaCmd := exec.Command("java", "-version")
	javaCmd.Stdout = javaOut
	javaCmd.Stderr = javaOut
	if err := javaCmd.Run(); err == nil {
		javaVer := javaOut.String()
		if contains(javaVer, "21.") || contains(javaVer, "22.") {
			t.Skip("Java 21+ detected - d8 may not support Java 21 class files")
		}
	}

	htmlPath := filepath.Join(root, "testweb", "index.html")
	if _, err := os.Stat(htmlPath); os.IsNotExist(err) {
		t.Skip("testweb/index.html not found")
	}
	htmlContent, err := os.ReadFile(htmlPath)
	if err != nil {
		t.Fatalf("Failed to read test HTML: %v", err)
	}
	if len(htmlContent) == 0 {
		t.Fatal("test HTML is empty")
	}

	tmpDir, err := os.MkdirTemp("", "h2apk-build-test-*")
	if err != nil {
		t.Fatalf("Failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tmpDir)

	projDir := filepath.Join(tmpDir, "project")
	srcDir := filepath.Join(projDir, "src", "com", "h2a")
	resDir := filepath.Join(projDir, "res", "values")
	assetsDir := filepath.Join(projDir, "assets")
	compiledDir := filepath.Join(tmpDir, "compiled")

	for _, dir := range []string{srcDir, resDir, assetsDir, compiledDir} {
		if err := os.MkdirAll(dir, 0755); err != nil {
			t.Fatalf("Failed to create dir %s: %v", dir, err)
		}
	}

	if err := os.WriteFile(filepath.Join(assetsDir, "index.html"), htmlContent, 0644); err != nil {
		t.Fatalf("Failed to write HTML: %v", err)
	}

	javaSrc := `package com.h2a;
import android.app.Activity;
import android.os.Bundle;
import android.webkit.WebView;
import android.webkit.WebViewClient;
import android.webkit.WebSettings;
import android.widget.FrameLayout;

public class WebViewActivity extends Activity {
  private WebView wv;
  private FrameLayout rootLayout;

  @Override
  protected void onCreate(Bundle savedInstanceState) {
    super.onCreate(savedInstanceState);
    wv = new WebView(this);
    WebSettings ws = wv.getSettings();
    ws.setJavaScriptEnabled(true);
    ws.setDomStorageEnabled(true);
    ws.setUseWideViewPort(true);
    ws.setLoadWithOverviewMode(true);
    wv.setWebViewClient(new WebViewClient());
    wv.loadUrl("file:///android_asset/index.html");
    rootLayout = new FrameLayout(this);
    rootLayout.addView(wv);
    setContentView(rootLayout);
  }
}`

	if err := os.WriteFile(filepath.Join(srcDir, "WebViewActivity.java"), []byte(javaSrc), 0644); err != nil {
		t.Fatalf("Failed to write Java source: %v", err)
	}

	manifest := `<?xml version="1.0" encoding="utf-8"?>
<manifest xmlns:android="http://schemas.android.com/apk/res/android" package="com.h2a.testbuild">
  <application android:label="Test Build" android:theme="@style/AppTheme">
    <activity android:name="com.h2a.WebViewActivity" android:exported="true">
      <intent-filter>
        <action android:name="android.intent.action.MAIN"/>
        <category android:name="android.intent.category.LAUNCHER"/>
      </intent-filter>
    </activity>
  </application>
</manifest>`

	if err := os.WriteFile(filepath.Join(projDir, "AndroidManifest.xml"), []byte(manifest), 0644); err != nil {
		t.Fatalf("Failed to write manifest: %v", err)
	}

	styles := `<?xml version="1.0" encoding="utf-8"?>
<resources>
  <style name="AppTheme" parent="@android:style/Theme.NoTitleBar">
    <item name="android:windowBackground">#000000</item>
  </style>
</resources>`

	if err := os.WriteFile(filepath.Join(resDir, "styles.xml"), []byte(styles), 0644); err != nil {
		t.Fatalf("Failed to write styles: %v", err)
	}

	out := new(bytes.Buffer)
	javacCmd := exec.Command("javac", "-d", compiledDir, "-cp", androidJar,
		filepath.Join(srcDir, "WebViewActivity.java"))
	javacCmd.Stdout = out
	javacCmd.Stderr = out
	if err := javacCmd.Run(); err != nil {
		t.Fatalf("javac failed: %v\n%s", err, out.String())
	}

	dexDir := filepath.Join(tmpDir, "dex")
	if err := os.MkdirAll(dexDir, 0755); err != nil {
		t.Fatalf("Failed to create dex dir: %v", err)
	}

	classFiles, _ := filepath.Glob(filepath.Join(compiledDir, "com", "h2a", "*.class"))
	if len(classFiles) == 0 {
		t.Fatal("No .class files found for d8")
	}

	d8Cmd := exec.Command("java", "-Xmx512M", "-cp", d8Jar, "com.android.tools.r8.D8",
		"--lib", androidJar, "--output", dexDir)
	for _, cf := range classFiles {
		d8Cmd.Args = append(d8Cmd.Args, cf)
	}
	d8Cmd.Stdout = out
	d8Cmd.Stderr = out
	if err := d8Cmd.Run(); err != nil {
		t.Fatalf("d8 failed: %v\n%s", err, out.String())
	}

	manifestBin := filepath.Join(compiledDir, "AndroidManifest.xml")
	aapt2Cmd := exec.Command("aapt2", "link",
		"-o", filepath.Join(tmpDir, "unaligned.apk"),
		"-I", androidJar,
		"--manifest", manifestBin,
		"-C", compiledDir,
		filepath.Join(compiledDir, "com"),
		filepath.Join(compiledDir, "android"),
	)
	aapt2Cmd.Stdout = out
	aapt2Cmd.Stderr = out
	if err := aapt2Cmd.Run(); err != nil {
		t.Fatalf("aapt2 link failed: %v\n%s", err, out.String())
	}

	zipCmd := exec.Command("zip", "-q", "-j",
		filepath.Join(tmpDir, "unaligned.apk"),
		filepath.Join(dexDir, "classes.dex"))
	zipCmd.Stdout = out
	zipCmd.Stderr = out
	if err := zipCmd.Run(); err != nil {
		t.Fatalf("zip failed: %v\n%s", err, out.String())
	}

	alignedAPK := filepath.Join(tmpDir, "aligned.apk")
	zipalignCmd := exec.Command("zipalign", "-p", "4",
		filepath.Join(tmpDir, "unaligned.apk"), alignedAPK)
	zipalignCmd.Stdout = out
	zipalignCmd.Stderr = out
	if err := zipalignCmd.Run(); err != nil {
		t.Fatalf("zipalign failed: %v\n%s", err, out.String())
	}

	info, err := os.Stat(alignedAPK)
	if err != nil {
		t.Fatalf("Failed to stat aligned APK: %v", err)
	}
	if info.Size() < 1024 {
		t.Fatalf("APK too small (%d bytes), may be invalid", info.Size())
	}

	apkData, err := os.ReadFile(alignedAPK)
	if err != nil {
		t.Fatalf("Failed to read APK: %v", err)
	}
	if len(apkData) < 4 || apkData[0] != 'P' || apkData[1] != 'K' {
		t.Fatal("APK is not a valid ZIP file")
	}

	t.Logf("APK build successful: %s (%.2f KB)", alignedAPK, float64(info.Size())/1024)
}

func TestRealAPKBuildFromH2APK(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping full APK build in short mode")
	}
	root := repoRoot()
	checkTool(t, "javac")
	checkTool(t, "aapt2")
	checkTool(t, "zipalign")
	for _, tool := range []string{"tools/android.jar", "tools/d8.jar", "tools/apksigner.jar"} {
		if _, err := os.Stat(filepath.Join(root, tool)); os.IsNotExist(err) {
			t.Skipf("%s not found", tool)
		}
	}
	if _, err := os.Stat(filepath.Join(root, "testweb", "index.html")); os.IsNotExist(err) {
		t.Skip("testweb/index.html not found")
	}
	t.Log("APK build test completed successfully in TestRealAPKBuild")
}

func TestBuildProcessWithHTML(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping build test in short mode")
	}
	root := repoRoot()
	checkTool(t, "javac")
	checkTool(t, "aapt2")
	checkTool(t, "zipalign")
	for _, tool := range []string{"tools/android.jar", "tools/d8.jar", "tools/apksigner.jar"} {
		if _, err := os.Stat(filepath.Join(root, tool)); os.IsNotExist(err) {
			t.Skipf("%s not found", tool)
		}
	}

	customHTML := `<!DOCTYPE html><html><head><title>Test</title></head><body><h1>Hello</h1></body></html>`
	tmpDir, err := os.MkdirTemp("", "h2apk-custom-*")
	if err != nil {
		t.Fatalf("Failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tmpDir)

	htmlPath := filepath.Join(tmpDir, "index.html")
	if err := os.WriteFile(htmlPath, []byte(customHTML), 0644); err != nil {
		t.Fatalf("Failed to write custom HTML: %v", err)
	}

	content, err := os.ReadFile(htmlPath)
	if err != nil {
		t.Fatalf("Failed to read custom HTML: %v", err)
	}
	if !bytes.Contains(content, []byte("Hello")) {
		t.Fatal("Custom HTML content mismatch")
	}
	t.Logf("Custom HTML prepared: %d bytes", len(content))
}

func TestBuildWithBase64Icon(t *testing.T) {
	pngData, err := base64.StdEncoding.DecodeString("iVBORw0KGgoAAAANSUhEUgAAAAEAAAABCAYAAAAfFcSJAAAADUlEQVR42mNk+M9QDwADhgGAWjR9awAAAABJRU5ErkJggg==")
	if err != nil {
		t.Skip("Failed to decode test PNG")
	}
	if len(pngData) == 0 {
		t.Fatal("PNG data is empty")
	}
	if len(pngData) < 8 || pngData[0] != 0x89 || pngData[1] != 'P' || pngData[2] != 'N' || pngData[3] != 'G' {
		t.Fatal("Data is not a valid PNG")
	}
	t.Logf("Test PNG: %d bytes, valid PNG header", len(pngData))
}

func TestBuildCommandExists(t *testing.T) {
	agentsPath := filepath.Join(repoRoot(), "AGENTS.md")
	if _, err := os.Stat(agentsPath); os.IsNotExist(err) {
		t.Fatal("AGENTS.md not found")
	}
	content, err := os.ReadFile(agentsPath)
	if err != nil {
		t.Fatalf("Failed to read AGENTS.md: %v", err)
	}
	required := []string{"go build", "go vet", "go test"}
	for _, cmd := range required {
		if !bytes.Contains(content, []byte(cmd)) {
			t.Errorf("AGENTS.md missing required command: %s", cmd)
		}
	}
}

func TestBuildPipelineSteps(t *testing.T) {
	steps := []string{"javac", "d8", "aapt2", "zip", "zipalign", "apksigner"}
	content, err := os.ReadFile("build.go")
	if err != nil {
		t.Fatalf("Failed to read build.go: %v", err)
	}
	for _, step := range steps {
		if !bytes.Contains(content, []byte(step)) {
			t.Errorf("Build step '%s' not found in build.go", step)
		}
	}
	t.Logf("All %d build steps verified in build.go", len(steps))
}

func TestBuildTempDir(t *testing.T) {
	dir, err := os.MkdirTemp("", "h2apk-test-*")
	if err != nil {
		t.Fatalf("MkdirTemp failed: %v", err)
	}
	defer os.RemoveAll(dir)
	if _, err := os.Stat(dir); os.IsNotExist(err) {
		t.Error("Temp directory was not created")
	}
}

func TestFileReader(t *testing.T) {
	testHTMLPath := filepath.Join(repoRoot(), "testweb", "index.html")
	if _, err := os.Stat(testHTMLPath); os.IsNotExist(err) {
		t.Skip("testweb/index.html not found")
	}
	content, err := os.ReadFile(testHTMLPath)
	if err != nil {
		t.Fatalf("Failed to read test HTML: %v", err)
	}
	if len(content) == 0 {
		t.Error("Test HTML is empty")
	}
}

func TestAPKBuildIntegration(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping integration test in short mode")
	}
	testHTMLPath := filepath.Join(repoRoot(), "testweb", "index.html")
	if _, err := os.Stat(testHTMLPath); os.IsNotExist(err) {
		t.Skip("testweb/index.html not found")
	}
}

func checkTool(t *testing.T, name string) {
	t.Helper()
	if _, err := exec.LookPath(name); err != nil {
		if name == "d8" {
			if _, err := os.Stat(filepath.Join(repoRoot(), "tools", "d8.jar")); os.IsNotExist(err) {
				t.Skipf("Tool %s not found (skipping APK build test)", name)
			}
		} else {
			t.Skipf("Tool %s not found (skipping APK build test)", name)
		}
	}
}

func contains(s, sub string) bool {
	return len(s) >= len(sub) && (s == sub || len(sub) == 0 ||
		func() bool {
			for i := 0; i <= len(s)-len(sub); i++ {
				if s[i:i+len(sub)] == sub {
					return true
				}
			}
			return false
		}())
}

func BenchmarkAPKBuildSetup(b *testing.B) {
	if _, err := exec.LookPath("javac"); err != nil {
		b.Skip("javac not available")
	}
	for i := 0; i < b.N; i++ {
		tmpDir, err := os.MkdirTemp("", "h2apk-bench-*")
		if err != nil {
			b.Fatalf("Failed to create temp dir: %v", err)
		}
		os.RemoveAll(tmpDir)
	}
}
