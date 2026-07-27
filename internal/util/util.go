package util

import (
	"bytes"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"image/png"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"strings"

	"h2apk/internal/types"
)

var pngEncoder = &png.Encoder{CompressionLevel: png.BestCompression}

// CompressPNG re-encodes a PNG at maximum compression.
// Returns the original bytes if decoding fails or compression yields no savings.
func CompressPNG(data []byte) []byte {
	img, err := png.Decode(bytes.NewReader(data))
	if err != nil {
		return data
	}
	var buf bytes.Buffer
	if err := pngEncoder.Encode(&buf, img); err != nil {
		return data
	}
	if buf.Len() >= len(data) {
		return data
	}
	return buf.Bytes()
}

// NewID generates a random 16-character hex build ID.
func NewID() string {
	b := make([]byte, 8)
	rand.Read(b)
	return hex.EncodeToString(b)
}

// CleanPkg sanitises a Java package name: lowercase, only [a-z0-9.].
func CleanPkg(s string) string {
	s = strings.ToLower(s)
	s = strings.Map(func(r rune) rune {
		if (r >= 'a' && r <= 'z') || (r >= '0' && r <= '9') || r == '.' {
			return r
		}
		return -1
	}, s)
	s = strings.Trim(s, ".")
	if s == "" {
		return "com.h2a.app"
	}
	return s
}

// XmlEscape escapes the five predefined XML entities.
func XmlEscape(s string) string {
	s = strings.ReplaceAll(s, "&", "&amp;")
	s = strings.ReplaceAll(s, "<", "&lt;")
	s = strings.ReplaceAll(s, ">", "&gt;")
	s = strings.ReplaceAll(s, "\"", "&quot;")
	s = strings.ReplaceAll(s, "'", "&apos;")
	return s
}

// SafeName converts an app name into a filename-safe string.
func SafeName(s string) string {
	s = strings.Map(func(r rune) rune {
		if (r >= 'a' && r <= 'z') || (r >= 'A' && r <= 'Z') || (r >= '0' && r <= '9') || r == '-' || r == '_' {
			return r
		}
		return '-'
	}, s)
	s = strings.Trim(s, "-_")
	if s == "" {
		return "app"
	}
	return s
}

// WriteFile creates parent directories then writes content to path.
func WriteFile(path, content string) {
	os.MkdirAll(filepath.Dir(path), 0755)
	os.WriteFile(path, []byte(content), 0644)
}

// CopyFile copies src to dst, creating dst if necessary.
func CopyFile(dst, src string) {
	in, _ := os.Open(src)
	if in == nil {
		return
	}
	defer in.Close()
	out, _ := os.Create(dst)
	if out == nil {
		return
	}
	defer out.Close()
	io.Copy(out, in)
}

// WriteJSON writes v as JSON with the given HTTP status code.
func WriteJSON(w http.ResponseWriter, code int, v interface{}) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(code)
	json.NewEncoder(w).Encode(v)
}

// ParseThemeColor parses req.ThemeColor (hex) and returns a Go hex string + int value.
func ParseThemeColor(req types.BuildRequest) (string, int) {
	h := req.ThemeColor
	if h == "" {
		h = "#1C1C1E"
	}
	if len(h) > 0 && h[0] == '#' {
		h = h[1:]
	}
	c, _ := strconv.ParseInt(h, 16, 32)
	alpha := int(c) | 0xFF000000
	return "0x" + fmt.Sprintf("%08X", alpha), alpha
}

// StatusBarColor returns the status-bar color string for the given context.
func StatusBarColor(isURL bool, themeHex string) string {
	if isURL {
		return themeHex
	}
	return "0x00000000"
}

// NavBarColor returns the nav-bar color string.
func NavBarColor(transparent bool, themeColor string) string {
	if transparent {
		return "0x00000000"
	}
	return themeColor
}

// VersionName returns v, defaulting to "1.0".
func VersionName(v string) string {
	if v == "" {
		return "1.0"
	}
	return v
}
