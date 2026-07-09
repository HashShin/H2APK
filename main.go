package main

import (
	"bufio"
	"bytes"
	"crypto/rand"
	_ "embed"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"image/png"
	"io"
	"log"
	"net"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"time"
)

//go:embed keystore/debug.keystore
var embeddedKS []byte

//go:embed assets/static/index.html
var indexHTML string

//go:embed assets/images/instapay.png
var instapayPNG []byte

//go:embed assets/images/binance.png
var binancePNG []byte

type Config struct {
	D8Jar        string `json:"d8_jar"`
	ApkSignerJar string `json:"apksigner_jar"`
	AndroidJar   string `json:"android_jar"`
}

var appConfig Config
var configLoaded bool

func loadConfig() Config {
	if configLoaded {
		return appConfig
	}
	configLoaded = true
	data, err := os.ReadFile(filepath.Join(baseDir, "config.json"))
	if err != nil {
		return appConfig
	}
	json.Unmarshal(data, &appConfig)
	return appConfig
}

// -- request / response types --

type BuildRequest struct {
	AppName     string `json:"app_name"`
	PackageName string `json:"package_name"`
	HTML        string `json:"html"`
	CSS         string `json:"css"`
	JS          string `json:"js"`
	URL         string `json:"url"`
	Icon        string `json:"icon"`         // base64-encoded PNG
	PullRefresh       bool   `json:"pull_refresh"`
	ThemeColor        string `json:"theme_color"`
	VersionCode       string `json:"version"`
	TransparentNavBar bool   `json:"transparent_nav"`
	BlockAds          bool   `json:"block_ads"`
	AdGuardDNS        bool   `json:"adguard_dns"`
	ZoomEnabled       bool   `json:"zoom_enabled"`
	SplashEnabled     bool   `json:"splash_enabled"`
	SplashDuration    int    `json:"splash_duration"`
	SplashColor       string `json:"splash_color"`
	SplashImage       string `json:"splash_image"`
	SplashUseIcon     bool   `json:"splash_use_icon"`
	SplashImageSize   int    `json:"splash_image_size"`
	SplashAnimation   string `json:"splash_animation"`
	DisableCopyText   bool   `json:"disable_copy_text"`
	HideScrollbars    bool   `json:"hide_scrollbars"`
	CameraPermission  bool   `json:"-"`
	MicPermission     bool   `json:"-"`
	NotifPermission   bool   `json:"-"`
	KeystoreBase64    string `json:"keystore"`
	KeystorePass      string `json:"ks_pass"`
	KeyAlias          string `json:"key_alias"`
	KeyPass           string `json:"key_pass"`
	AssetFiles        map[string]string `json:"asset_files"` // filename -> base64
}

type BuildInfo struct {
	Success bool   `json:"success"`
	BuildID string `json:"build_id,omitempty"`
	APKName string `json:"apk_name,omitempty"`
	Error   string `json:"error,omitempty"`
	Log     string `json:"log,omitempty"`
}

type record struct {
	status  string // building | done | failed
	apkName string
	err     string
	log     string
	logCh   chan string
}

var (
	builds   = map[string]*record{}
	buildsMu sync.RWMutex
	baseDir  string
	ksOnce   sync.Once
	ksPath   string
)

func main() {
	baseDir, _ = os.Getwd()

	checkDeps()

	os.MkdirAll(filepath.Join(baseDir, "output"), 0755)
	cleanOldBuilds()
	go func() {
		for range time.Tick(1 * time.Hour) {
			cleanOldBuilds()
		}
	}()

	http.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/" {
			w.Header().Set("Content-Type", "text/html; charset=utf-8")
			io.WriteString(w, indexHTML)
			return
		}
		http.NotFound(w, r)
	})
	http.HandleFunc("/api/build", handleBuild)
	http.HandleFunc("/api/status/", handleStatus)
	http.HandleFunc("/api/download/", handleDownload)
	http.HandleFunc("/api/log/", handleLogStream)
	http.HandleFunc("/instapay.png", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "image/png")
		w.Write(instapayPNG)
	})
	http.HandleFunc("/binance.png", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "image/png")
		w.Write(binancePNG)
	})

	port := env("PORT", "")
	envPort := readEnvPort()
	if envPort != "" {
		port = envPort
	}
	if port == "" {
		port = "8080"
	}

	scanner := bufio.NewScanner(os.Stdin)
	for {
		listener, err := net.Listen("tcp", "0.0.0.0:"+port)
		if err != nil && strings.Contains(err.Error(), "address already in use") {
			fmt.Printf("Port %s is in use. Enter another port: ", port)
			scanner.Scan()
			port = strings.TrimSpace(scanner.Text())
			if port == "" {
				port = "8080"
			}
			saveEnvPort(port)
			continue
		}
		if err != nil {
			log.Fatal(err)
		}
		fmt.Printf("  H2APK — HTML/URL to APK\n")
		fmt.Printf("  http://localhost:%s\n", port)
		if ip := localIP(); ip != "" {
			fmt.Printf("  http://%s:%s\n", ip, port)
		}
		fmt.Println()
		saveEnvPort(port)
		log.Fatal(http.Serve(listener, nil))
	}
}

func cleanOldBuilds() {
	cutoff := time.Now().Add(-24 * time.Hour)
	outputDir := filepath.Join(baseDir, "output")
	entries, err := os.ReadDir(outputDir)
	if err != nil {
		return
	}
	for _, e := range entries {
		info, err := e.Info()
		if err != nil {
			continue
		}
		if info.ModTime().Before(cutoff) {
			path := filepath.Join(outputDir, e.Name())
			os.RemoveAll(path)
			fmt.Printf("  cleaned: %s\n", e.Name())
		}
	}
}

func checkDeps() {
	type dep struct {
		name string
		path string
		ok   bool
	}
	deps := []dep{
		{name: "javac", path: ""},
		{name: "java", path: ""},
		{name: "aapt2", path: ""},
		{name: "zipalign", path: ""},
		{name: "zip", path: ""},
	}
	for i, d := range deps {
		p, err := exec.LookPath(d.name)
		if err == nil {
			deps[i].ok = true
			deps[i].path = p
		}
	}

	jarDeps := []dep{
		{name: "d8.jar", path: findLocalOrSystem("tools/d8.jar", "d8_jar", "/data/data/com.termux/files/usr/share/java/d8.jar")},
		{name: "apksigner.jar", path: findLocalOrSystem("tools/apksigner.jar", "apksigner_jar", "/data/data/com.termux/files/usr/share/java/apksigner.jar")},
	}
	for i, d := range jarDeps {
		if _, err := os.Stat(d.path); err == nil {
			jarDeps[i].ok = true
		}
	}

	androidJar := findAndroidJar()

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
		fmt.Println("  All dependencies found.\n")
	}
}

// -- handlers --

func handleBuild(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "POST only", http.StatusMethodNotAllowed)
		return
	}
	var req BuildRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSON(w, 400, BuildInfo{Success: false, Error: "bad request: " + err.Error()})
		return
	}
	log.Printf("Build request: zoom=%t blockAds=%t adGuard=%t url=%q", req.ZoomEnabled, req.BlockAds, req.AdGuardDNS, req.URL)
	if strings.TrimSpace(req.HTML) == "" && strings.TrimSpace(req.URL) == "" {
		writeJSON(w, 400, BuildInfo{Success: false, Error: "HTML or URL is required"})
		return
	}
	isURL := strings.TrimSpace(req.URL) != ""
	if isURL && !strings.HasPrefix(req.URL, "http://") && !strings.HasPrefix(req.URL, "https://") {
		req.URL = "https://" + req.URL
	}
	if req.AppName == "" {
		req.AppName = "MyApp"
	}
	if req.PackageName == "" {
		req.PackageName = "com.h2a.app"
	}
	req.PackageName = cleanPkg(req.PackageName)
	if !strings.Contains(req.PackageName, ".") {
		http.Error(w, "invalid package name: must contain at least one dot (e.g. com.example.app)", 400)
		return
	}

	id := newID()
	buildsMu.Lock()
	builds[id] = &record{status: "building", logCh: make(chan string, 50)}
	buildsMu.Unlock()

	// Auto-detect camera/mic/notification needs from content
	if isURL {
		// URL mode: can't scan remote content — enable all, harmless if unused
		req.CameraPermission = true
		req.MicPermission = true
		req.NotifPermission = true
	} else {
		content := strings.ToLower(req.HTML + req.CSS + req.JS)
		for _, data := range req.AssetFiles {
			content += strings.ToLower(data)
		}
		if strings.Contains(content, "getusermedia") {
			req.CameraPermission = strings.Contains(content, "{video") || strings.Contains(content, "video:") || strings.Contains(content, "video :")
			req.MicPermission = strings.Contains(content, "{audio") || strings.Contains(content, "audio:") || strings.Contains(content, "audio :")
		}
		// Detect Notification API usage (JS bridge or Web Notification API)
		req.NotifPermission = strings.Contains(content, "notification.requestpermission") ||
			strings.Contains(content, "new notification(") ||
			strings.Contains(content, "notification.permission") ||
			strings.Contains(content, "h2a.shownotification") ||
			strings.Contains(content, "h2a.requestnotificationpermission") ||
			strings.Contains(content, "h2a.getnotificationpermission")
	}
	log.Printf("Auto-detect: camera=%t mic=%t notif=%t isURL=%t", req.CameraPermission, req.MicPermission, req.NotifPermission, isURL)
	go doBuild(id, req, isURL)
	writeJSON(w, 202, BuildInfo{Success: true, BuildID: id})
}

func handleStatus(w http.ResponseWriter, r *http.Request) {
	id := strings.TrimPrefix(r.URL.Path, "/api/status/")
	buildsMu.RLock()
	rec, ok := builds[id]
	buildsMu.RUnlock()
	if !ok {
		writeJSON(w, 404, BuildInfo{Success: false, Error: "not found"})
		return
	}
	writeJSON(w, 200, BuildInfo{
		Success: rec.status == "done",
		BuildID: id,
		APKName: rec.apkName,
		Error:   rec.err,
		Log:     rec.log,
	})
}

func handleLogStream(w http.ResponseWriter, r *http.Request) {
	id := strings.TrimPrefix(r.URL.Path, "/api/log/")
	buildsMu.RLock()
	rec, ok := builds[id]
	buildsMu.RUnlock()
	if !ok {
		http.NotFound(w, r)
		return
	}

	flusher, ok := w.(http.Flusher)
	if !ok {
		http.Error(w, "streaming not supported", 500)
		return
	}

	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")

	// Send already-logged lines first
	for _, line := range strings.Split(rec.log, "\n") {
		if line != "" {
			fmt.Fprintf(w, "data: %s\n\n", line)
			flusher.Flush()
		}
	}

	// Stream new lines
	for line := range rec.logCh {
		fmt.Fprintf(w, "data: %s\n\n", line)
		flusher.Flush()
	}

	// Send completion marker
	if rec.status == "done" {
		fmt.Fprintf(w, "event: done\ndata: %s\n\n", rec.apkName)
	} else {
		fmt.Fprintf(w, "event: failed\ndata: %s\n\n", rec.err)
	}
	flusher.Flush()
}

func handleDownload(w http.ResponseWriter, r *http.Request) {
	id := strings.TrimPrefix(r.URL.Path, "/api/download/")
	buildsMu.RLock()
	rec, ok := builds[id]
	buildsMu.RUnlock()
	if !ok || rec.status != "done" {
		http.Error(w, "not ready", http.StatusNotFound)
		return
	}
	p := filepath.Join(baseDir, "output", rec.apkName)
	if _, err := os.Stat(p); err != nil {
		http.Error(w, "file missing", http.StatusNotFound)
		return
	}
	w.Header().Set("Content-Type", "application/vnd.android.package-archive")
	w.Header().Set("Content-Disposition", fmt.Sprintf(`attachment; filename="%s"`, rec.apkName))
	http.ServeFile(w, r, p)
}

// -- build pipeline --

func doBuild(id string, req BuildRequest, isURL bool) {
	rec := getRec(id)
	work := filepath.Join(baseDir, "output", "build_"+id)
	logs := &strings.Builder{}
	logf := func(f string, a ...interface{}) {
		s := fmt.Sprintf(f, a...)
		logs.WriteString(s + "\n")
		fmt.Println("[build]", s)
		select {
		case rec.logCh <- s:
		default:
		}
	}

	defer func() {
		if r := recover(); r != nil {
			if r != "build-fail" {
				rec.status = "failed"
				rec.err = fmt.Sprintf("panic: %v", r)
			}
		}
		rec.log = logs.String()
		close(rec.logCh)
		os.RemoveAll(work)
	}()

	os.MkdirAll(work, 0755)
	proj := filepath.Join(work, "project")
	assets := filepath.Join(proj, "assets")
	os.MkdirAll(assets, 0755)

	androidJar := findAndroidJar()

	// 1. icon (must process before manifest since it affects the icon attribute)
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
			os.WriteFile(iconPath, compressPNG(iconData), 0644)

			os.MkdirAll(flatDir, 0755)
			out, err := exec.Command("aapt2", "compile", "-o", flatDir, iconPath).CombinedOutput()
			logf("[aapt2 compile] %s", string(out))
			if err != nil {
				logf("Icon compile failed, skipping: %v", err)
			} else {
				flatFiles, _ = filepath.Glob(flatDir + "/*.flat")
				hasIcon = true
				// Also copy to assets for NotificationHelper
				os.WriteFile(filepath.Join(assets, "icon.png"), compressPNG(iconData), 0644)
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

	// 2. manifest
	logf("Writing AndroidManifest.xml")
	iconAttr := ""
	if hasIcon {
		iconAttr = ` android:icon="@mipmap/icon"`
	}
	var splashActivity string
	if req.SplashEnabled {
		splashActivity = `    <activity android:name="com.h2a.SplashActivity" android:exported="true" android:theme="@android:style/Theme.NoTitleBar">
      <intent-filter>
        <action android:name="android.intent.action.MAIN"/>
        <category android:name="android.intent.category.LAUNCHER"/>
      </intent-filter>
    </activity>
    <activity android:name="com.h2a.WebViewActivity" android:exported="false" android:theme="@android:style/Theme.NoTitleBar" android:configChanges="orientation|screenSize">
    </activity>`
	} else {
		splashActivity = `    <activity android:name="com.h2a.WebViewActivity" android:exported="true" android:theme="@android:style/Theme.NoTitleBar" android:configChanges="orientation|screenSize">
      <intent-filter>
        <action android:name="android.intent.action.MAIN"/>
        <category android:name="android.intent.category.LAUNCHER"/>
      </intent-filter>
    </activity>`
	}
	m := fmt.Sprintf(`<?xml version="1.0" encoding="utf-8"?>
<manifest xmlns:android="http://schemas.android.com/apk/res/android" package="%s">
  <uses-permission android:name="android.permission.INTERNET"/>
  <uses-permission android:name="android.permission.WRITE_EXTERNAL_STORAGE" android:maxSdkVersion="28"/>`, req.PackageName)
	if req.CameraPermission {
		m += "\n  <uses-permission android:name=\"android.permission.CAMERA\"/>"
	}
	if req.MicPermission {
		m += "\n  <uses-permission android:name=\"android.permission.RECORD_AUDIO\"/>"
		m += "\n  <uses-permission android:name=\"android.permission.MODIFY_AUDIO_SETTINGS\"/>"
	}
	if req.NotifPermission {
		m += "\n  <uses-permission android:name=\"android.permission.POST_NOTIFICATIONS\"/>"
	}
	m += fmt.Sprintf(`
  <application android:label="%s"%s android:usesCleartextTraffic="true">
%s
  </application>
</manifest>`, xmlEscape(req.AppName), iconAttr, splashActivity)
	writeFile(filepath.Join(proj, "AndroidManifest.xml"), m)

	// 3. assets (HTML mode only)
	loadURL := "file:///android_asset/index.html"
	if isURL {
		loadURL = req.URL
	} else {
		logf("Writing web assets")
		writeFile(filepath.Join(assets, "index.html"), wrapHTML(req))
		for name, data := range req.AssetFiles {
			decoded, err := base64.StdEncoding.DecodeString(data)
			if err == nil {
				content := string(decoded)
				// Optimize PNG images
				if strings.HasSuffix(strings.ToLower(name), ".png") {
					content = string(compressPNG(decoded))
				}
				// Preserve folder structure for zip uploads
				destPath := filepath.Join(assets, filepath.Clean("/"+name))
				// Create parent directories if needed
				destDir := filepath.Dir(destPath)
				os.MkdirAll(destDir, 0755)
				writeFile(destPath, content)
			}
		}
	}

	// 4. Java source for WebView activity
	logf("Compiling Java source")
	srcDir := filepath.Join(work, "src", "com", "h2a")
	os.MkdirAll(srcDir, 0755)
	// generate PaddingClient after we know isURL/needsPerms
	// (moved later — see below before writeFile WebViewActivity)
	chromePermCode := ""
	if req.CameraPermission || req.MicPermission {
		camFlag := "false"
		if req.CameraPermission { camFlag = "true" }
		micFlag := "false"
		if req.MicPermission { micFlag = "true" }
		chromePermCode = fmt.Sprintf(`
import android.webkit.PermissionRequest;

public class H2AChromeClient extends WebChromeClient {
  private View customView;
  private CustomViewCallback callback;
  private FrameLayout container;
  private WebView webView;
  private PermissionRequest pendingPermission;
  private WebViewActivity activity;
  private boolean cameraEnabled = %s;
  private boolean micEnabled = %s;
  private static final int REQ_CAMERA = 1001;
  private static final int REQ_MIC = 1002;

  public H2AChromeClient(FrameLayout container, WebView webView) {
    this.container = container;
    this.webView = webView;
    this.activity = (WebViewActivity) webView.getContext();
  }

  @Override
  public boolean onCreateWindow(WebView view, boolean isDialog, boolean isUserGesture, android.os.Message resultMsg) {
    return false;
  }

  @Override
  public void onShowCustomView(View view, CustomViewCallback callback) {
    if (this.customView != null) {
      callback.onCustomViewHidden();
      return;
    }
    this.customView = view;
    this.callback = callback;
    webView.setVisibility(View.GONE);
    container.addView(view, new FrameLayout.LayoutParams(
      ViewGroup.LayoutParams.MATCH_PARENT,
      ViewGroup.LayoutParams.MATCH_PARENT
    ));
  }

  @Override
  public void onHideCustomView() {
    if (customView == null) return;
    webView.setVisibility(View.VISIBLE);
    container.removeView(customView);
    customView = null;
    if (callback != null) {
      callback.onCustomViewHidden();
      callback = null;
    }
  }

  public boolean dismissCustomView() {
    if (customView == null) return false;
    onHideCustomView();
    return true;
  }

  @Override
  public void onPermissionRequest(PermissionRequest request) {
    boolean needCam = false, needMic = false;
    for (String r : request.getResources()) {
      if (r.equals(PermissionRequest.RESOURCE_VIDEO_CAPTURE)) needCam = true;
      if (r.equals(PermissionRequest.RESOURCE_AUDIO_CAPTURE)) needMic = true;
    }
    boolean camOK = true, micOK = true;
    if (android.os.Build.VERSION.SDK_INT >= 23) {
      if (needCam && cameraEnabled) {
        camOK = activity.checkSelfPermission(android.Manifest.permission.CAMERA) == android.content.pm.PackageManager.PERMISSION_GRANTED;
      }
      if (needMic && micEnabled) {
        micOK = activity.checkSelfPermission(android.Manifest.permission.RECORD_AUDIO) == android.content.pm.PackageManager.PERMISSION_GRANTED;
      }
    }
    if (camOK && micOK) {
      request.grant(request.getResources());
      return;
    }
    pendingPermission = request;
    if (android.os.Build.VERSION.SDK_INT >= 23) {
      if (needCam && !camOK) activity.reRequestPermission(android.Manifest.permission.CAMERA, REQ_CAMERA);
      if (needMic && !micOK) activity.reRequestPermission(android.Manifest.permission.RECORD_AUDIO, REQ_MIC);
    } else {
      request.deny();
      pendingPermission = null;
    }
  }

  public void onPermissionResult(int requestCode, boolean granted) {
    if (pendingPermission == null) return;
    if (granted) {
      pendingPermission.grant(pendingPermission.getResources());
    } else {
      pendingPermission.deny();
    }
    pendingPermission = null;
  }
}`, camFlag, micFlag)
	} else {
		chromePermCode = `
public class H2AChromeClient extends WebChromeClient {
  private View customView;
  private CustomViewCallback callback;
  private FrameLayout container;
  private WebView webView;

  public H2AChromeClient(FrameLayout container, WebView webView) {
    this.container = container;
    this.webView = webView;
  }

  @Override
  public boolean onCreateWindow(WebView view, boolean isDialog, boolean isUserGesture, android.os.Message resultMsg) {
    return false;
  }

  @Override
  public void onShowCustomView(View view, CustomViewCallback callback) {
    if (this.customView != null) {
      callback.onCustomViewHidden();
      return;
    }
    this.customView = view;
    this.callback = callback;
    webView.setVisibility(View.GONE);
    container.addView(view, new FrameLayout.LayoutParams(
      ViewGroup.LayoutParams.MATCH_PARENT,
      ViewGroup.LayoutParams.MATCH_PARENT
    ));
  }

  @Override
  public void onHideCustomView() {
    if (customView == null) return;
    webView.setVisibility(View.VISIBLE);
    container.removeView(customView);
    customView = null;
    if (callback != null) {
      callback.onCustomViewHidden();
      callback = null;
    }
  }

  public boolean dismissCustomView() {
    if (customView == null) return false;
    onHideCustomView();
    return true;
  }
}`
	}
	writeFile(filepath.Join(srcDir, "H2AChromeClient.java"),
		`package com.h2a;
import android.webkit.WebChromeClient;
import android.webkit.WebView;
import android.view.View;
import android.view.ViewGroup;
import android.widget.FrameLayout;`+chromePermCode)
	indicatorField := ""
	pullInit := ""
	themeColorStr, themeColorInt := parseThemeColor(req)
	flBg := themeColorStr
	if !isURL {
		flBg = "0xFF000000"
	}
	blockFlag := "false"
	if req.BlockAds || req.AdGuardDNS { blockFlag = "true" }
	if req.PullRefresh {
		indicatorField = "private PullIndicator pullIndicator;"
		clientArg := "new PaddingClient(pl, " + blockFlag
		if req.AdGuardDNS { clientArg += ", true" }
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
	needsPerms := req.CameraPermission || req.MicPermission
	useAssetLoader := !isURL && needsPerms
	disableCopyImplements := ", android.view.View.OnLongClickListener"
	disableCopyInit := "wv.setOnLongClickListener(this);"
	disableCopyMethod := ""
	if req.DisableCopyText {
		disableCopyMethod = `
  @Override
  public boolean onLongClick(android.view.View v) { return true; }`
	} else {
		disableCopyMethod = `
  @Override
  public boolean onLongClick(android.view.View v) {
    WebView.HitTestResult r = ((WebView) v).getHitTestResult();
    return r != null && (r.getType() == WebView.HitTestResult.SRC_ANCHOR_TYPE || r.getType() == WebView.HitTestResult.SRC_IMAGE_ANCHOR_TYPE);
  }`
	}

	permImports := ""
	permSettings := ""
	permFields := ""
	permOnCreate := ""
	permMethods := ""
	if needsPerms {
		permImports = "\nimport android.Manifest;\nimport android.content.pm.PackageManager;"
		permSettings = "ws.setMediaPlaybackRequiresUserGesture(false);"
		permFields = ""
		// asset serving is handled entirely in PaddingClient.shouldInterceptRequest
		permMethods = `
  @Override
  public void onRequestPermissionsResult(int requestCode, String[] perms, int[] grantResults) {
    boolean g = grantResults.length > 0 && grantResults[0] == PackageManager.PERMISSION_GRANTED;
    if (chromeClient != null) chromeClient.onPermissionResult(requestCode, g);
  }

  public void reRequestPermission(String perm, int code) {
    requestPermissions(new String[]{perm}, code);
  }`
	}
	if req.NotifPermission {
		if permImports == "" {
			permImports = "\nimport android.Manifest;\nimport android.content.pm.PackageManager;"
		}
		permOnCreate += `
    if (android.os.Build.VERSION.SDK_INT >= 33) {
      if (checkSelfPermission(android.Manifest.permission.POST_NOTIFICATIONS) != PackageManager.PERMISSION_GRANTED) {
        requestPermissions(new String[]{android.Manifest.permission.POST_NOTIFICATIONS}, 2001);
      }
    }`
		if permMethods == "" {
			permMethods = `
  @Override
  public void onRequestPermissionsResult(int requestCode, String[] perms, int[] grantResults) {
  }`
		}
	}

	notifInterface := ""
	if req.NotifPermission {
		notifInterface = "wv.addJavascriptInterface(new NotificationHelper(this), \"H2A\");"
	}

	writeFile(filepath.Join(srcDir, "PaddingClient.java"),
		genPaddingClient(req.BlockAds || req.AdGuardDNS, req.AdGuardDNS, useAssetLoader))

	if req.NotifPermission {
		writeFile(filepath.Join(srcDir, "NotificationHelper.java"),
			`package com.h2a;
import android.app.Activity;
import android.app.Notification;
import android.app.NotificationChannel;
import android.app.NotificationManager;
import android.os.Build;
import android.webkit.JavascriptInterface;

public class NotificationHelper {
  private Activity activity;
  private android.graphics.Bitmap appIcon;
  private int iconResId;

  public NotificationHelper(Activity a) {
    this.activity = a;
    iconResId = a.getResources().getIdentifier("icon", "mipmap", a.getPackageName());
    if (iconResId == 0) iconResId = android.R.drawable.ic_dialog_info;
    try {
      java.io.InputStream is = a.getAssets().open("icon.png");
      appIcon = android.graphics.BitmapFactory.decodeStream(is);
      is.close();
    } catch (Exception e) {
      appIcon = null;
    }
    if (Build.VERSION.SDK_INT >= 26) {
      NotificationChannel ch = new NotificationChannel(
        "h2a_notifs", "Notifications", NotificationManager.IMPORTANCE_DEFAULT);
      activity.getSystemService(NotificationManager.class).createNotificationChannel(ch);
    }
  }

  @JavascriptInterface
  public void showNotification(String title, String body) {
    if (Build.VERSION.SDK_INT >= 33 &&
        activity.checkSelfPermission(android.Manifest.permission.POST_NOTIFICATIONS)
        != android.content.pm.PackageManager.PERMISSION_GRANTED) return;
    Notification.Builder b = new Notification.Builder(activity, "h2a_notifs")
      .setContentTitle(title)
      .setContentText(body)
      .setSmallIcon(iconResId)
      .setAutoCancel(true);
    if (appIcon != null) b.setLargeIcon(appIcon);
    activity.getSystemService(NotificationManager.class)
      .notify((int)(System.currentTimeMillis() % Integer.MAX_VALUE), b.build());
  }

  @JavascriptInterface
  public String getNotificationPermission() {
    if (Build.VERSION.SDK_INT >= 33) {
      return activity.checkSelfPermission(android.Manifest.permission.POST_NOTIFICATIONS)
        == android.content.pm.PackageManager.PERMISSION_GRANTED ? "granted" : "denied";
    }
    return "granted";
  }

  @JavascriptInterface
  public void requestNotificationPermission() {
    if (Build.VERSION.SDK_INT >= 33) {
      activity.requestPermissions(
        new String[]{android.Manifest.permission.POST_NOTIFICATIONS}, 2001);
    }
  }
}`)
	}

	assetInit := ""
	if useAssetLoader {
		loadURL = `https://appassets.androidplatform.net/index.html`
	}

	writeFile(filepath.Join(srcDir, "WebViewActivity.java"),
		fmt.Sprintf(`package com.h2a;
import android.app.Activity;
import android.os.Bundle;
import android.os.Environment;
import android.webkit.WebView;
import android.webkit.WebSettings;
import android.webkit.DownloadListener;
import android.webkit.URLUtil;
import android.app.DownloadManager;
import android.net.Uri;
import android.content.Context;
import android.widget.Toast;
import android.widget.TextView;
import android.view.Gravity;
import android.graphics.drawable.GradientDrawable;
import android.graphics.Typeface;
import android.view.MotionEvent;
import android.view.WindowManager;
import android.widget.FrameLayout;
import android.content.res.Configuration;
import android.util.Log;%s
public class WebViewActivity extends Activity implements DownloadListener%s {
  private WebView wv;
  private H2AChromeClient chromeClient;
  private long lastBackPress = 0;
  private Toast toast;
  %s
  %s

  @Override
  protected void onCreate(Bundle savedInstanceState) {
    super.onCreate(savedInstanceState);
    try { WebView.setWebContentsDebuggingEnabled(true); } catch (Exception ignored) {}
    if (android.os.Build.VERSION.SDK_INT >= 21) {
      getWindow().addFlags(android.view.WindowManager.LayoutParams.FLAG_DRAWS_SYSTEM_BAR_BACKGROUNDS);
    }
    wv = new WebView(this);
    wv.setVerticalScrollBarEnabled(%t);
    wv.setHorizontalScrollBarEnabled(%t);
    WebSettings ws = wv.getSettings();
    ws.setJavaScriptEnabled(true);
    ws.setDomStorageEnabled(true);
    ws.setSupportMultipleWindows(false);
    %s
    if (%t) {
      ws.setUseWideViewPort(true);
      ws.setLoadWithOverviewMode(true);
      ws.setSupportZoom(true);
      ws.setBuiltInZoomControls(true);
      ws.setDisplayZoomControls(false);
      android.util.Log.d("H2A", "Zoom enabled: supportZoom=true, builtInZoom=true, wideViewPort=true");
    } else {
      android.util.Log.d("H2A", "Zoom disabled");
    }
    if (%t || %t) {
      android.webkit.CookieManager.getInstance().setAcceptThirdPartyCookies(wv, false);
      ws.setSavePassword(false);
      ws.setGeolocationEnabled(false);
    }
    FrameLayout fl = new FrameLayout(this);
    wv.setWebViewClient(%s);
    chromeClient = new H2AChromeClient(fl, wv);
    wv.setWebChromeClient(chromeClient);
    wv.setDownloadListener(this);
    %s
    %s
    wv.loadUrl("%s");
    fl.setBackgroundColor(%s);
    fl.addView(wv);
    %s
    %s
    %s
    setContentView(fl);
    applySystemUI();
  }

  private void applySystemUI() {
    if (android.os.Build.VERSION.SDK_INT < 19) return;
    int flags = android.view.View.SYSTEM_UI_FLAG_LAYOUT_STABLE;
    if (getResources().getConfiguration().orientation == Configuration.ORIENTATION_LANDSCAPE) {
      flags |= android.view.View.SYSTEM_UI_FLAG_LAYOUT_FULLSCREEN
             | android.view.View.SYSTEM_UI_FLAG_LAYOUT_HIDE_NAVIGATION
             | android.view.View.SYSTEM_UI_FLAG_FULLSCREEN
             | android.view.View.SYSTEM_UI_FLAG_HIDE_NAVIGATION
             | android.view.View.SYSTEM_UI_FLAG_IMMERSIVE_STICKY;
    }
    getWindow().getDecorView().setSystemUiVisibility(flags);
  }

  @Override
  public void onConfigurationChanged(Configuration newConfig) {
    super.onConfigurationChanged(newConfig);
    applySystemUI();
  }

  @Override
  public void onBackPressed() {
    if (chromeClient != null && chromeClient.dismissCustomView()) return;
    if (wv.canGoBack()) {
      wv.goBack();
      return;
    }
    long now = System.currentTimeMillis();
    if (now - lastBackPress < 2000) {
      if (toast != null) toast.cancel();
      super.onBackPressed();
      return;
    }
    lastBackPress = now;
    GradientDrawable bg = new GradientDrawable();
    bg.setCornerRadius(40);
    bg.setColor(0xDD1B1B1F);
    TextView tv = new TextView(this);
    tv.setText("Tap again to exit");
    tv.setTextColor(0xFFFFFFFF);
    tv.setTextSize(15);
    tv.setTypeface(Typeface.create("sans-serif-medium", Typeface.NORMAL));
    tv.setPadding(48, 28, 48, 28);
    tv.setBackground(bg);
    toast = new Toast(this);
    toast.setView(tv);
    toast.setGravity(Gravity.TOP, 0, 0);
    toast.setDuration(Toast.LENGTH_SHORT);
    toast.show();
  }

  @Override
  protected void onPause() {
    super.onPause();
    if (toast != null) toast.cancel();
  }

  @Override
  public void onDownloadStart(String url, String userAgent, String contentDisposition, String mimeType, long contentLength) {
    DownloadManager.Request req = new DownloadManager.Request(Uri.parse(url));
    req.setMimeType(mimeType);
    req.addRequestHeader("User-Agent", userAgent);
    req.setNotificationVisibility(DownloadManager.Request.VISIBILITY_VISIBLE_NOTIFY_COMPLETED);
    String name = URLUtil.guessFileName(url, contentDisposition, mimeType);
    req.setTitle(name);
    req.setDestinationInExternalPublicDir(Environment.DIRECTORY_DOWNLOADS, name);
    DownloadManager dm = (DownloadManager) getSystemService(Context.DOWNLOAD_SERVICE);
    if (dm != null) dm.enqueue(req);
  }%s
  %s
}`, permImports, disableCopyImplements, indicatorField, permFields, !req.HideScrollbars, !req.HideScrollbars, permSettings, req.ZoomEnabled, req.BlockAds || req.AdGuardDNS, req.BlockAds || req.AdGuardDNS, clientCreate, notifInterface, assetInit, loadURL, flBg, pullInit, permOnCreate, disableCopyInit, disableCopyMethod, permMethods))

	if req.SplashEnabled {
		duration := req.SplashDuration
		if duration <= 0 {
			duration = 2000
		}
		bgColor := req.SplashColor
		if bgColor == "" {
			bgColor = "#000000"
		}
		imgPct := req.SplashImageSize
		if imgPct < 0 {
			imgPct = 0
		}
		if imgPct > 100 {
			imgPct = 100
		}
		anim := req.SplashAnimation
		if anim == "" {
			anim = "fade"
		}

		var animImports, animCode, exitAnim string
		switch anim {
		case "fade":
			animImports = "import android.view.animation.AlphaAnimation;"
			animCode = "AlphaAnimation fa = new AlphaAnimation(0f, 1f); fa.setDuration(600); iv.startAnimation(fa);"
			exitAnim = "overridePendingTransition(android.R.anim.fade_in, android.R.anim.fade_out);"
		case "slide":
			animImports = "import android.view.animation.AlphaAnimation;\nimport android.view.animation.Animation;\nimport android.view.animation.AnimationSet;\nimport android.view.animation.TranslateAnimation;"
			animCode = "TranslateAnimation ta = new TranslateAnimation(Animation.RELATIVE_TO_SELF, 0f, Animation.RELATIVE_TO_SELF, 0f, Animation.RELATIVE_TO_SELF, 0.3f, Animation.RELATIVE_TO_SELF, 0f); ta.setDuration(500); AlphaAnimation fa = new AlphaAnimation(0f, 1f); fa.setDuration(500); AnimationSet as = new AnimationSet(true); as.addAnimation(ta); as.addAnimation(fa); iv.startAnimation(as);"
			exitAnim = "overridePendingTransition(android.R.anim.fade_in, android.R.anim.fade_out);"
		default:
			animImports = ""
			animCode = ""
			exitAnim = ""
		}

		writeFile(filepath.Join(srcDir, "SplashActivity.java"),
			fmt.Sprintf(`package com.h2a;
import android.app.Activity;
import android.content.Intent;
import android.os.Bundle;
import android.os.Handler;
import android.widget.ImageView;
import android.widget.RelativeLayout;
import android.graphics.Color;
import android.view.Gravity;
%s

public class SplashActivity extends Activity implements Runnable {
  @Override
  protected void onCreate(Bundle savedInstanceState) {
    super.onCreate(savedInstanceState);
    RelativeLayout layout = new RelativeLayout(this);
    try {
      layout.setBackgroundColor(Color.parseColor("%s"));
    } catch (Exception e) {
      layout.setBackgroundColor(0xFF000000);
    }
    int screenW = getResources().getDisplayMetrics().widthPixels;
    int imgSize = (int)(screenW * %d / 100f);
    ImageView iv = new ImageView(this);
    int id = getResources().getIdentifier("splash_image", "drawable", getPackageName());
    if (id != 0) {
      iv.setImageResource(id);
      iv.setScaleType(ImageView.ScaleType.FIT_CENTER);
    }
    RelativeLayout.LayoutParams lp = new RelativeLayout.LayoutParams(imgSize, imgSize);
    lp.addRule(RelativeLayout.CENTER_IN_PARENT);
    layout.addView(iv, lp);
    setContentView(layout);
    %s
    new Handler().postDelayed(this, %d);
  }
  public void run() {
    startActivity(new Intent(this, WebViewActivity.class));
    %s
    finish();
  }
}`, animImports, bgColor, imgPct, animCode, duration, exitAnim))
	}

	if req.PullRefresh {
		writeFile(filepath.Join(srcDir, "PullIndicator.java"),
			`package com.h2a;
import android.content.Context;
import android.graphics.Canvas;
import android.graphics.Paint;
import android.graphics.Path;
import android.graphics.RectF;
import android.view.View;
public class PullIndicator extends View {
  private float progress;
  private float spin;
  private float extraDeg;
  private boolean loading;
  private float spinAngle;
  private Paint arcPaint, arrowPaint, cardBg;
  private RectF ringOval, cardOval;
  private Path arrowPath;
  private float R, strokeW, aTip, aInset, aWing, cardR;

  public PullIndicator(Context ctx, int accent) {
    super(ctx);
    init(accent);
  }

  private void init(int accent) {
    R = dp(9);
    strokeW = dp(2);
    aTip = dp(2.6f);
    aInset = dp(1.6f);
    aWing = dp(2);

    arcPaint = new Paint(Paint.ANTI_ALIAS_FLAG);
    arcPaint.setStyle(Paint.Style.STROKE);
    arcPaint.setStrokeWidth(strokeW);
    arcPaint.setStrokeCap(Paint.Cap.ROUND);
    arcPaint.setColor(0xFF222222);

    cardBg = new Paint(Paint.ANTI_ALIAS_FLAG);
    cardBg.setStyle(Paint.Style.FILL);
    cardBg.setColor(0xFFFFFFFF);
    cardR = dp(15);

    arrowPaint = new Paint(Paint.ANTI_ALIAS_FLAG);
    arrowPaint.setStyle(Paint.Style.FILL);
    arrowPaint.setColor(0xFF222222);

    ringOval = new RectF();
    cardOval = new RectF();
    arrowPath = new Path();

    setVisibility(View.GONE);
    setScaleX(0.6f);
    setScaleY(0.6f);
  }

  public void setPullProgress(float p) {
    progress = Math.min(p, 1f);
    setAlpha(progress);
    setScaleX(0.6f + 0.4f * progress);
    setScaleY(0.6f + 0.4f * progress);
    invalidate();
  }

  public void setSpin(float s) {
    spin = s;
  }

  public void setSpinBoost(float deg) {
    extraDeg = deg;
  }

  public void setLoading(boolean l) {
    loading = l;
    if (l) {
      setAlpha(1f);
      setScaleX(1f);
      setScaleY(1f);
      spinAngle = 0;
      postInvalidateDelayed(16);
    }
    invalidate();
  }

  @Override
  protected void onDraw(Canvas canvas) {
    super.onDraw(canvas);
    int w = getWidth(), h = getHeight();
    int cx = w / 2, cy = h / 2;

    cardOval.set(cx - cardR, cy - cardR, cx + cardR, cy + cardR);
    ringOval.set(cx - R, cy - R, cx + R, cy + R);

    canvas.drawOval(cardOval, cardBg);

    if (loading) {
      spinAngle += 5;
      if (spinAngle >= 360) spinAngle -= 360;
      int segs = 14;
      float segSweep = 12;
      for (int i = 0; i < segs; i++) {
        float segStart = spinAngle - i * (segSweep + 6);
        int alpha = 255 - i * (255 / segs);
        arcPaint.setAlpha(alpha);
        canvas.drawArc(ringOval, segStart, segSweep, false, arcPaint);
      }
      arcPaint.setAlpha(255);
      postInvalidateDelayed(16);
    } else {
      float sweep = progress * 290f;
      float startA = 125;
      float endA = startA + sweep;
      canvas.save();
      canvas.rotate(spin * 720 + extraDeg, cx, cy);
      canvas.drawArc(ringOval, startA, sweep, false, arcPaint);

      if (sweep > 0) {
        float rad = (float) Math.toRadians(endA);
        float cos = (float) Math.cos(rad);
        float sin = (float) Math.sin(rad);

        // Tangent T = (-sin, cos), Normal N = (cos, sin)
        float tx = -sin;
        float ty = cos;
        float nx = cos;
        float ny = sin;

        // Point on ring
        float px = cx + R * cos;
        float py = cy + R * sin;

        // Arrowhead: tip, base-left, base-right
        float tipX = px + tx * aTip;
        float tipY = py + ty * aTip;
        float blX = px - tx * aInset + nx * aWing;
        float blY = py - ty * aInset + ny * aWing;
        float brX = px - tx * aInset - nx * aWing;
        float brY = py - ty * aInset - ny * aWing;

        arrowPath.reset();
        arrowPath.moveTo(tipX, tipY);
        arrowPath.lineTo(blX, blY);
        arrowPath.lineTo(brX, brY);
        arrowPath.close();
        canvas.drawPath(arrowPath, arrowPaint);
      }
      canvas.restore();
    }
  }

  private float dp(float px) { return px * getResources().getDisplayMetrics().density; }
}`)
		writeFile(filepath.Join(srcDir, "PullListener.java"),
			`package com.h2a;
import android.os.Handler;
import android.os.Looper;
import android.view.MotionEvent;
import android.view.View;
import android.webkit.WebView;
public class PullListener implements View.OnTouchListener, PaddingClient.PullCallback, Runnable {
  private WebView wv;
  private PullIndicator indicator;
  private float startY;
  private float pullDist;
  private boolean dragging;
  private boolean loading;
  private float indicatorH;
  private float threshold;
  private float maxSlide;
  private Handler handler;
  private Runnable forceHide;

  public PullListener(WebView wv, PullIndicator indicator) {
    this.wv = wv;
    this.indicator = indicator;
    float d = indicator.getContext().getResources().getDisplayMetrics().density;
    this.indicatorH = 56 * d;
    this.threshold = 56 * d;
    this.maxSlide = indicatorH * 2;
    this.handler = new Handler(Looper.getMainLooper());
    this.forceHide = this;
    indicator.setTranslationY(-this.indicatorH);
  }

  @Override
  public void run() {
    loading = false;
    indicator.setVisibility(View.GONE);
    indicator.setTranslationY(-indicatorH);
    indicator.setLoading(false);
    indicator.setPullProgress(0);
    indicator.setSpin(0);
    indicator.setSpinBoost(0);
  }

  @Override
  public void onPageFinished() {
    handler.removeCallbacks(forceHide);
    loading = false;
    indicator.setVisibility(View.GONE);
    indicator.setTranslationY(-indicatorH);
    indicator.setLoading(false);
    indicator.setPullProgress(0);
    indicator.setSpin(0);
    indicator.setSpinBoost(0);
  }

  @Override
  public boolean onTouch(View v, MotionEvent e) {
    if (e.getPointerCount() > 1) { dragging = false; return false; }
    switch (e.getAction()) {
      case MotionEvent.ACTION_DOWN:
        if (!loading) {
          startY = e.getY();
          pullDist = 0;
          dragging = true;
        }
        break;
      case MotionEvent.ACTION_MOVE:
        if (!dragging || loading) break;
        if (wv.getScrollY() > 0) { dragging = false; break; }
        pullDist = e.getY() - startY;
        if (pullDist <= 0) break;
        wv.evaluateJavascript("document.body.style.webkitUserSelect='none';document.body.style.userSelect='none'", null);
        indicator.setVisibility(View.VISIBLE);
        float resisted = Math.min((float) Math.pow(pullDist, 0.85), maxSlide);
        indicator.setTranslationY(resisted - indicatorH);
        indicator.setPullProgress(pullDist / threshold);
        indicator.setSpin(resisted / maxSlide);
        float maxPull = (float) Math.pow(maxSlide, 1.0 / 0.85);
        float over = Math.max(0, pullDist - maxPull);
        indicator.setSpinBoost(Math.min(over * 0.5f, 45f));
        return true;
      case MotionEvent.ACTION_UP:
      case MotionEvent.ACTION_CANCEL:
        dragging = false;
        wv.evaluateJavascript("document.body.style.webkitUserSelect='';document.body.style.userSelect=''", null);
        if (loading) break;
        if (pullDist >= threshold) {
          loading = true;
          indicator.setTranslationY(0);
          indicator.setLoading(true);
          handler.postDelayed(forceHide, 10000);
          wv.reload();
          return true;
        }
        indicator.setVisibility(View.GONE);
        indicator.setTranslationY(-indicatorH);
        indicator.setPullProgress(0);
        indicator.setSpin(0);
        indicator.setSpinBoost(0);
        break;
    }
    return false;
  }
}`)
	}

	classesDir := filepath.Join(work, "classes")
	os.MkdirAll(classesDir, 0755)
	javacFiles := []string{
		filepath.Join(srcDir, "PaddingClient.java"),
		filepath.Join(srcDir, "WebViewActivity.java"),
		filepath.Join(srcDir, "H2AChromeClient.java"),
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

	// 4. DEX with d8.jar
	logf("Generating classes.dex")
	dexPath := filepath.Join(proj, "classes.dex")
	d8jar := findLocalOrSystem("tools/d8.jar", "d8_jar",
		filepath.Join(os.Getenv("ANDROID_HOME"), "build-tools", "34.0.0", "lib", "d8.jar"))
	classFiles, _ := filepath.Glob(filepath.Join(classesDir, "com", "h2a", "*.class"))
	d8Args := []string{"-Xmx512M", "-cp", d8jar, "com.android.tools.r8.D8",
		"--lib", androidJar,
		"--output", proj,
	}
	d8Args = append(d8Args, classFiles...)
	run(rec, "java", d8Args, logf)

	// 5. Package with aapt2
	logf("Packaging APK")
	unsigned := filepath.Join(work, "unsigned.apk")
	aaptArgs := []string{"link",
		"--manifest", filepath.Join(proj, "AndroidManifest.xml"),
		"--version-code", "1",
		"--version-name", versionName(req.VersionCode),
		"--min-sdk-version", "21",
		"--target-sdk-version", "34",
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

	// 5. Add dex to APK
	logf("Adding classes.dex")
	run(rec, "zip", []string{"-j", unsigned, dexPath}, logf)

	// 6. Zipalign (must happen BEFORE signing)
	logf("Aligning APK")
	aligned := filepath.Join(work, "aligned.apk")
	run(rec, "zipalign", []string{"-p", "4", unsigned, aligned}, logf)

	// 7. Sign (custom keystore or embedded debug keystore)
	logf("Signing APK")
	ks := getKeystore()
	ksPass := "pass:h2ah2a"
	keyAlias := "h2a"
	keyPass := "pass:h2ah2a"
	if req.KeystoreBase64 != "" {
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
	apkSignerJar := findLocalOrSystem("tools/apksigner.jar", "apksigner_jar",
		"/data/data/com.termux/files/usr/share/java/apksigner.jar")
	run(rec, "java", []string{"-jar", apkSignerJar, "sign",
		"--ks", ks, "--ks-pass", ksPass, "--ks-key-alias", keyAlias,
		"--key-pass", keyPass, "--out", signed, aligned,
	}, logf)

	// 8. Finalize
	final := safeName(req.AppName) + ".apk"
	copyFile(filepath.Join(baseDir, "output", final), signed)
	rec.status = "done"
	rec.apkName = final
	logf("Done: %s", final)
}

// -- helpers --

// findLocalOrSystem looks for a file first in tools/, then config.json, then the hardcoded system path.
func findLocalOrSystem(localPath, configKey, systemPath string) string {
	p := filepath.Join(baseDir, localPath)
	if _, err := os.Stat(p); err == nil {
		return p
	}
	cfg := loadConfig()
	if configKey == "d8_jar" && cfg.D8Jar != "" {
		return cfg.D8Jar
	}
	if configKey == "apksigner_jar" && cfg.ApkSignerJar != "" {
		return cfg.ApkSignerJar
	}
	return systemPath
}

// getKeystore writes the embedded debug keystore to disk on first call.
func getKeystore() string {
	ksOnce.Do(func() {
		ksPath = filepath.Join(baseDir, "tmp", "debug.keystore")
		os.MkdirAll(filepath.Dir(ksPath), 0755)
		os.WriteFile(ksPath, embeddedKS, 0644)
	})
	return ksPath
}

func parseThemeColor(req BuildRequest) (string, int) {
	hex := req.ThemeColor
	if hex == "" {
		hex = "#1C1C1E"
	}
	if len(hex) > 0 && hex[0] == '#' {
		hex = hex[1:]
	}
	c, _ := strconv.ParseInt(hex, 16, 32)
	alpha := int(c) | 0xFF000000
	return "0x" + fmt.Sprintf("%08X", alpha), alpha
}

func statusBarColor(isURL bool, themeHex string) string {
	if isURL {
		return themeHex
	}
	return "0x00000000"
}

func navBarColor(transparent bool, themeColor string) string {
	if transparent {
		return "0x00000000"
	}
	return themeColor
}

func versionName(v string) string {
	if v == "" {
		return "1.0"
	}
	return v
}

func wrapHTML(req BuildRequest) string {
	css := ""
	if req.CSS != "" {
		css += "\n  " + req.CSS
	}
	css = "\n  <style>\n  body{margin:0}\n" + css + "\n  </style>"
	notifShim := ""
	if req.NotifPermission {
		notifShim = `
  <script>
(function(){
  if(typeof Notification!=='undefined')return;
  if(typeof H2A==='undefined'||!H2A.showNotification)return;
  var p=H2A.getNotificationPermission(),cbs=[];
  window.Notification=function(t,o){
    if(p==='granted')H2A.showNotification(t,(o&&o.body)||'');
  };
  Object.defineProperty(Notification,'permission',{get:function(){return p;}});
  Notification.requestPermission=function(){
    return new Promise(function(r){
      if(p==='granted'){r('granted');return;}
      cbs.push(r);H2A.requestNotificationPermission();
      var n=0,i=setInterval(function(){
        p=H2A.getNotificationPermission();
        if(p==='granted'||++n>60){
          clearInterval(i);
          cbs.forEach(function(c){c(p==='granted'?'granted':'denied');});
          cbs=[];
        }
      },500);
    });
  };
})();
  </script>`
	}
	js := ""
	if req.JS != "" {
		js = "\n  <script>\n" + req.JS + "\n  </script>"
	}
	return fmt.Sprintf(`<!DOCTYPE html>
<html>
<head>
  <meta charset="utf-8">
  <meta name="viewport" content="width=device-width,initial-scale=1.0,user-scalable=yes,maximum-scale=5.0">
  <title>%s</title>%s%s
</head>
<body>
%s%s
</body>
</html>`, req.AppName, css, notifShim, req.HTML, js)
}

var pngEncoder = &png.Encoder{CompressionLevel: png.BestCompression}

func compressPNG(data []byte) []byte {
	img, err := png.Decode(bytes.NewReader(data))
	if err != nil {
		return data // not a valid PNG, return as-is
	}
	var buf bytes.Buffer
	if err := pngEncoder.Encode(&buf, img); err != nil {
		return data
	}
	if buf.Len() >= len(data) {
		return data // no savings
	}
	return buf.Bytes()
}

func findAndroidJar() string {
	// Check local copy first
	local := filepath.Join(baseDir, "tools", "android.jar")
	if _, err := os.Stat(local); err == nil {
		return local
	}
	// Check config.json
	cfg := loadConfig()
	if cfg.AndroidJar != "" {
		if _, err := os.Stat(cfg.AndroidJar); err == nil {
			return cfg.AndroidJar
		}
	}
	// Fall back to SDK
	sdk := os.Getenv("ANDROID_HOME")
	for _, v := range []string{"34", "35", "36", "33"} {
		p := filepath.Join(sdk, "platforms", "android-"+v, "android.jar")
		if _, err := os.Stat(p); err == nil {
			return p
		}
	}
	return ""
}

func run(rec *record, name string, args []string, logf func(string, ...interface{})) {
	cmd := exec.Command(name, args...)
	out, err := cmd.CombinedOutput()
	logf("[%s] %s", name, string(out))
	if err != nil {
		fail(rec, name+" failed: "+err.Error()+"\n"+string(out))
	}
}

func fail(rec *record, msg string) {
	rec.status = "failed"
	rec.err = msg
	panic("build-fail")
}

func genPaddingClient(blockAds bool, adguardDNS bool, useAssetLoader bool) string {
	if !blockAds {
		if useAssetLoader {
			return `package com.h2a;
import android.webkit.WebResourceResponse;
import android.webkit.WebView;
import android.webkit.WebViewClient;
import android.net.Uri;
import android.util.Log;
import java.io.InputStream;
import java.io.ByteArrayInputStream;
public class PaddingClient extends WebViewClient {
  public interface PullCallback {
    void onPageFinished();
  }
  private PullCallback callback;
  private static final String SECURE_HOST = "appassets.androidplatform.net";
  public PaddingClient() { this.callback = null; }
  public PaddingClient(PullCallback cb) { this.callback = cb; }
  @Override
  public boolean shouldOverrideUrlLoading(WebView view, String url) {
    view.loadUrl(url);
    return true;
  }
  private static String guessMime(String path) {
    String l = path.toLowerCase();
    if (l.endsWith(".html") || l.endsWith(".htm")) return "text/html";
    if (l.endsWith(".css")) return "text/css";
    if (l.endsWith(".js")) return "application/javascript";
    if (l.endsWith(".json")) return "application/json";
    if (l.endsWith(".png")) return "image/png";
    if (l.endsWith(".jpg") || l.endsWith(".jpeg")) return "image/jpeg";
    if (l.endsWith(".gif")) return "image/gif";
    if (l.endsWith(".svg")) return "image/svg+xml";
    if (l.endsWith(".webp")) return "image/webp";
    if (l.endsWith(".ico")) return "image/x-icon";
    if (l.endsWith(".woff")) return "font/woff";
    if (l.endsWith(".woff2")) return "font/woff2";
    return "text/plain";
  }
  private WebResourceResponse serveAsset(WebView view, String url) {
    try {
      android.net.Uri uri = android.net.Uri.parse(url);
      if (SECURE_HOST.equals(uri.getHost())) {
        String path = uri.getPath();
        if (path == null || path.isEmpty() || "/".equals(path)) path = "/index.html";
        if (path.startsWith("/")) path = path.substring(1);
        InputStream is = view.getContext().getAssets().open(path);
        return new WebResourceResponse(guessMime(path), null, is);
      }
    } catch (Exception e) {}
    return null;
  }
  @Override
  public WebResourceResponse shouldInterceptRequest(WebView view, String url) {
    WebResourceResponse asset = serveAsset(view, url);
    if (asset != null) return asset;
    return super.shouldInterceptRequest(view, url);
  }
  @Override
  public WebResourceResponse shouldInterceptRequest(WebView view, android.webkit.WebResourceRequest request) {
    WebResourceResponse asset = serveAsset(view, request.getUrl().toString());
    if (asset != null) return asset;
    return super.shouldInterceptRequest(view, request);
  }
  @Override
  public void onPageFinished(WebView view, String url) {
    super.onPageFinished(view, url);
    view.evaluateJavascript("(function(){var m=document.querySelector('meta[name=viewport]');if(m)m.content+=(m.content?',':'')+'user-scalable=yes,maximum-scale=5.0';else{var n=document.createElement('meta');n.name='viewport';n.content='width=device-width,initial-scale=1.0,user-scalable=yes,maximum-scale=5.0';document.head.appendChild(n);}})()", null);
    if (callback != null) callback.onPageFinished();
  }
}`
		}
		return `package com.h2a;
import android.webkit.WebView;
import android.webkit.WebViewClient;
public class PaddingClient extends WebViewClient {
  public interface PullCallback {
    void onPageFinished();
  }
  private PullCallback callback;
  public PaddingClient() { this.callback = null; }
  public PaddingClient(PullCallback cb) { this.callback = cb; }
  @Override
  public boolean shouldOverrideUrlLoading(WebView view, String url) {
    view.loadUrl(url);
    return true;
  }
  @Override
  public void onPageFinished(WebView view, String url) {
    super.onPageFinished(view, url);
    view.evaluateJavascript("(function(){var m=document.querySelector('meta[name=viewport]');if(m)m.content+=(m.content?',':'')+'user-scalable=yes,maximum-scale=5.0';else{var n=document.createElement('meta');n.name='viewport';n.content='width=device-width,initial-scale=1.0,user-scalable=yes,maximum-scale=5.0';document.head.appendChild(n);}})()", null);
    if (callback != null) callback.onPageFinished();
  }
}`
	}
	return `package com.h2a;
import android.webkit.WebResourceResponse;
import android.webkit.WebView;
import android.webkit.WebViewClient;
import android.net.Uri;
import java.io.ByteArrayInputStream;
import java.io.InputStream;
import java.util.HashSet;
import java.util.HashMap;
import java.net.DatagramSocket;
import java.net.DatagramPacket;
import java.net.InetAddress;
public class PaddingClient extends WebViewClient {
  public interface PullCallback {
    void onPageFinished();
  }
  private PullCallback callback;
  private HashSet<String> blocked;
  private boolean blockAds;
  private boolean useAdGuardDNS;
  private HashMap<String,String> dnsCache;
  private int dnsSeq;
  private boolean useAssetLoader = ` + fmt.Sprintf("%t", useAssetLoader) + `;
  public PaddingClient() { this.callback = null; this.blockAds = false; this.useAdGuardDNS = false; }
  public PaddingClient(PullCallback cb) { this.callback = cb; this.blockAds = false; this.useAdGuardDNS = false; }
  public PaddingClient(boolean block) { this.callback = null; init(block, false); }
  public PaddingClient(boolean block, boolean dns) { this.callback = null; init(block, dns); }
  public PaddingClient(PullCallback cb, boolean block) { this.callback = cb; init(block, false); }
  public PaddingClient(PullCallback cb, boolean block, boolean dns) { this.callback = cb; init(block, dns); }
  private void init(boolean block, boolean dns) {
    this.blockAds = block;
    this.useAdGuardDNS = dns;
    if (dns) { dnsCache = new HashMap<String,String>(); dnsSeq = 0; }
    if (!block) return;
    blocked = new HashSet<String>();
    String[] base = {
      "doubleclick.net","googlesyndication.com","googleadservices.com","adservice.google.com",
      "amazon-adsystem.com","adsrvr.org","adnxs.com","openx.net","pubmatic.com",
      "rubiconproject.com","criteo.com","casalemedia.com","adform.net","appnexus.com",
      "bidswitch.net","moatads.com","taboola.com","outbrain.com","popads.net",
      "propellerads.com","exoclick.com","juicyads.com","advertising.com","adzerk.net",
      "criteo.net","sharethrough.com","triplelift.com","sovrn.com","indexww.com",
      "contextweb.com","rlcdn.com","adsafeprotected.com","1rx.io","adlightning.com",
      "qerelink.qpon","propellerpops.com","onclickads.net","onclkds.com","popcash.net",
      "trafficjunky.net","adsterra.com","ad-maven.com","adinplay.com","monetag.com",
      "prop-fra-01.com","evadav.net","galaksion.com"
    };
    for (String d : base) blocked.add(d);
    String[] redirects = {
      "disgusting-sun.com","disgusting-zoo.com","disgusting-moon.com",
      "tsyndolls.com","tsyndoll.com","trafficjunky.com","trafficjunky.net",
      "tsyndicate.com","rtbsuperhub.com","bidresolving.com","pushzones.com",
      "adsafeprotected.com","rta.direct","wwwpromoter.com","theankara.com",
      "chrunched.com","evadav.net","straightdirectory.com","simplecyberdefense.com",
      "clickaine.com","clickadu.com","g-em.com","adxxx.com","xvideoslive.com",
      "rm358.com"
    };
    for (String d : redirects) blocked.add(d);` + adguardBlocklist(adguardDNS) + `
  }
  @Override
  public boolean shouldOverrideUrlLoading(WebView view, String url) {
    return handleNavigation(view, url, false);
  }
  @Override
  public boolean shouldOverrideUrlLoading(WebView view, android.webkit.WebResourceRequest request) {
    boolean gesture = false;
    if (android.os.Build.VERSION.SDK_INT >= 24) gesture = request.hasGesture();
    String url = request.getUrl().toString();
    handleNavigation(view, url, gesture);
    return true; // always handle ourselves, don't fall through to String overload
  }
  private boolean handleNavigation(WebView view, String url, boolean hasGesture) {
    if (!blockAds || url == null) { view.loadUrl(url); return true; }
    String cur = view.getUrl();
    // Allow same-domain navigation (internal page links)
    if (cur != null) {
      try {
        java.net.URL nu = new java.net.URL(url);
        java.net.URL cu = new java.net.URL(cur);
        String nh = nu.getHost();
        String ch = cu.getHost();
        if (nh != null && ch != null && (nh.equals(ch) || nh.endsWith("."+ch))) {
          view.loadUrl(url);
          return true;
        }
      } catch (Exception e) {}
    }
    // Block everything else
    view.stopLoading();
    return true;
  }
  // @deprecated - kept for compilation compatibility, not called
  private boolean isSuspiciousUrl(String url, String currentUrl) {
    try {
      java.net.URL u = new java.net.URL(url);
      java.net.URL cu = new java.net.URL(currentUrl);
      String h = u.getHost();
      String ch = cu.getHost();
      if (h == null || ch == null || h.equals(ch) || h.endsWith("."+ch)) return false;
      // Different domain — check for ad patterns
      String q = u.getQuery();
      String p = u.getPath();
      if (q != null && q.toLowerCase().matches(".*(clickid|zoneid|campaign|sid|subid|aff|offerid|traffic|adid|popunder|popup|redirect|promo|partner|ymid|token).*"))
        return true;
      if (p != null && p.matches("/[0-9]+/[0-9]+"))
        return true;
      if (h.matches("[a-z]{2,3}[0-9]{2,4}\\..*"))
        return true;
    } catch (Exception e) {}
    return false;
  }

  private boolean isAdDomain(String url) {
    if (blocked == null) return false;
    try {
      java.net.URL p = new java.net.URL(url);
      String h = p.getHost();
      if (h != null) {
        while (h.contains(".")) {
          if (blocked.contains(h)) return true;
          h = h.substring(h.indexOf(".") + 1);
        }
        if (blocked.contains(h)) return true;
      }
    } catch (Exception e) {}
    return false;
  }
  private static String guessMime(String path) {
    String l = path.toLowerCase();
    if (l.endsWith(".html") || l.endsWith(".htm")) return "text/html";
    if (l.endsWith(".css")) return "text/css";
    if (l.endsWith(".js")) return "application/javascript";
    if (l.endsWith(".json")) return "application/json";
    if (l.endsWith(".png")) return "image/png";
    if (l.endsWith(".jpg") || l.endsWith(".jpeg")) return "image/jpeg";
    if (l.endsWith(".gif")) return "image/gif";
    if (l.endsWith(".svg")) return "image/svg+xml";
    if (l.endsWith(".webp")) return "image/webp";
    if (l.endsWith(".ico")) return "image/x-icon";
    if (l.endsWith(".woff")) return "font/woff";
    if (l.endsWith(".woff2")) return "font/woff2";
    return "text/plain";
  }
  private WebResourceResponse serveAsset(WebView view, String url) {
    try {
      Uri uri = Uri.parse(url);
      if ("appassets.androidplatform.net".equals(uri.getHost())) {
        String path = uri.getPath();
        if (path == null || path.isEmpty() || "/".equals(path)) path = "/index.html";
        if (path.startsWith("/")) path = path.substring(1);
        InputStream is = view.getContext().getAssets().open(path);
        return new WebResourceResponse(guessMime(path), null, is);
      }
    } catch (Exception e) {}
    return null;
  }
  @Override
  public WebResourceResponse shouldInterceptRequest(WebView view, String url) {
    if (useAssetLoader) { WebResourceResponse a = serveAsset(view, url); if (a != null) return a; }
    return intercept(url);
  }
  @Override
  public WebResourceResponse shouldInterceptRequest(WebView view, android.webkit.WebResourceRequest req) {
    if (useAssetLoader) { WebResourceResponse a = serveAsset(view, req.getUrl().toString()); if (a != null) return a; }
    return intercept(req.getUrl().toString());
  }
  private String adguardResolve(String host) {
    if (host == null) return null;
    if (dnsCache.containsKey(host)) return dnsCache.get(host);
    try {
      byte[][] parts = new byte[][]{host.getBytes("UTF-8")};
      int qlen = 12 + host.length() + 2 + 4;
      byte[] q = new byte[qlen];
      q[0] = (byte)(dnsSeq >> 8); q[1] = (byte)(dnsSeq & 0xFF); dnsSeq++;
      q[2] = 1; q[5] = 1;
      int pos = 12;
      for (String label : host.split("\\.")) {
        byte[] b = label.getBytes("UTF-8");
        q[pos++] = (byte)b.length;
        System.arraycopy(b, 0, q, pos, b.length);
        pos += b.length;
      }
      q[pos++] = 0; q[pos++] = 1; q[pos++] = 0; q[pos++] = 1;
      DatagramSocket s = new DatagramSocket();
      s.setSoTimeout(2000);
      s.send(new DatagramPacket(q, pos, InetAddress.getByName("94.140.14.14"), 53));
      byte[] r = new byte[512];
      DatagramPacket resp = new DatagramPacket(r, 512);
      s.receive(resp);
      s.close();
      int nans = ((r[6] & 0xFF) << 8) | (r[7] & 0xFF);
      int off = pos;
      for (int i = 0; i < nans; i++) {
        if ((r[off] & 0xC0) == 0xC0) off += 2;
        else { while (r[off] != 0) off += (r[off] & 0xFF) + 1; off++; }
        int type = ((r[off + 1] & 0xFF) << 8) | (r[off + 2] & 0xFF);
        int len = ((r[off + 9] & 0xFF) << 8) | (r[off + 10] & 0xFF);
        if (type == 1 && len == 4) {
          String ip = (r[off+11]&0xFF)+"."+(r[off+12]&0xFF)+"."+(r[off+13]&0xFF)+"."+(r[off+14]&0xFF);
          dnsCache.put(host, ip);
          return ip;
        }
        off += 10 + len;
      }
      dnsCache.put(host, ".");
      return ".";
    } catch (Exception e) {
      dnsCache.put(host, ".");
      return ".";
    }
  }

  private WebResourceResponse intercept(String url) {
    if (blockAds && url != null) {
      String l = url.toLowerCase();
      if (l.startsWith("intent://") || l.startsWith("market://") ||
          l.startsWith("shopee://") || l.startsWith("shopeelink://") || l.startsWith("lazada://"))
        return new WebResourceResponse("text/plain", "UTF-8", new ByteArrayInputStream("".getBytes()));
      if (blocked != null) {
        try {
          java.net.URL p = new java.net.URL(url);
          String h = p.getHost();
          if (h != null) {
            while (h.contains(".")) {
              if (blocked.contains(h)) return new WebResourceResponse("text/plain", "UTF-8", new ByteArrayInputStream("".getBytes()));
              h = h.substring(h.indexOf(".") + 1);
            }
            if (blocked.contains(h)) return new WebResourceResponse("text/plain", "UTF-8", new ByteArrayInputStream("".getBytes()));
          }
        } catch (Exception e) {}
      }
      if (useAdGuardDNS) {
        try {
          java.net.URL p = new java.net.URL(url);
          String ip = adguardResolve(p.getHost());
          if ("0.0.0.0".equals(ip))
            return new WebResourceResponse("text/plain", "UTF-8", new ByteArrayInputStream("".getBytes()));
        } catch (Exception e) {}
      }
    }
    return null;
  }
  @Override
  public void onPageFinished(WebView view, String url) {
    super.onPageFinished(view, url);
    if (blockAds) {
      view.evaluateJavascript(
        "(function(){" +
        "var _o=window.open;window.open=function(url,n){if(url){try{var a=document.createElement('a');a.href=url;if(a.hostname&&a.hostname!==location.hostname)return null;}catch(e){}}if(n==='_blank'||n==='_new')return null;return _o.apply(this,arguments);};" +
        "if(navigator.sendBeacon)navigator.sendBeacon=function(){return false;};" +
        "try{Object.defineProperty(navigator,'plugins',{get:function(){return[1,2,3,4,5];}})}catch(e){}" +
        "try{Object.defineProperty(navigator,'hardwareConcurrency',{get:function(){return 4;}})}catch(e){}" +
        "try{Object.defineProperty(navigator,'deviceMemory',{get:function(){return 4;}})}catch(e){}" +
        "try{delete window.RTCPeerConnection;window.RTCPeerConnection=undefined;}catch(e){}" +
        "try{delete window.webkitRTCPeerConnection;window.webkitRTCPeerConnection=undefined;}catch(e){}" +
        "var s=document.createElement('style');" +
        "s.textContent='.adsbox,.adsbygoogle," +
        "ins.adsbygoogle,div[id^=div-gpt-ad],div[id^=google_ads_iframe_]," +
        ".ad-popup,.ad-overlay,.modal-ad,.overlay-ad,.popup-ad,.popup-overlay," +
        "[class*=popup-ad],.sponsored-content,[id*=google_ads]{display:none!important}';" +
        "document.head.appendChild(s);" +
        "var adDomains={" +
        "\"doubleclick.net\":1,\"googlesyndication.com\":1,\"googleadservices.com\":1,\"adservice.google.com\":1," +
        "\"amazon-adsystem.com\":1,\"adsrvr.org\":1,\"adnxs.com\":1,\"openx.net\":1,\"pubmatic.com\":1," +
        "\"rubiconproject.com\":1,\"criteo.com\":1,\"casalemedia.com\":1,\"adform.net\":1,\"appnexus.com\":1," +
        "\"bidswitch.net\":1,\"moatads.com\":1,\"taboola.com\":1,\"outbrain.com\":1,\"popads.net\":1," +
        "\"propellerads.com\":1,\"exoclick.com\":1,\"juicyads.com\":1,\"advertising.com\":1,\"adzerk.net\":1," +
        "\"criteo.net\":1,\"sharethrough.com\":1,\"triplelift.com\":1,\"sovrn.com\":1,\"indexww.com\":1," +
        "\"contextweb.com\":1,\"rlcdn.com\":1,\"adsafeprotected.com\":1,\"1rx.io\":1,\"adlightning.com\":1," +
        "\"qerelink.qpon\":1,\"propellerpops.com\":1,\"onclickads.net\":1,\"onclkds.com\":1,\"popcash.net\":1," +
        "\"trafficjunky.net\":1,\"adsterra.com\":1,\"ad-maven.com\":1,\"adinplay.com\":1,\"monetag.com\":1," +
        "\"prop-fra-01.com\":1,\"evadav.net\":1,\"galaksion.com\":1" +
        "};" +
        "function isAdSrc(el){" +
        "var t=el.tagName;" +
        "if(t==='IMG'||t==='IFRAME'||t==='SCRIPT'){" +
        "var s=el.src||el.getAttribute('src')||'';" +
        "for(var d in adDomains){if(s.indexOf(d)!==-1)return true;}" +
        "}" +
        "return false;" +
        "}" +
        "function hideAds(root){" +
        "var imgs=root.querySelectorAll('img,iframe,script');" +
        "for(var i=0;i<imgs.length;i++){" +
        "if(isAdSrc(imgs[i])){" +
        "var p=imgs[i].parentElement;" +
        "for(var j=0;j<4&&p&&p!==document.body;j++){" +
        "if(p.offsetWidth>100||p.offsetHeight>100){p.style.display='none';break;}" +
        "p=p.parentElement;" +
        "}" +
        "}" +
        "}" +
        "}" +
        "hideAds(document);" +
        "new MutationObserver(function(ms){ms.forEach(function(m){" +
        "m.addedNodes.forEach(function(n){if(n.nodeType===1&&n.querySelectorAll)hideAds(n);});" +
        "})}).observe(document.documentElement,{childList:true,subtree:true});" +
        "})()",
        null);
    }
    view.evaluateJavascript("(function(){var m=document.querySelector('meta[name=viewport]');if(m)m.content+=(m.content?',':'')+'user-scalable=yes,maximum-scale=5.0';else{var n=document.createElement('meta');n.name='viewport';n.content='width=device-width,initial-scale=1.0,user-scalable=yes,maximum-scale=5.0';document.head.appendChild(n);}})()", null);
    if (callback != null) callback.onPageFinished();
  }
}`
}

func adguardBlocklist(adguardDNS bool) string {
	if !adguardDNS {
		return ""
	}
	return "\n    String[] adg = {\n" +
		`      "2mdn.net","2o7.net","33across.com","4cp776.site","4dex.io","abmr.net",
      "addthis.com","adengage.com","adf.ly","adkeeper.net","admedo.com",
      "adnetwork.net","adobela.com","adonnetwork.com","adplexo.com",
      "adpone.com","adpushup.com","adreclaim.com","adrecover.com",
      "adservd.com","adservicer.com","adspirit.com","adsymptotic.com","adtaily.com",
      "adtech.com","adtelligent.com","adtrue.com","adups.com","advangelists.com",
      "adventori.com","adversal.com","advertnative.com","adview.com","adzerk.com",
      "affexa.com","affiliaweb.com","affluentco.com","airpush.com",
      "amobee.com","ampliffy.com","aniview.com","antevenio.com",
      "aolp.jp","apester.com","appenda.com","arcadebuzz.com","atdmt.com","atlassolutions.com",
      "audiencetv.com","avantisvideo.com","bannerflow.com","bannersnack.com",
      "baronsoffers.com","beachfront.com","beintoo.com","bet365affiliates.com","bf-ad.net",
      "bidder.com","bidgear.com","bidmachine.io","bizrate.com","blismedia.com","blogads.com",
      "bluecava.com","bluekai.com","bounceexchange.com","brainty.com","brightcom.com",
      "btrll.com","buysellads.com","buzzvil.com","carbonads.com","carambo.la","cbox.ws",
      "celtra.com","cetzboo.com","chango.com","cheqzone.com","chitika.com","choicestream.com",
      "clean.gg","clearseasmedia.com","clevertap.com","clixgalore.com","cmail1.com",
      "coinad.com","collective.com","commindo-media.de","commumobi.com","compasslabs.ai",
      "congstar.de","connatix.com","connexity.net","consumable.com","conversantmedia.com",
      "creafi.com","crispmedia.com","cxense.com","cyberagent.co.jp","dable.io","dainikb.com",
      "datawrkz.com","dc-storm.com","decenterads.com","deloton.com","deltaprojects.com",
      "demdex.com","dep-x.com","dgmax.io","digitaltarget.ru","dpcdn.com",
      "e-planning.net","effectivemeasure.com","eleavers.com","emxdigital.com","engagebdr.com",
      "enoratraffic.com","epom.com","eskimi.com","etargetnet.com","everesttech.net",
      "exosrv.com","exponential.com","eyeota.net","eyereturn.com","fastcmp.com",
      "fearlessrevenue.com","flixsyndication.com","flocktory.com","freestar.com",
      "fuseplatform.com","gamoshi.io","genieesspv.jp","getintent.com","gigya.com",
      "gladlyads.com","gladlyads.in","globalhopedall.com","globulematchw.xyz","gobicybe.com",
      "gravity.com","greedseed.com","grofers.com","grow-ist.com",
      "growthhouse.co","gumgum.com","hearty.llc","hellobar.com","hiido.com","hilltopads.com",
      "hola-player.com","hoodline.com","hyprmx.com","iasds01.com","ibillboard.com",
      "ictv.com","idle-ads.com","ignitionone.com","impact.com","imrworldwide.com",
      "infolinks.com","inmobi.com","innity.com","intentiq.com","intergi.com","inviziads.com",
      "iocket.net","ipredictive.com","ispot.tv","jampp.com","jivox.com","kadam.net",
      "kevel.com","keymedia.info","kixer.com","komoona.com","krux.net","lacreates.com",
      "leadbolt.net","leadklozer.com","lemmatechnologies.com",
      "ligatus.com","linkprice.com","linuxmobi.com","liquidm.com","lkqd.com","lognv.com",
      "longtailvideo.com","loopme.com","lucky-ads.com","macromill.com","magnite.com",
      "mailfire.io","mantisad.net","marchex.io","markethealth.com","marketron.com",
      "marvellousmachine.com","masteraffiliates.org","mathtag.com","mb104.com",
      "mc-market.org","mcsqd.com","media.net","media6degrees.com","mediaalley.com",
      "mediabong.net","medialand.ru","medianetnow.com","mediasquare.fr","medicx.com",
      "merchenta.com","mgage.com","microad.jp","millennialmedia.com","misterbell.it",
      "mixmarket.biz","mmismm.com","mobads.baidu.com","mobivity.com","mobtrks.com",
      "mopub.com","mowaymedia.com","musculahq.com","myaffiliates.com","myexpertise.de",
      "mynativeplatform.com","narrative.io","nativeads.com","networld.hk","newstogram.com",
      "nexac.com","ngenix.net","nielsen-online.com","nitroscripts.com","nowspots.com",
      "nxtck.com","ogury.com","omniture.com","onads.com","onaudience.com","onedmp.com",
      "onetag-sys.com","openmarket.mobi","optmnstr.com","oraclecloudads.com",
      "padsdel.com","pagefair.net","parrable.com","payclick.it",
      "pcash.im","peerfly.com","performancerevenues.com","permutive.com","phoenix-widget.com",
      "pixfuture.com","popin.cc","powerlinks.com","premiumnetwork.com",
      "primis.tech","projectagora.com","provers.pro","pubgalaxy.com",
      "pubnative.net","pulseem.com","pusherism.com","pushhouse.com",
      "quantcount.com","quantum-ad-s.com","quantserve.com","qubit.com","quinstreet.com",
      "r-ad.ne.jp","radiumone.com","rankmylist.com","rayjump.com","reachforce.com",
      "redshell.io","redirect.com","refersion.com","reklama.com","reklamstore.com",
      "remintrex.com","reso.no","resultrix.com","retargetly.com","revenuehut.com","revup.jp",
      "rfihub.com","rhythmone.com","richaudience.com","ringsget.com","roia.biz",
      "rtbhouse.com","rtbsystem.org","ru4.com","s4m.io","scorecardresearch.com",
      "scribblelive.com","seeding-ads.com","segment.io","sekindo.com",
      "sellwild.com","seniormind.de","seoquake.com","serpwoo.com","sexad.net",
      "shareaholic.com","shareasale.com","sharethis.com","shorte.st","silverpop.com",
      "sitescout.com","skimresources.com","smaato.com","smartadserver.com","smartclip.net",
      "smartyads.com","snapads.com","snigelweb.com","sociallykeeda.com","socialprivacy.org",
      "softcube.com","sonobi.com","soo.gd","sparkflow.ai","spctrm.com","specless.tech",
      "speedcurve.com","spilgames.com","spinmedia.com","spotxchange.com","springserve.com",
      "sputnik-burst.info","stackadapt.com","steelhouse.com","strapad.com","streamrail.net",
      "sublimemedia.net","subusers.com","successfultogether.co.uk","sudoads.com",
      "sundaysky.com","superawesome.tv","supersonicads.com",
      "survata.com","syndopop.com","tabmo.io","taptica.com","teads.tv",
      "technoratimedia.com","telecoming.com","tentaculos.net",
      "theadx.com","theblogfrog.com","thebootube.com","thundertech.com","tibacta.com",
      "tidal.life","tiqcdn.com","tmsmedia.io","tonefuse.com","tradedoubler.com",
      "traffichaus.com","trafficstars.com","trekblue.com",
      "truoptik.com","tubemogul.com","turn.com","twiagos.com","tynt.com","uberads.com",
      "udtwenu.com","ultimamedia.com","unbounce.com","underdogmedia.com","undertone.com",
      "uniconsent.com","unilead.com","unruly.co","upravel.com","vado.tv","valueclickmedia.com",
      "vastserved.com","vertex-int.com","vibrantmedia.com","vidazoo.com","videoamp.com",
      "videobyte.com","vidoomy.com","viglink.com","viral-loops.com","virtusize.com",
      "visiblemeasures.com","viulife.com","voicefive.com","voluum.com","vrtcal.com",
      "vungle.com","wagawin.com","weborama.com",
      "whistleout.com","widgetserve.com","worldnaturenet.xyz","wp113.com",
      "xad.com","xaxis.com","xpanama.net","xplusone.com",
      "yashi.com","yieldivision.com","yieldlab.net","yieldlove.com","yieldmo.com",
      "yieldoptimizer.com","yoc.com","yodle.com","yoggrt.com","z4rtist.com",
      "zap.buzz","zeeto.io","zemanta.com","zeotap.com","zetaglobal.com","ziffdavis.com",
      "zipari.com","zmags.com","zprk.com","zymanga.net","zymanga.com"
    };
    for (String d : adg) blocked.add(d);` + "\n"
}

func getRec(id string) *record {
	buildsMu.RLock()
	defer buildsMu.RUnlock()
	return builds[id]
}

func newID() string {
	b := make([]byte, 8)
	rand.Read(b)
	return hex.EncodeToString(b)
}

func cleanPkg(s string) string {
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

func xmlEscape(s string) string {
	s = strings.ReplaceAll(s, "&", "&amp;")
	s = strings.ReplaceAll(s, "<", "&lt;")
	s = strings.ReplaceAll(s, ">", "&gt;")
	s = strings.ReplaceAll(s, "\"", "&quot;")
	s = strings.ReplaceAll(s, "'", "&apos;")
	return s
}

func safeName(s string) string {
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

func writeFile(path, content string) {
	os.MkdirAll(filepath.Dir(path), 0755)
	os.WriteFile(path, []byte(content), 0644)
}

func copyFile(dst, src string) {
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

func writeJSON(w http.ResponseWriter, code int, v interface{}) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(code)
	json.NewEncoder(w).Encode(v)
}

func env(k, def string) string {
	if v := os.Getenv(k); v != "" {
		return v
	}
	return def
}

func localIP() string {
	addrs, err := net.InterfaceAddrs()
	if err != nil {
		return ""
	}
	for _, a := range addrs {
		if ipnet, ok := a.(*net.IPNet); ok && !ipnet.IP.IsLoopback() && ipnet.IP.To4() != nil {
			return ipnet.IP.String()
		}
	}
	return ""
}

func readEnvPort() string {
	data, err := os.ReadFile(filepath.Join(baseDir, ".env"))
	if err != nil {
		return ""
	}
	for _, line := range strings.Split(string(data), "\n") {
		line = strings.TrimSpace(line)
		if strings.HasPrefix(line, "PORT=") {
			return strings.TrimPrefix(line, "PORT=")
		}
	}
	return ""
}

func saveEnvPort(port string) {
	envPath := filepath.Join(baseDir, ".env")
	data, err := os.ReadFile(envPath)
	lines := []string{}
	found := false
	if err == nil {
		for _, line := range strings.Split(string(data), "\n") {
			trimmed := strings.TrimSpace(line)
			if strings.HasPrefix(trimmed, "PORT=") {
				lines = append(lines, "PORT="+port)
				found = true
			} else if trimmed != "" {
				lines = append(lines, line)
			}
		}
	}
	if !found {
		lines = append(lines, "PORT="+port)
	}
	os.WriteFile(envPath, []byte(strings.Join(lines, "\n")+"\n"), 0644)
}
