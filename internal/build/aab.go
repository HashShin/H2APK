package build

import (
	"archive/zip"
	"io"
	"os"
	"path/filepath"
	"strconv"
	"strings"

	"h2apk/internal/types"
	"h2apk/internal/util"
)

// buildAAB produces a signed Android App Bundle (.aab) for Play Store upload.
// It runs aapt2 in proto format, reorganises the output into the bundle module
// layout, invokes bundletool to assemble the .aab, then signs it with jarsigner.
func buildAAB(
	baseDir, work, proj, assetsDir, androidJar string,
	flatFiles []string,
	dexPath string,
	req types.BuildRequest,
	versionCode, minSDK, targetSDK int,
	ks, ksPass, keyAlias, keyPass string,
	bundletoolJar, aabName string,
	rec *Record,
	logf func(string, ...interface{}),
) {
	if bundletoolJar == "" {
		fail(rec, "bundletool.jar not found — place tools/bundletool.jar in the project directory to enable Play Store release builds")
	}

	isURL := strings.TrimSpace(req.URL) != ""

	// 1. aapt2 link in proto format → proto-base.apk
	logf("Building AAB: compiling resources (proto format)")
	protoApk := filepath.Join(work, "proto-base.apk")
	protoArgs := []string{"link",
		"--proto-format",
		"--manifest", filepath.Join(proj, "AndroidManifest.xml"),
		"--version-code", strconv.Itoa(versionCode),
		"--version-name", util.VersionName(req.VersionName),
		"--min-sdk-version", strconv.Itoa(minSDK),
		"--target-sdk-version", strconv.Itoa(targetSDK),
		"--auto-add-overlay",
		"-o", protoApk,
	}
	for _, f := range flatFiles {
		protoArgs = append(protoArgs, "-R", f)
	}
	if !isURL {
		protoArgs = append(protoArgs, "-A", assetsDir)
	}
	if androidJar != "" {
		protoArgs = append(protoArgs, "-I", androidJar)
	}
	run(rec, "aapt2", protoArgs, logf)

	// 2. Unzip proto-base.apk and reorganise into the bundle module layout.
	logf("Building AAB: assembling bundle module")
	moduleDir := filepath.Join(work, "bundle_module")
	os.MkdirAll(filepath.Join(moduleDir, "manifest"), 0755)
	os.MkdirAll(filepath.Join(moduleDir, "dex"), 0755)

	if err := extractProtoAPK(protoApk, moduleDir); err != nil {
		fail(rec, "failed to extract proto APK: "+err.Error())
	}

	// Copy dex into the module.
	util.CopyFile(filepath.Join(moduleDir, "dex", "classes.dex"), dexPath)

	// 3. Zip the module dir into base.zip.
	logf("Building AAB: zipping bundle module")
	baseZip := filepath.Join(work, "base.zip")
	if err := zipDir(moduleDir, baseZip); err != nil {
		fail(rec, "failed to create base.zip: "+err.Error())
	}

	// 4. bundletool build-bundle → app.aab
	logf("Building AAB: running bundletool")
	rawAAB := filepath.Join(work, "app.aab")
	run(rec, "java", []string{
		"-jar", bundletoolJar,
		"build-bundle",
		"--modules=" + baseZip,
		"--output=" + rawAAB,
		"--overwrite",
	}, logf)

	// 5. Sign the AAB with jarsigner (AAB is a ZIP/JAR; apksigner cannot sign AABs).
	logf("Building AAB: signing with jarsigner")
	// jarsigner signs in-place when --out is omitted; we sign rawAAB then copy.
	run(rec, "jarsigner", []string{
		"-keystore", ks,
		"-storepass", strings.TrimPrefix(ksPass, "pass:"),
		"-keypass", strings.TrimPrefix(keyPass, "pass:"),
		"-sigalg", "SHA256withRSA",
		"-digestalg", "SHA-256",
		rawAAB,
		keyAlias,
	}, logf)

	// 6. Copy to output/<aabName>.
	util.CopyFile(filepath.Join(baseDir, "output", aabName), rawAAB)
	logf("AAB ready: %s", aabName)
}

// extractProtoAPK reads a proto-format APK (zip) and places its entries into
// the bundle module directory using the required bundle layout:
//
//	AndroidManifest.xml → manifest/AndroidManifest.xml
//	resources.pb        → resources.pb
//	res/**              → res/**
//	assets/**           → assets/**
func extractProtoAPK(apkPath, moduleDir string) error {
	r, err := zip.OpenReader(apkPath)
	if err != nil {
		return err
	}
	defer r.Close()

	for _, f := range r.File {
		name := f.Name
		var dest string
		switch {
		case name == "AndroidManifest.xml":
			dest = filepath.Join(moduleDir, "manifest", "AndroidManifest.xml")
		case name == "resources.pb":
			dest = filepath.Join(moduleDir, "resources.pb")
		case strings.HasPrefix(name, "res/"):
			dest = filepath.Join(moduleDir, name)
		case strings.HasPrefix(name, "assets/"):
			dest = filepath.Join(moduleDir, name)
		default:
			// Skip META-INF and other entries not needed by bundletool.
			continue
		}

		if f.FileInfo().IsDir() {
			os.MkdirAll(dest, 0755)
			continue
		}
		os.MkdirAll(filepath.Dir(dest), 0755)

		rc, err := f.Open()
		if err != nil {
			return err
		}
		out, err := os.Create(dest)
		if err != nil {
			rc.Close()
			return err
		}
		_, err = io.Copy(out, rc)
		out.Close()
		rc.Close()
		if err != nil {
			return err
		}
	}
	return nil
}

// zipDir creates a zip archive at destZip containing all files under srcDir,
// with paths relative to srcDir.
func zipDir(srcDir, destZip string) error {
	f, err := os.Create(destZip)
	if err != nil {
		return err
	}
	defer f.Close()

	w := zip.NewWriter(f)
	defer w.Close()

	return filepath.Walk(srcDir, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		if info.IsDir() {
			return nil
		}
		rel, err := filepath.Rel(srcDir, path)
		if err != nil {
			return err
		}
		// Use forward slashes inside the zip.
		rel = filepath.ToSlash(rel)

		fw, err := w.Create(rel)
		if err != nil {
			return err
		}
		src, err := os.Open(path)
		if err != nil {
			return err
		}
		defer src.Close()
		_, err = io.Copy(fw, src)
		return err
	})
}
