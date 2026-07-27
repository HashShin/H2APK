package config

import (
	"encoding/json"
	"fmt"
	"log"
	"os"
	"os/exec"
	"path/filepath"

	"h2apk/internal/types"
)

// Resolver resolves tool paths and loads config relative to BaseDir.
type Resolver struct {
	BaseDir string
	cfg     types.Config
	loaded  bool
}

// Load reads config.json from BaseDir once and caches the result.
func (r *Resolver) Load() types.Config {
	if r.loaded {
		return r.cfg
	}
	r.loaded = true
	data, err := os.ReadFile(filepath.Join(r.BaseDir, "config.json"))
	if err != nil {
		return r.cfg
	}
	if err := json.Unmarshal(data, &r.cfg); err != nil {
		log.Printf("config.json parse error: %v", err)
	}
	return r.cfg
}

// FindLocalOrSystem resolves a tool path: local tools/ dir → config.json → system fallback.
func (r *Resolver) FindLocalOrSystem(localPath, configKey, systemPath string) string {
	p := filepath.Join(r.BaseDir, localPath)
	if _, err := os.Stat(p); err == nil {
		return p
	}
	cfg := r.Load()
	if configKey == "d8_jar" && cfg.D8Jar != "" {
		return cfg.D8Jar
	}
	if configKey == "apksigner_jar" && cfg.ApkSignerJar != "" {
		return cfg.ApkSignerJar
	}
	return systemPath
}

// FindAndroidJar locates android.jar via local tools/, config.json, or $ANDROID_HOME.
func (r *Resolver) FindAndroidJar() string {
	local := filepath.Join(r.BaseDir, "tools", "android.jar")
	if _, err := os.Stat(local); err == nil {
		return local
	}
	cfg := r.Load()
	if cfg.AndroidJar != "" {
		if _, err := os.Stat(cfg.AndroidJar); err == nil {
			return cfg.AndroidJar
		}
	}
	sdk := os.Getenv("ANDROID_HOME")
	for _, v := range []string{"34", "35", "36", "33"} {
		p := filepath.Join(sdk, "platforms", "android-"+v, "android.jar")
		if _, err := os.Stat(p); err == nil {
			return p
		}
	}
	return ""
}

// CheckDeps prints a dependency availability table at startup.
func (r *Resolver) CheckDeps() {
	type dep struct {
		name string
		path string
		ok   bool
	}
	deps := []dep{
		{name: "javac"},
		{name: "java"},
		{name: "aapt2"},
		{name: "zipalign"},
		{name: "zip"},
	}
	for i, d := range deps {
		p, err := exec.LookPath(d.name)
		if err == nil {
			deps[i].ok = true
			deps[i].path = p
		}
	}

	jarDeps := []dep{
		{name: "d8.jar", path: r.FindLocalOrSystem("tools/d8.jar", "d8_jar", "/data/data/com.termux/files/usr/share/java/d8.jar")},
		{name: "apksigner.jar", path: r.FindLocalOrSystem("tools/apksigner.jar", "apksigner_jar", "/data/data/com.termux/files/usr/share/java/apksigner.jar")},
	}
	for i, d := range jarDeps {
		if _, err := os.Stat(d.path); err == nil {
			jarDeps[i].ok = true
		}
	}

	androidJar := r.FindAndroidJar()

	fmt.Println("  Dependency check")
	fmt.Println("  ────────────────")
	for _, d := range deps {
		mark := "✓"
		if !d.ok {
			mark = "✗"
		}
		fmt.Printf("  %s %-12s %s\n", mark, d.name, d.path)
	}
	for _, d := range jarDeps {
		mark := "✓"
		if !d.ok {
			mark = "✗"
		}
		fmt.Printf("  %s %-12s %s\n", mark, d.name, d.path)
	}
	ajMark := "✓"
	if androidJar == "" {
		ajMark = "✗"
	}
	fmt.Printf("  %s %-12s %s\n", ajMark, "android.jar", androidJar)

	missing := 0
	for _, d := range deps {
		if !d.ok {
			missing++
		}
	}
	for _, d := range jarDeps {
		if !d.ok {
			missing++
		}
	}
	if androidJar == "" {
		missing++
	}
	fmt.Println()
	if missing > 0 {
		fmt.Printf("  %d dependency(s) missing. Builds may fail.\n\n", missing)
	} else {
		fmt.Println("  All dependencies found.")
	}
}
