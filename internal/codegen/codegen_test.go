package codegen

import (
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"

	"h2apk/internal/types"
	"h2apk/internal/util"
)

func TestPaddingClientGeneration(t *testing.T) {
	tests := []struct {
		name           string
		blockAds       bool
		adguardDNS     bool
		useAssetLoader bool
	}{
		{"basic", false, false, false},
		{"basic+asset", false, false, true},
		{"adblock", true, false, false},
		{"adguard", true, true, false},
		{"adguard+asset", true, true, true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			code := GenPaddingClient(tt.blockAds, tt.adguardDNS, tt.useAssetLoader)
			if code == "" {
				t.Fatal("GenPaddingClient returned empty string")
			}
			if !strings.Contains(code, "public class PaddingClient") {
				t.Error("Missing PaddingClient class")
			}
		})
	}
}

func TestGenAndroidManifest(t *testing.T) {
	req := types.BuildRequest{PackageName: "com.test.app"}
	m := GenAndroidManifest(req, false)

	if m == "" {
		t.Fatal("GenAndroidManifest returned empty string")
	}
	if !strings.Contains(m, `package="com.test.app"`) {
		t.Error("Missing package attribute in manifest")
	}
	if !strings.Contains(m, "android:theme") {
		t.Error("Missing theme in manifest")
	}
}

func TestGenAndroidManifestAllowCleartext(t *testing.T) {
	reqOn := types.BuildRequest{PackageName: "com.test.app", AllowCleartext: true}
	mOn := GenAndroidManifest(reqOn, false)
	if !strings.Contains(mOn, `android:usesCleartextTraffic="true"`) {
		t.Error("AllowCleartext=true: expected usesCleartextTraffic=true in manifest")
	}

	reqOff := types.BuildRequest{PackageName: "com.test.app", AllowCleartext: false}
	mOff := GenAndroidManifest(reqOff, false)
	if !strings.Contains(mOff, `android:usesCleartextTraffic="false"`) {
		t.Error("AllowCleartext=false: expected usesCleartextTraffic=false in manifest")
	}
}

func TestGenWebViewActivityWebDebug(t *testing.T) {
	paramsOn := WebViewActivityParams{WebDebug: true}
	srcOn := GenWebViewActivitySrc(paramsOn)
	if !strings.Contains(srcOn, "setWebContentsDebuggingEnabled(true)") {
		t.Error("WebDebug=true: expected setWebContentsDebuggingEnabled(true) in source")
	}

	paramsOff := WebViewActivityParams{WebDebug: false}
	srcOff := GenWebViewActivitySrc(paramsOff)
	if !strings.Contains(srcOff, "setWebContentsDebuggingEnabled(false)") {
		t.Error("WebDebug=false: expected setWebContentsDebuggingEnabled(false) in source")
	}
}

func TestGenJavaFile(t *testing.T) {
	tmpDir, err := os.MkdirTemp("", "h2apk-test-*")
	if err != nil {
		t.Fatalf("MkdirTemp failed: %v", err)
	}
	defer os.RemoveAll(tmpDir)

	testFile := filepath.Join(tmpDir, "Test.java")
	util.WriteFile(testFile, `package test;
public class Test {
    public void hello() {}
}`)

	content, err := os.ReadFile(testFile)
	if err != nil {
		t.Fatalf("Failed to read generated file: %v", err)
	}
	if !strings.Contains(string(content), "public class Test") {
		t.Error("Generated file content mismatch")
	}
}

func TestShimScriptGeneration(t *testing.T) {
	clipboard := clipShimScript()
	if clipboard == "" {
		t.Fatal("clipShimScript returned empty string")
	}

	notif := notifShimScript()
	if notif == "" {
		t.Fatal("notifShimScript returned empty string")
	}

	share := shareShimScript()
	if share == "" {
		t.Fatal("shareShimScript returned empty string")
	}

	speech := speechShimScript()
	if speech == "" {
		t.Fatal("speechShimScript returned empty string")
	}
}

func TestRegexPatterns(t *testing.T) {
	urlRegex := regexp.MustCompile(`(?i)^https?://`)
	tests := []struct {
		url   string
		isURL bool
	}{
		{"https://example.com", true},
		{"http://example.com", true},
		{"file:///path/to/index.html", false},
		{"not-a-url", false},
	}

	for _, tt := range tests {
		result := urlRegex.MatchString(tt.url)
		if result != tt.isURL {
			t.Errorf("URL regex match(%s) = %v, want %v", tt.url, result, tt.isURL)
		}
	}
}

func BenchmarkPaddingClientGeneration(b *testing.B) {
	for i := 0; i < b.N; i++ {
		GenPaddingClient(true, true, false)
	}
}
