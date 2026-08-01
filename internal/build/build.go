package build

import (
	"encoding/base64"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"sync"

	"h2apk/internal/assets"
	"h2apk/internal/codegen"
	"h2apk/internal/config"
	"h2apk/internal/types"
	"h2apk/internal/util"
)

// Builder executes the APK build pipeline.
type Builder struct {
	BaseDir string
	Cfg     *config.Resolver
	Reg     *Registry
	ksOnce  sync.Once
	ksPath  string
}

func (b *Builder) Build(id string, req types.BuildRequest, isURL bool) {
	rec, _ := b.Reg.Get(id)
	work := filepath.Join(b.BaseDir, "output", "build_"+id)
	logs := &strings.Builder{}
	logf := func(f string, a ...interface{}) {
		s := fmt.Sprintf(f, a...)
		logs.WriteString(s + "\n")
		fmt.Println("[build]", s)
		select {
		case rec.LogCh <- s:
		default:
		}
	}

	defer func() {
		if r := recover(); r != nil {
			if r != "build-fail" {
				rec.Status = "failed"
				rec.Err = fmt.Sprintf("panic: %v", r)
			}
		}
		rec.Log = logs.String()
		close(rec.LogCh)
		os.RemoveAll(work)
	}()

	os.MkdirAll(work, 0755)
	proj := filepath.Join(work, "project")
	assets := filepath.Join(proj, "assets")
	os.MkdirAll(assets, 0755)

	androidJar := b.Cfg.FindAndroidJar()

	// 1. icon
	flatDir := filepath.Join(work, "compiled")
	var flatFiles []string
	hasIcon := false
	if req.Icon != "" {
		logf("Processing app icon")
		iconData, err := base64.StdEncoding.DecodeString(req.Icon)
		if err != nil {
			logf("Icon decode failed, skipping: %v", err)
		} else {
			mipmapDir := filepath.Join(proj, "res", "mipmap")
			os.MkdirAll(mipmapDir, 0755)
			iconPath := filepath.Join(mipmapDir, "icon.png")
			os.WriteFile(iconPath, util.CompressPNG(iconData), 0644)
			os.MkdirAll(flatDir, 0755)
			out, err := exec.Command("aapt2", "compile", "-o", flatDir, iconPath).CombinedOutput()
			logf("[aapt2 compile] %s", string(out))
			if err != nil {
				logf("Icon compile failed, skipping: %v", err)
			} else {
				flatFiles, _ = filepath.Glob(flatDir + "/*.flat")
				hasIcon = true
				os.WriteFile(filepath.Join(assets, "icon.png"), util.CompressPNG(iconData), 0644)
			}
		}
	}

	// 1b. splash image
	if req.SplashEnabled {
		drawableDir := filepath.Join(proj, "res", "drawable")
		os.MkdirAll(drawableDir, 0755)
		splashData := req.SplashImage
		if req.SplashUseIcon && req.Icon != "" {
			splashData = req.Icon
		}
		if splashData != "" {
			imgData, err := base64.StdEncoding.DecodeString(splashData)
			if err == nil {
				splashPath := filepath.Join(drawableDir, "splash_image.png")
				os.WriteFile(splashPath, imgData, 0644)
				os.MkdirAll(flatDir, 0755)
				out, err := exec.Command("aapt2", "compile", "-o", flatDir, splashPath).CombinedOutput()
				logf("[aapt2 compile splash] %s", string(out))
				if err == nil {
					sf, _ := filepath.Glob(flatDir + "/*.flat")
					flatFiles = append(flatFiles, sf...)
				}
			}
		}
	}

	// 1c. styles.xml
	{
		themeHex := req.ThemeColor
		if themeHex == "" {
			themeHex = "#1C1C1E"
		}
		if len(themeHex) > 0 && themeHex[0] == '#' {
			themeHex = themeHex[1:]
		}
		windowBgXml := "#FF000000"
		if isURL {
			if len(themeHex) == 6 {
				windowBgXml = "#FF" + themeHex
			} else if len(themeHex) == 8 {
				windowBgXml = "#" + themeHex
			}
		}
		accentColor := util.SelectionAccentColor("#" + themeHex)
		valuesDir := filepath.Join(proj, "res", "values")
		os.MkdirAll(valuesDir, 0755)
		os.MkdirAll(flatDir, 0755)
		stylesPath := filepath.Join(valuesDir, "styles.xml")
		stylesXml := `<?xml version="1.0" encoding="utf-8"?>
<resources>
  <style name="AppTheme" parent="@android:style/Theme.NoTitleBar">
    <item name="android:windowBackground">` + windowBgXml + `</item>
    <item name="android:windowNoTitle">true</item>
    <item name="android:colorAccent">` + accentColor + `</item>
    <item name="android:colorControlActivated">` + accentColor + `</item>
  </style>
</resources>`
		os.WriteFile(stylesPath, []byte(stylesXml), 0644)
		out, err := exec.Command("aapt2", "compile", "-o", flatDir, stylesPath).CombinedOutput()
		logf("[aapt2 compile styles] %s", string(out))
		if err == nil {
			sf, _ := filepath.Glob(flatDir + "/values_styles.arsc.flat")
			flatFiles = append(flatFiles, sf...)
		}
	}

	// 2. manifest
	logf("Writing AndroidManifest.xml")
	util.WriteFile(filepath.Join(proj, "AndroidManifest.xml"), codegen.GenAndroidManifest(req, hasIcon))

	// 3. assets
	loadURL := "file:///android_asset/index.html"
	if isURL {
		loadURL = req.URL
	} else {
		logf("Writing web assets")
		util.WriteFile(filepath.Join(assets, "index.html"), codegen.WrapHTML(req))
		logf("AssetFiles count: %d", len(req.AssetFiles))
		for name, data := range req.AssetFiles {
			logf("Processing asset: %s", name)
			decoded, err := base64.StdEncoding.DecodeString(data)
			if err != nil {
				logf("Failed to decode %s: %v", name, err)
				continue
			}
			content := string(decoded)
			if strings.HasSuffix(strings.ToLower(name), ".png") {
				content = string(util.CompressPNG(decoded))
			} else {
				lname := strings.ToLower(name)
				if strings.HasSuffix(lname, ".html") || strings.HasSuffix(lname, ".htm") {
					content = codegen.InjectShims(content, req)
				}
			}
			destPath := filepath.Join(assets, filepath.Clean("/"+name))
			destDir := filepath.Dir(destPath)
			os.MkdirAll(destDir, 0755)
			logf("Writing %s to %s", name, destPath)
			util.WriteFile(destPath, content)
		}
	}

	// 4. Java source
	logf("Compiling Java source")
	srcDir := filepath.Join(work, "src", "com", "h2a")
	os.MkdirAll(srcDir, 0755)

	themeColorStr, themeColorInt := util.ParseThemeColor(req)

	util.WriteFile(filepath.Join(srcDir, "H2AChromeClient.java"), codegen.GenChromeClientSrc(req, themeColorInt))

	blockFlag := "false"
	if req.BlockAds || req.AdGuardDNS {
		blockFlag = "true"
	}
	indicatorField := ""
	pullInit := ""
	if req.PullRefresh {
		indicatorField = "private PullIndicator pullIndicator;"
		clientArg := "new PaddingClient(pl, " + blockFlag
		if req.AdGuardDNS {
			clientArg += ", true"
		}
		clientArg += ")"
		if !req.BlockAds && !req.AdGuardDNS {
			clientArg = "new PaddingClient(pl)"
		}
		pullInit = fmt.Sprintf(`pullIndicator = new PullIndicator(this, 0x%06X);
    int size = (int)(56 * getResources().getDisplayMetrics().density);
    FrameLayout.LayoutParams ilp = new FrameLayout.LayoutParams(size, size);
    ilp.gravity = Gravity.TOP | Gravity.CENTER_HORIZONTAL;
    ilp.topMargin = 0;
    pullIndicator.setLayoutParams(ilp);
    fl.addView(pullIndicator);
    PullListener pl = new PullListener(wv, pullIndicator);
    wv.setWebViewClient(%s);
    wv.setOnTouchListener(pl);`, themeColorInt&0xFFFFFF, clientArg)
	}

	clientCreate := "new PaddingClient()"
	if req.BlockAds || req.AdGuardDNS {
		if req.AdGuardDNS {
			clientCreate = "new PaddingClient(true, true)"
		} else {
			clientCreate = "new PaddingClient(true)"
		}
	}

	needsPerms := req.CameraPermission || req.MicPermission || req.GeoPermission
	useAssetLoader := !isURL && (needsPerms || len(req.AssetFiles) > 0)

	disableCopyMethod := `
  @Override
  public boolean onLongClick(android.view.View v) {
    WebView.HitTestResult r = ((WebView) v).getHitTestResult();
    return r != null && (r.getType() == WebView.HitTestResult.SRC_ANCHOR_TYPE || r.getType() == WebView.HitTestResult.SRC_IMAGE_ANCHOR_TYPE);
  }`
	if req.DisableCopyText {
		disableCopyMethod = `
  @Override
  public boolean onLongClick(android.view.View v) { return true; }`
	}

	permSettings := ""
	if needsPerms {
		permSettings = "ws.setMediaPlaybackRequiresUserGesture(false);"
	}

	permOnCreate := ""
	if req.NotifPermission {
		permOnCreate = `
    if (android.os.Build.VERSION.SDK_INT >= 33) {
      if (checkSelfPermission(android.Manifest.permission.POST_NOTIFICATIONS) != PackageManager.PERMISSION_GRANTED) {
        requestPermissions(new String[]{android.Manifest.permission.POST_NOTIFICATIONS}, 2001);
      }
    }`
	}

	notifInterface := ""
	if req.NotifPermission {
		notifInterface = `wv.addJavascriptInterface(new NotificationHelper(this), "H2A");`
	}

	assetInit := ""
	if useAssetLoader {
		loadURL = `file:///android_asset/index.html`
	}

	fileChooserMethods := `
  private String extToMime(String ext) {
    switch (ext.toLowerCase()) {
      case ".txt": return "text/plain";
      case ".pdf": return "application/pdf";
      case ".jpg": case ".jpeg": return "image/jpeg";
      case ".png": return "image/png";
      case ".gif": return "image/gif";
      case ".webp": return "image/webp";
      case ".mp4": return "video/mp4";
      case ".mp3": return "audio/mpeg";
      case ".json": return "application/json";
      case ".csv": return "text/csv";
      case ".zip": return "application/zip";
      case ".doc": return "application/msword";
      case ".docx": return "application/vnd.openxmlformats-officedocument.wordprocessingml.document";
      case ".xls": return "application/vnd.ms-excel";
      case ".xlsx": return "application/vnd.openxmlformats-officedocument.spreadsheetml.sheet";
      case ".htm": case ".html": return "text/html";
      default: return "*/*";
    }
  }

  public void launchFileChooser(android.webkit.ValueCallback<android.net.Uri[]> cb, String[] acceptTypes) {
    if (filePathCallback != null) { filePathCallback.onReceiveValue(null); }
    filePathCallback = cb;
    String mime = "*/*";
    if (acceptTypes != null && acceptTypes.length > 0) {
      for (String a : acceptTypes) {
        if (a == null) continue;
        String t = a.trim();
        if (t.length() == 0) continue;
        if (t.startsWith(".")) { mime = extToMime(t); break; }
        if (t.indexOf('/') >= 0 && t.indexOf(',') < 0) { mime = t; break; }
      }
    }
    android.content.Intent intent = new android.content.Intent(android.content.Intent.ACTION_OPEN_DOCUMENT);
    intent.addCategory(android.content.Intent.CATEGORY_OPENABLE);
    intent.setType(mime);
    intent.putExtra(android.content.Intent.EXTRA_ALLOW_MULTIPLE, true);
    try {
      startActivityForResult(intent, H2A_FILE_CHOOSER_REQ);
    } catch (Exception e) {
      if (filePathCallback != null) { filePathCallback.onReceiveValue(null); filePathCallback = null; }
    }
  }

  @Override
  protected void onActivityResult(int requestCode, int resultCode, android.content.Intent data) {
    super.onActivityResult(requestCode, resultCode, data);
    if (requestCode != H2A_FILE_CHOOSER_REQ) return;
    if (filePathCallback == null) return;
    android.net.Uri[] results = null;
    if (resultCode == Activity.RESULT_OK && data != null) {
      if (data.getClipData() != null) {
        int n = data.getClipData().getItemCount();
        results = new android.net.Uri[n];
        for (int i = 0; i < n; i++) results[i] = data.getClipData().getItemAt(i).getUri();
      } else if (data.getData() != null) {
        results = new android.net.Uri[]{ data.getData() };
      }
    }
    filePathCallback.onReceiveValue(results);
    filePathCallback = null;
  }`

	util.WriteFile(filepath.Join(srcDir, "ClipboardHelper.java"), codegen.GenClipboardHelperSrc())
	util.WriteFile(filepath.Join(srcDir, "FileHelper.java"), codegen.GenFileHelperSrc())
	util.WriteFile(filepath.Join(srcDir, "ShareHelper.java"), codegen.GenShareHelperSrc())
	util.WriteFile(filepath.Join(srcDir, "TTSHelper.java"), codegen.GenTTSHelperSrc())
	util.WriteFile(filepath.Join(srcDir, "PaddingClient.java"),
		codegen.GenPaddingClient(req.BlockAds||req.AdGuardDNS, req.AdGuardDNS, useAssetLoader))

	if req.NotifPermission {
		util.WriteFile(filepath.Join(srcDir, "NotificationHelper.java"), codegen.GenNotificationHelperSrc())
	}

	navBarInit := ""
	navBarFlags := "android.view.View.SYSTEM_UI_FLAG_LAYOUT_STABLE"
	navBarInsetFix := ""
	if req.TransparentNavBar {
		// Make the bottom navigation bar transparent and let the page draw
		// edge-to-edge underneath it. LAYOUT_HIDE_NAVIGATION lays the content
		// out behind the bottom nav bar WITHOUT hiding it (the status bar is
		// untouched — that would require LAYOUT_FULLSCREEN). The contrast scrim
		// that Android 10+ enforces over transparent bars is disabled so the
		// bar is truly see-through.
		navBarInit = "getWindow().setNavigationBarColor((int)0L);\n" +
			"      if (android.os.Build.VERSION.SDK_INT >= 29) { getWindow().setNavigationBarContrastEnforced(false); }"
		navBarFlags = "android.view.View.SYSTEM_UI_FLAG_LAYOUT_STABLE | android.view.View.SYSTEM_UI_FLAG_LAYOUT_HIDE_NAVIGATION"
		// Because the window now lays out edge-to-edge, pad ONLY the top by the
		// status-bar height so page content stays below the status bar (top
		// untouched) while still drawing under the transparent bottom nav bar
		// (bottom padding stays 0). Done statically (no inset listener) to keep
		// the generated class free of anonymous inner classes.
		navBarInsetFix = "int h2aSbh = 0; " +
			"int h2aSbId = getResources().getIdentifier(\"status_bar_height\", \"dimen\", \"android\"); " +
			"if (h2aSbId > 0) h2aSbh = getResources().getDimensionPixelSize(h2aSbId); " +
			"fl.setPadding(0, h2aSbh, 0, 0);"
	}

	params := codegen.WebViewActivityParams{
		PermImports:           "\nimport android.Manifest;\nimport android.content.pm.PackageManager;",
		DisableCopyImplements: ", android.view.View.OnLongClickListener",
		IndicatorField:        indicatorField,
		PermFields:            "",
		FlBg:                  themeColorStr,
		WebDebug:              req.BuildMode != "release",
		ThemeColorInt:         themeColorInt,
		HideScrollbars:        req.HideScrollbars,
		GeoPermission:         req.GeoPermission,
		PermSettings:          permSettings,
		ZoomEnabled:           req.ZoomEnabled,
		BlockAds:              req.BlockAds,
		AdGuardDNS:            req.AdGuardDNS,
		ClientCreate:          clientCreate,
		NotifInterface:        notifInterface,
		AssetInit:             assetInit,
		LoadURL:               loadURL,
		PullInit:              pullInit,
		PermOnCreate:          permOnCreate,
		DisableCopyInit:       "wv.setOnLongClickListener(this);",
		DisableCopyMethod:     disableCopyMethod,
		PermMethods: `
  @Override
  public void onRequestPermissionsResult(int requestCode, String[] perms, int[] grantResults) {
    boolean g = grantResults.length > 0 && grantResults[0] == PackageManager.PERMISSION_GRANTED;
    if (requestCode == 1003) {
      if (chromeClient != null) chromeClient.onGeoPermissionResult(g);
    } else if (requestCode == 1001) {
      if (chromeClient != null) chromeClient.onPermissionResult(requestCode, g);
    } else if (requestCode == 1002) {
      if (chromeClient != null) chromeClient.onPermissionResult(requestCode, g);
    }
  }

  public void reRequestPermission(String perm, int code) {
    requestPermissions(new String[]{perm}, code);
  }`,
		FileChooserMethods: fileChooserMethods,
		NavBarInit:         navBarInit,
		NavBarFlags:        navBarFlags,
		NavBarInsetFix:     navBarInsetFix,
	}
	util.WriteFile(filepath.Join(srcDir, "WebViewActivity.java"), codegen.GenWebViewActivitySrc(params))

	if req.SplashEnabled {
		util.WriteFile(filepath.Join(srcDir, "SplashActivity.java"), codegen.GenSplashActivitySrc(req))
	}

	if req.PullRefresh {
		util.WriteFile(filepath.Join(srcDir, "PullIndicator.java"), codegen.GenPullIndicatorSrc())
		util.WriteFile(filepath.Join(srcDir, "PullListener.java"), codegen.GenPullListenerSrc())
	}

	// 5. javac
	classesDir := filepath.Join(work, "classes")
	os.MkdirAll(classesDir, 0755)
	javacFiles := []string{
		filepath.Join(srcDir, "PaddingClient.java"),
		filepath.Join(srcDir, "WebViewActivity.java"),
		filepath.Join(srcDir, "H2AChromeClient.java"),
		filepath.Join(srcDir, "ClipboardHelper.java"),
		filepath.Join(srcDir, "FileHelper.java"),
		filepath.Join(srcDir, "ShareHelper.java"),
		filepath.Join(srcDir, "TTSHelper.java"),
	}
	if req.NotifPermission {
		javacFiles = append(javacFiles, filepath.Join(srcDir, "NotificationHelper.java"))
	}
	if req.SplashEnabled {
		javacFiles = append(javacFiles, filepath.Join(srcDir, "SplashActivity.java"))
	}
	if req.PullRefresh {
		javacFiles = append(javacFiles,
			filepath.Join(srcDir, "PullIndicator.java"),
			filepath.Join(srcDir, "PullListener.java"),
		)
	}
	run(rec, "javac", append([]string{
		"-source", "1.8", "-target", "1.8",
		"-Xlint:-options,-deprecation",
		"-cp", androidJar,
		"-d", classesDir,
	}, javacFiles...), logf)

	// 6. d8
	logf("Generating classes.dex")
	dexPath := filepath.Join(proj, "classes.dex")
	d8jar := b.Cfg.FindLocalOrSystem("tools/d8.jar", "d8_jar",
		filepath.Join(os.Getenv("ANDROID_HOME"), "build-tools", "34.0.0", "lib", "d8.jar"))
	classFiles, _ := filepath.Glob(filepath.Join(classesDir, "com", "h2a", "*.class"))
	d8Args := []string{"-Xmx512M", "-cp", d8jar, "com.android.tools.r8.D8",
		"--lib", androidJar,
		"--output", proj,
	}
	d8Args = append(d8Args, classFiles...)
	run(rec, "java", d8Args, logf)

	// Effective build parameters (with fallbacks).
	release := req.BuildMode == "release"
	versionCode := req.VersionCode
	if versionCode < 1 {
		versionCode = 1
	}
	minSDK := req.MinSDK
	if minSDK < 1 {
		minSDK = 21
	}
	targetSDK := req.TargetSDK
	if targetSDK < 1 {
		targetSDK = 34
	}

	// 7. aapt2 link
	logf("Packaging APK")
	unsigned := filepath.Join(work, "unsigned.apk")
	aaptArgs := []string{"link",
		"--manifest", filepath.Join(proj, "AndroidManifest.xml"),
		"--version-code", strconv.Itoa(versionCode),
		"--version-name", util.VersionName(req.VersionName),
		"--min-sdk-version", strconv.Itoa(minSDK),
		"--target-sdk-version", strconv.Itoa(targetSDK),
		"--auto-add-overlay",
		"-o", unsigned,
	}
	for _, f := range flatFiles {
		aaptArgs = append(aaptArgs, "-R", f)
	}
	if !isURL {
		aaptArgs = append(aaptArgs, "-A", assets)
	}
	if androidJar != "" {
		aaptArgs = append(aaptArgs, "-I", androidJar)
	}
	run(rec, "aapt2", aaptArgs, logf)

	// 8. add dex
	logf("Adding classes.dex")
	run(rec, "zip", []string{"-j", unsigned, dexPath}, logf)

	// 9. zipalign
	logf("Aligning APK")
	aligned := filepath.Join(work, "aligned.apk")
	run(rec, "zipalign", []string{"-p", "4", unsigned, aligned}, logf)

	// 10. sign
	logf("Signing APK")
	ks := b.getKeystore()
	ksPass := "pass:h2ah2a"
	keyAlias := "h2a"
	keyPass := "pass:h2ah2a"
	if release {
		logf("Using custom keystore for release signing")
		ksData, err := base64.StdEncoding.DecodeString(req.KeystoreBase64)
		if err != nil {
			fail(rec, "failed to decode keystore: "+err.Error())
		}
		ks = filepath.Join(work, "custom.keystore")
		os.WriteFile(ks, ksData, 0644)
		ksPass = "pass:" + req.KeystorePass
		keyAlias = req.KeyAlias
		if req.KeyPass != "" {
			keyPass = "pass:" + req.KeyPass
		} else {
			keyPass = ksPass
		}
	} else if req.KeystoreBase64 != "" {
		logf("Using custom keystore")
		if req.KeystorePass == "" {
			fail(rec, "keystore password is required for release signing")
		}
		if req.KeyAlias == "" {
			fail(rec, "key alias is required for release signing")
		}
		ksData, err := base64.StdEncoding.DecodeString(req.KeystoreBase64)
		if err != nil {
			fail(rec, "failed to decode keystore: "+err.Error())
		}
		ks = filepath.Join(work, "custom.keystore")
		os.WriteFile(ks, ksData, 0644)
		ksPass = "pass:" + req.KeystorePass
		keyAlias = req.KeyAlias
		if req.KeyPass != "" {
			keyPass = "pass:" + req.KeyPass
		} else {
			keyPass = ksPass
		}
	}
	signed := filepath.Join(work, "signed.apk")
	apkSignerJar := b.Cfg.FindLocalOrSystem("tools/apksigner.jar", "apksigner_jar",
		"/data/data/com.termux/files/usr/share/java/apksigner.jar")
	run(rec, "java", []string{"-jar", apkSignerJar, "sign",
		"--ks", ks, "--ks-pass", ksPass, "--ks-key-alias", keyAlias,
		"--key-pass", keyPass, "--out", signed, aligned,
	}, logf)

	// 11. finalize
	safeName := util.SafeName(req.AppName)
	if release {
		// Release: produce a release-signed APK (for sideload testing) and an AAB (for Play upload).
		apkName := safeName + "-release.apk"
		util.CopyFile(filepath.Join(b.BaseDir, "output", apkName), signed)

		// Build the AAB.
		aabName := safeName + ".aab"
		bundletoolJar := b.Cfg.FindLocalOrSystem("tools/bundletool.jar", "bundletool_jar", "")
		buildAAB(b.BaseDir, work, proj, assets, androidJar, flatFiles, dexPath,
			req, versionCode, minSDK, targetSDK, ks, ksPass, keyAlias, keyPass,
			bundletoolJar, aabName, rec, logf)

		rec.Status = "done"
		rec.APKName = aabName
		rec.Artifacts = []types.Artifact{
			{Name: aabName, Kind: "aab"},
			{Name: apkName, Kind: "apk"},
		}
		logf("Done: %s + %s", aabName, apkName)
	} else {
		final := safeName + ".apk"
		util.CopyFile(filepath.Join(b.BaseDir, "output", final), signed)
		rec.Status = "done"
		rec.APKName = final
		rec.Artifacts = []types.Artifact{{Name: final, Kind: "apk"}}
		logf("Done: %s", final)
	}
}

func run(rec *Record, name string, args []string, logf func(string, ...interface{})) {
	cmd := exec.Command(name, args...)
	out, err := cmd.CombinedOutput()
	logf("[%s] %s", name, string(out))
	if err != nil {
		fail(rec, name+" failed: "+err.Error()+"\n"+string(out))
	}
}

func fail(rec *Record, msg string) {
	rec.Status = "failed"
	rec.Err = msg
	panic("build-fail")
}

func (b *Builder) getKeystore() string {
	b.ksOnce.Do(func() {
		b.ksPath = filepath.Join(b.BaseDir, "tmp", "debug.keystore")
		os.MkdirAll(filepath.Dir(b.ksPath), 0755)
		os.WriteFile(b.ksPath, assets.Keystore, 0644)
	})
	return b.ksPath
}
