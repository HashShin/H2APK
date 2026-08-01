package util

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"h2apk/internal/types"
)

func TestParseThemeColor(t *testing.T) {
	req := types.BuildRequest{ThemeColor: ""}
	hex, colorInt := ParseThemeColor(req)
	if hex == "" {
		t.Error("ThemeColor hex should not be empty")
	}
	if colorInt == 0 {
		t.Error("Theme color int should not be 0")
	}
}

func TestNavColorGeneration(t *testing.T) {
	tests := []struct {
		name        string
		transparent bool
		themeColor  string
		expected    string
	}{
		{"opaque", false, "0x1B1B1F", "0x1B1B1F"},
		{"transparent", true, "0x1B1B1F", "0x00000000"},
		{"opaque black", false, "0xFF000000", "0xFF000000"},
		{"opaque white", false, "0xFFFFFFFF", "0xFFFFFFFF"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := NavBarColor(tt.transparent, tt.themeColor)
			if result != tt.expected {
				t.Errorf("NavBarColor(%v, %s) = %s, want %s",
					tt.transparent, tt.themeColor, result, tt.expected)
			}
		})
	}
}

func TestShellColorGeneration(t *testing.T) {
	tests := []struct {
		name        string
		isURL       bool
		themeHex    string
		shouldNotBe string
	}{
		{"url", true, "0x1B1B1F", "0x1C1C1E"},
		{"local", false, "0x1B1B1F", ""},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := StatusBarColor(tt.isURL, tt.themeHex)
			if result == tt.shouldNotBe && tt.shouldNotBe != "" {
				t.Errorf("StatusBarColor(%v, %s) = %s (should not be default)",
					tt.isURL, tt.themeHex, result)
			}
		})
	}
}

func TestBuildRequestDefaults(t *testing.T) {
	req := types.BuildRequest{
		PackageName: "com.test.app",
		URL:         "https://example.com",
	}

	hex, colorInt := ParseThemeColor(req)
	if hex == "" {
		t.Error("ThemeColor hex should not be empty")
	}
	if colorInt == 0 {
		t.Error("Theme color int should not be 0")
	}
	if !strings.HasPrefix(hex, "0x") && !strings.HasPrefix(hex, "#") {
		t.Errorf("ThemeColor hex should start with 0x or #: %s", hex)
	}
}

func TestVersionName(t *testing.T) {
	tests := []struct {
		input string
	}{
		{"1"},
		{"1.0"},
		{"2.3.4"},
	}

	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			got := VersionName(tt.input)
			if got == "" {
				t.Error("VersionName should not return empty")
			}
		})
	}
}

func TestSafeName(t *testing.T) {
	tests := []string{
		"Hello World",
		"test-app",
		"hello.world",
		"UPPER",
	}

	for _, input := range tests {
		t.Run(input, func(t *testing.T) {
			got := SafeName(input)
			if got == "" {
				t.Errorf("SafeName(%q) returned empty", input)
			}
		})
	}
}

func TestXMLEscape(t *testing.T) {
	tests := []struct {
		input string
		want  string
	}{
		{"hello", "hello"},
		{"a<b", "a&lt;b"},
		{"a>b", "a&gt;b"},
		{"a&b", "a&amp;b"},
		{"a\"b", "a&quot;b"},
		{"a'b", "a&apos;b"},
	}

	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			got := XmlEscape(tt.input)
			if got != tt.want {
				t.Errorf("XmlEscape(%q) = %q, want %q", tt.input, got, tt.want)
			}
		})
	}
}

func TestCopyFile(t *testing.T) {
	tmpDir, err := os.MkdirTemp("", "h2apk-test-*")
	if err != nil {
		t.Fatalf("MkdirTemp failed: %v", err)
	}
	defer os.RemoveAll(tmpDir)

	src := filepath.Join(tmpDir, "src.txt")
	dst := filepath.Join(tmpDir, "dst.txt")

	if err := os.WriteFile(src, []byte("hello world"), 0644); err != nil {
		t.Fatalf("Failed to write source: %v", err)
	}

	CopyFile(dst, src)

	content, err := os.ReadFile(dst)
	if err != nil {
		t.Fatalf("Failed to read destination: %v", err)
	}
	if string(content) != "hello world" {
		t.Errorf("CopyFile content mismatch: got %q", string(content))
	}
}

func TestSelectionAccentColor(t *testing.T) {
	tests := []struct {
		input string
	}{
		// Very dark — must be lightened.
		{"#1C1C1E"},
		{"#000000"},
		{"#111111"},
		// Medium-bright — should stay close to original.
		{"#888888"},
		{"#AAAAAA"},
		// Already bright — unchanged.
		{"#FFFFFF"},
		{"#FF8800"},
		// No hash prefix.
		{"1C1C1E"},
	}

	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			got := SelectionAccentColor(tt.input)
			if got == "" {
				t.Error("result is empty")
			}
			if got[0] != '#' {
				t.Errorf("result should start with #: %s", got)
			}
		})
	}

	// Verify dark colours are lightened.
	if got := SelectionAccentColor("#1C1C1E"); got == "#1C1C1E" {
		t.Error("dark #1C1C1E should be lightened")
	}
	// Verify bright colours are unchanged.
	if got := SelectionAccentColor("#FFFFFF"); got != "#FFFFFF" {
		t.Errorf("bright #FFFFFF should be unchanged, got %s", got)
	}
}

func BenchmarkNavColor(b *testing.B) {
	for i := 0; i < b.N; i++ {
		NavBarColor(false, "0x1B1B1F")
	}
}
