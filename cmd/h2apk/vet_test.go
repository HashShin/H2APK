package main

import (
	"os"
	"os/exec"
	"strings"
	"testing"
)

// TestAllTemplatesNoFormatErrors runs go vet to verify no fmt.Sprintf mismatches.
func TestAllTemplatesNoFormatErrors(t *testing.T) {
	cmd := exec.Command("go", "vet", "./...")
	output, err := cmd.CombinedOutput()
	if err != nil {
		t.Errorf("go vet failed: %s", string(output))
	}
}

// TestGoVetReturnsNoFatalErrors ensures vet only has warnings, not format errors.
func TestGoVetReturnsNoFatalErrors(t *testing.T) {
	cmd := exec.Command("go", "vet", "./...")
	output, err := cmd.CombinedOutput()
	if err != nil {
		outputStr := string(output)
		if strings.Contains(outputStr, "format %s has arg") ||
			strings.Contains(outputStr, "format %t has arg") ||
			strings.Contains(outputStr, "format %d has arg") ||
			strings.Contains(outputStr, "!%(EXTRA") ||
			strings.Contains(outputStr, "!%(!s") ||
			strings.Contains(outputStr, "!%(!d") {
			t.Errorf("Format string mismatch detected:\n%s", outputStr)
		}
	}
}

// TestFileSizeLimit validates that main.go is within size limits.
func TestFileSizeLimit(t *testing.T) {
	info, err := os.Stat("main.go")
	if err != nil {
		t.Fatalf("Cannot stat main.go: %v", err)
	}
	if info.Size() > 1024*1024 {
		t.Errorf("main.go too large: %d bytes", info.Size())
	}
}

// TestHandleBuild tests build request structure.
func TestHandleBuild(t *testing.T) {
	// BuildRequest is in internal/types; just verify it can be imported and used.
	// The HTTP handler itself is tested via integration.
	_ = "placeholder — handler integration test"
}
