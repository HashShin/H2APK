package codegen

import (
	"fmt"

	"h2apk/internal/types"
)

// webViewActivityParams holds all substitution values for genWebViewActivitySrc.
type WebViewActivityParams struct {
	PermImports           string
	DisableCopyImplements string
	IndicatorField        string
	PermFields            string
	FlBg                  string
	ThemeColorInt         int
	HideScrollbars        bool
	GeoPermission         bool
	PermSettings          string
	ZoomEnabled           bool
	BlockAds              bool
	AdGuardDNS            bool
	ClientCreate          string
	NotifInterface        string
	AssetInit             string
	LoadURL               string
	PullInit              string
	PermOnCreate          string
	DisableCopyInit       string
	DisableCopyMethod     string
	PermMethods           string
	FileChooserMethods    string
}

// genChromeClientSrc returns the full source of H2AChromeClient.java.
// When camera/mic/geo permissions are needed the full permission-handling variant
// is generated; otherwise a geo-only stub is returned.
func GenChromeClientSrc(req types.BuildRequest, themeColorInt int) string {
	var body string
	if req.CameraPermission || req.MicPermission || req.GeoPermission {
		camFlag := "false"
		if req.CameraPermission {
			camFlag = "true"
		}
		micFlag := "false"
		if req.MicPermission {
			micFlag = "true"
		}
		body = fmt.Sprintf(`
import android.webkit.PermissionRequest;

public class H2AChromeClient extends WebChromeClient implements android.content.DialogInterface.OnClickListener {
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
  private static final int REQ_GEO = 1003;
  private int themeColor = (int)%dL;
  private String pendingGeoOrigin;
  private android.webkit.GeolocationPermissions.Callback pendingGeoCallback;
  private android.webkit.JsResult pendingJsResult;
  private android.webkit.JsPromptResult pendingPromptResult;
  private android.widget.EditText promptInput;

  public H2AChromeClient(FrameLayout container, WebView webView, int themeColor) {
    this.container = container;
    this.webView = webView;
    this.activity = (WebViewActivity) webView.getContext();
    this.themeColor = themeColor;
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

  @Override
  public boolean onShowFileChooser(WebView webView,
      android.webkit.ValueCallback<android.net.Uri[]> filePathCallback,
      WebChromeClient.FileChooserParams fileChooserParams) {
    String[] accept = null;
    try { accept = fileChooserParams.getAcceptTypes(); } catch (Exception e) {}
    if (activity == null) { try { activity = (WebViewActivity) webView.getContext(); } catch (Exception e) {} }
    if (activity == null) { filePathCallback.onReceiveValue(null); return true; }
    activity.launchFileChooser(filePathCallback, accept);
    return true;
  }

  @Override
  public void onClick(android.content.DialogInterface dialog, int which) {
    if (which == android.content.DialogInterface.BUTTON_POSITIVE) {
      if (pendingPromptResult != null) {
        String val = promptInput != null ? promptInput.getText().toString() : "";
        pendingPromptResult.confirm(val);
      } else if (pendingJsResult != null) {
        pendingJsResult.confirm();
      }
    } else {
      if (pendingPromptResult != null) pendingPromptResult.cancel();
      else if (pendingJsResult != null) pendingJsResult.cancel();
    }
    pendingJsResult = null;
    pendingPromptResult = null;
    promptInput = null;
  }

  private android.app.AlertDialog buildDialog(android.content.Context ctx, String title, String msg) {
    android.view.ContextThemeWrapper themed = new android.view.ContextThemeWrapper(ctx, android.R.style.Theme_Material_Dialog);
    android.app.AlertDialog.Builder b = new android.app.AlertDialog.Builder(themed);
    if (title != null && title.length() > 0) b.setTitle(title);
    if (msg != null) b.setMessage(msg);
    b.setCancelable(false);
    return b.create();
  }

  @Override
  public boolean onJsAlert(WebView view, String url, String message, android.webkit.JsResult result) {
    pendingJsResult = result;
    android.app.AlertDialog d = buildDialog(view.getContext(), null, message);
    d.setButton(android.content.DialogInterface.BUTTON_POSITIVE, "OK", this);
    d.show();
    d.getButton(android.content.DialogInterface.BUTTON_POSITIVE).setTextColor(themeColor);
    return true;
  }

  @Override
  public boolean onJsConfirm(WebView view, String url, String message, android.webkit.JsResult result) {
    pendingJsResult = result;
    android.app.AlertDialog d = buildDialog(view.getContext(), null, message);
    d.setButton(android.content.DialogInterface.BUTTON_POSITIVE, "OK", this);
    d.setButton(android.content.DialogInterface.BUTTON_NEGATIVE, "Cancel", this);
    d.show();
    d.getButton(android.content.DialogInterface.BUTTON_POSITIVE).setTextColor(themeColor);
    d.getButton(android.content.DialogInterface.BUTTON_NEGATIVE).setTextColor(themeColor);
    return true;
  }

  @Override
  public boolean onJsPrompt(WebView view, String url, String message, String defaultValue, android.webkit.JsPromptResult result) {
    pendingPromptResult = result;
    android.widget.EditText input = new android.widget.EditText(view.getContext());
    input.setText(defaultValue != null ? defaultValue : "");
    promptInput = input;
    android.app.AlertDialog d = buildDialog(view.getContext(), message, null);
    d.setView(input);
    d.setButton(android.content.DialogInterface.BUTTON_POSITIVE, "OK", this);
    d.setButton(android.content.DialogInterface.BUTTON_NEGATIVE, "Cancel", this);
    d.show();
    d.getButton(android.content.DialogInterface.BUTTON_POSITIVE).setTextColor(themeColor);
    d.getButton(android.content.DialogInterface.BUTTON_NEGATIVE).setTextColor(themeColor);
    return true;
  }

  @Override
  public void onGeolocationPermissionsShowPrompt(String origin, android.webkit.GeolocationPermissions.Callback callback) {
    if (android.os.Build.VERSION.SDK_INT >= 23 &&
        activity.checkSelfPermission(android.Manifest.permission.ACCESS_FINE_LOCATION)
          != android.content.pm.PackageManager.PERMISSION_GRANTED) {
      pendingGeoOrigin = origin;
      pendingGeoCallback = callback;
      activity.reRequestPermission(android.Manifest.permission.ACCESS_FINE_LOCATION, REQ_GEO);
    } else {
      callback.invoke(origin, true, false);
    }
  }

  public void onGeoPermissionResult(boolean granted) {
    if (pendingGeoCallback != null) {
      pendingGeoCallback.invoke(pendingGeoOrigin, granted, false);
      pendingGeoCallback = null;
      pendingGeoOrigin = null;
    }
  }
}`, camFlag, micFlag, themeColorInt)
	} else {
		body = `
public class H2AChromeClient extends WebChromeClient implements android.content.DialogInterface.OnClickListener {
  private View customView;
  private CustomViewCallback callback;
  private FrameLayout container;
  private WebView webView;
  private WebViewActivity activity;
  private int themeColor;
  private android.webkit.JsResult pendingJsResult;
  private android.webkit.JsPromptResult pendingPromptResult;
  private android.widget.EditText promptInput;
  private static final int REQ_GEO = 1003;
  private String pendingGeoOrigin;
  private android.webkit.GeolocationPermissions.Callback pendingGeoCallback;

  public H2AChromeClient(FrameLayout container, WebView webView, int themeColor) {
    this.container = container;
    this.webView = webView;
    this.activity = (WebViewActivity) webView.getContext();
    this.themeColor = themeColor;
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
  public boolean onShowFileChooser(WebView webView,
      android.webkit.ValueCallback<android.net.Uri[]> filePathCallback,
      WebChromeClient.FileChooserParams fileChooserParams) {
    String[] accept = null;
    try { accept = fileChooserParams.getAcceptTypes(); } catch (Exception e) {}
    if (activity == null) { try { activity = (WebViewActivity) webView.getContext(); } catch (Exception e) {} }
    if (activity == null) { filePathCallback.onReceiveValue(null); return true; }
    activity.launchFileChooser(filePathCallback, accept);
    return true;
  }

  @Override
  public void onClick(android.content.DialogInterface dialog, int which) {
    if (which == android.content.DialogInterface.BUTTON_POSITIVE) {
      if (pendingPromptResult != null) {
        String val = promptInput != null ? promptInput.getText().toString() : "";
        pendingPromptResult.confirm(val);
      } else if (pendingJsResult != null) {
        pendingJsResult.confirm();
      }
    } else {
      if (pendingPromptResult != null) pendingPromptResult.cancel();
      else if (pendingJsResult != null) pendingJsResult.cancel();
    }
    pendingJsResult = null;
    pendingPromptResult = null;
    promptInput = null;
  }

  private android.app.AlertDialog buildDialog(android.content.Context ctx, String title, String msg) {
    android.view.ContextThemeWrapper themed = new android.view.ContextThemeWrapper(ctx, android.R.style.Theme_Material_Dialog);
    android.app.AlertDialog.Builder b = new android.app.AlertDialog.Builder(themed);
    if (title != null && title.length() > 0) b.setTitle(title);
    if (msg != null) b.setMessage(msg);
    b.setCancelable(false);
    return b.create();
  }

  @Override
  public boolean onJsAlert(WebView view, String url, String message, android.webkit.JsResult result) {
    pendingJsResult = result;
    android.app.AlertDialog d = buildDialog(view.getContext(), null, message);
    d.setButton(android.content.DialogInterface.BUTTON_POSITIVE, "OK", this);
    d.show();
    d.getButton(android.content.DialogInterface.BUTTON_POSITIVE).setTextColor(themeColor);
    return true;
  }

  @Override
  public boolean onJsConfirm(WebView view, String url, String message, android.webkit.JsResult result) {
    pendingJsResult = result;
    android.app.AlertDialog d = buildDialog(view.getContext(), null, message);
    d.setButton(android.content.DialogInterface.BUTTON_POSITIVE, "OK", this);
    d.setButton(android.content.DialogInterface.BUTTON_NEGATIVE, "Cancel", this);
    d.show();
    d.getButton(android.content.DialogInterface.BUTTON_POSITIVE).setTextColor(themeColor);
    d.getButton(android.content.DialogInterface.BUTTON_NEGATIVE).setTextColor(themeColor);
    return true;
  }

  @Override
  public boolean onJsPrompt(WebView view, String url, String message, String defaultValue, android.webkit.JsPromptResult result) {
    pendingPromptResult = result;
    android.widget.EditText input = new android.widget.EditText(view.getContext());
    input.setText(defaultValue != null ? defaultValue : "");
    promptInput = input;
    android.app.AlertDialog d = buildDialog(view.getContext(), message, null);
    d.setView(input);
    d.setButton(android.content.DialogInterface.BUTTON_POSITIVE, "OK", this);
    d.setButton(android.content.DialogInterface.BUTTON_NEGATIVE, "Cancel", this);
    d.show();
    d.getButton(android.content.DialogInterface.BUTTON_POSITIVE).setTextColor(themeColor);
    d.getButton(android.content.DialogInterface.BUTTON_NEGATIVE).setTextColor(themeColor);
    return true;
  }

  @Override
  public void onGeolocationPermissionsShowPrompt(String origin, android.webkit.GeolocationPermissions.Callback callback) {
    if (android.os.Build.VERSION.SDK_INT >= 23 &&
        activity.checkSelfPermission(android.Manifest.permission.ACCESS_FINE_LOCATION)
          != android.content.pm.PackageManager.PERMISSION_GRANTED) {
      pendingGeoOrigin = origin;
      pendingGeoCallback = callback;
      activity.reRequestPermission(android.Manifest.permission.ACCESS_FINE_LOCATION, REQ_GEO);
    } else {
      callback.invoke(origin, true, false);
    }
  }

  public void onGeoPermissionResult(boolean granted) {
    if (pendingGeoCallback != null) {
      pendingGeoCallback.invoke(pendingGeoOrigin, granted, false);
      pendingGeoCallback = null;
      pendingGeoOrigin = null;
    }
  }

  public void onPermissionResult(int requestCode, boolean granted) {
    // Stub: camera/mic permissions not requested in this build
  }
}`
	}
	return `package com.h2a;
import android.webkit.WebChromeClient;
import android.webkit.WebView;
import android.view.View;
import android.view.ViewGroup;
import android.widget.FrameLayout;
import android.app.AlertDialog;
import android.content.DialogInterface;
import android.widget.EditText;` + body
}

// genWebViewActivitySrc returns the full source of WebViewActivity.java.
func GenWebViewActivitySrc(p WebViewActivityParams) string {
	return fmt.Sprintf(`package com.h2a;
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
import android.util.Log;
import android.webkit.ValueCallback;%s
public class WebViewActivity extends Activity implements DownloadListener%s {
  private WebView wv;
  private H2AChromeClient chromeClient;
  private long lastBackPress = 0;
  private Toast toast;
  private ValueCallback<Uri[]> filePathCallback;
  private static final int H2A_FILE_CHOOSER_REQ = 3001;
  %s
  %s

  @Override
  protected void onCreate(Bundle savedInstanceState) {
    super.onCreate(savedInstanceState);
    getWindow().getDecorView().setBackgroundColor(%s);
    try { WebView.setWebContentsDebuggingEnabled(true); } catch (Exception ignored) {}
    if (android.os.Build.VERSION.SDK_INT >= 21) {
      getWindow().addFlags(android.view.WindowManager.LayoutParams.FLAG_DRAWS_SYSTEM_BAR_BACKGROUNDS);
      getWindow().setStatusBarColor((int)%dL);
    }
    wv = new WebView(this);
    wv.setBackgroundColor(%s);
    wv.setVerticalScrollBarEnabled(%t);
    wv.setHorizontalScrollBarEnabled(%t);
    WebSettings ws = wv.getSettings();
    ws.setJavaScriptEnabled(true);
    ws.setDomStorageEnabled(true);
    ws.setGeolocationEnabled(%t);
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
    }
    FrameLayout fl = new FrameLayout(this);
    wv.setWebViewClient(%s);
    chromeClient = new H2AChromeClient(fl, wv, (int)%dL);
    wv.setWebChromeClient(chromeClient);
    wv.setDownloadListener(this);
    wv.addJavascriptInterface(new ClipboardHelper(this), "H2AClip");
    wv.addJavascriptInterface(new FileHelper(this), "H2AFile");
    wv.addJavascriptInterface(new ShareHelper(this), "H2AShare");
    wv.addJavascriptInterface(new TTSHelper(this), "H2ATTS");
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
    if (url != null && url.startsWith("blob:")) {
      final String safeFilename = URLUtil.guessFileName(url, contentDisposition, mimeType).replace("'", "\\'");
      final String safeMime = (mimeType == null ? "" : mimeType).replace("'", "\\'");
      final String safeUrl = url.replace("\\", "\\\\").replace("'", "\\'");
      wv.evaluateJavascript(
        "(function(){" +
        "fetch('" + safeUrl + "')" +
        ".then(function(r){return r.blob();})" +
        ".then(function(b){" +
        "var fr=new FileReader();" +
        "fr.onload=function(){" +
        "var b64=fr.result.indexOf(',')>=0?fr.result.split(',')[1]:fr.result;" +
        "var mt=b.type||'" + safeMime + "';" +
        "if(window.H2AFile)H2AFile.saveBase64File(b64,'" + safeFilename + "',mt);" +
        "};" +
        "fr.readAsDataURL(b);" +
        "}).catch(function(e){console.error('blob dl',e);});" +
        "})()", null);
      return;
    }
    try {
      DownloadManager.Request req = new DownloadManager.Request(Uri.parse(url));
      req.setMimeType(mimeType);
      req.addRequestHeader("User-Agent", userAgent);
      req.setNotificationVisibility(DownloadManager.Request.VISIBILITY_VISIBLE_NOTIFY_COMPLETED);
      String name = URLUtil.guessFileName(url, contentDisposition, mimeType);
      req.setTitle(name);
      req.setDestinationInExternalPublicDir(Environment.DIRECTORY_DOWNLOADS, name);
      DownloadManager dm = (DownloadManager) getSystemService(Context.DOWNLOAD_SERVICE);
      if (dm != null) dm.enqueue(req);
    } catch (Exception e) {
      Toast.makeText(this, "Download failed: " + e.getMessage(), Toast.LENGTH_LONG).show();
    }
  }%s
  %s
  %s
}`,
		p.PermImports, p.DisableCopyImplements,
		p.IndicatorField, p.PermFields,
		p.FlBg, p.ThemeColorInt, p.FlBg,
		!p.HideScrollbars, !p.HideScrollbars,
		p.GeoPermission,
		p.PermSettings,
		p.ZoomEnabled,
		p.BlockAds || p.AdGuardDNS, p.BlockAds || p.AdGuardDNS,
		p.ClientCreate, p.ThemeColorInt,
		p.NotifInterface, p.AssetInit,
		p.LoadURL, p.FlBg,
		p.PullInit, p.PermOnCreate, p.DisableCopyInit,
		p.DisableCopyMethod, p.PermMethods, p.FileChooserMethods,
	)
}

// genSplashActivitySrc returns the full source of SplashActivity.java.
func GenSplashActivitySrc(req types.BuildRequest) string {
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
	}

	return fmt.Sprintf(`package com.h2a;
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
    try {
      getWindow().getDecorView().setBackgroundColor(Color.parseColor("%s"));
    } catch (Exception e) {
      getWindow().getDecorView().setBackgroundColor(0xFF000000);
    }
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
}`, animImports, bgColor, bgColor, imgPct, animCode, duration, exitAnim)
}

// genPullIndicatorSrc returns the full source of PullIndicator.java.
func GenPullIndicatorSrc() string {
	return `package com.h2a;
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
    setAlpha(0.4f + 0.6f * progress);
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

        float tx = -sin;
        float ty = cos;
        float nx = cos;
        float ny = sin;

        float px = cx + R * cos;
        float py = cy + R * sin;

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
}`
}

// genPullListenerSrc returns the full source of PullListener.java.
func GenPullListenerSrc() string {
	return `package com.h2a;
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
    this.threshold = 115 * d;
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
}`
}

// genClipboardHelperSrc returns the full source of ClipboardHelper.java.
func GenClipboardHelperSrc() string {
	return `package com.h2a;
import android.app.Activity;
import android.content.ClipboardManager;
import android.content.ClipData;
import android.content.Context;
import android.webkit.JavascriptInterface;
public class ClipboardHelper implements Runnable {
  private Activity activity;
  private volatile String pendingWrite;
  private volatile String readResult;
  private final Object lock = new Object();
  private volatile boolean done;
  private int mode;
  public ClipboardHelper(Activity a) { this.activity = a; }
  @JavascriptInterface
  public String readText() {
    synchronized (lock) {
      mode = 0; done = false; readResult = "";
      activity.runOnUiThread(this);
      long start = System.currentTimeMillis();
      while (!done && System.currentTimeMillis() - start < 1000) {
        try { lock.wait(1000); } catch (InterruptedException e) { break; }
      }
      return readResult;
    }
  }
  @JavascriptInterface
  public void writeText(String text) {
    synchronized (lock) { mode = 1; pendingWrite = text == null ? "" : text; }
    activity.runOnUiThread(this);
  }
  public void run() {
    ClipboardManager cm = (ClipboardManager) activity.getSystemService(Context.CLIPBOARD_SERVICE);
    if (mode == 0) {
      String r = "";
      try {
        if (cm != null && cm.hasPrimaryClip() && cm.getPrimaryClip() != null
            && cm.getPrimaryClip().getItemCount() > 0) {
          CharSequence cs = cm.getPrimaryClip().getItemAt(0).coerceToText(activity);
          r = cs == null ? "" : cs.toString();
        }
      } catch (Exception e) {}
      synchronized (lock) { readResult = r; done = true; lock.notifyAll(); }
    } else {
      try { if (cm != null) cm.setPrimaryClip(ClipData.newPlainText("text", pendingWrite)); } catch (Exception e) {}
    }
  }
}`
}

// genFileHelperSrc returns the full source of FileHelper.java.
func GenFileHelperSrc() string {
	return `package com.h2a;
import android.app.Activity;
import android.app.Notification;
import android.app.NotificationChannel;
import android.app.NotificationManager;
import android.content.ContentValues;
import android.os.Build;
import android.os.Environment;
import android.provider.MediaStore;
import android.webkit.JavascriptInterface;
import android.widget.Toast;
import java.io.File;
import java.io.FileOutputStream;
import java.io.OutputStream;
import android.util.Base64;
import java.util.concurrent.atomic.AtomicInteger;
public class FileHelper implements Runnable {
  private static final String CHANNEL_ID = "h2a_downloads";
  private static final AtomicInteger notifIdCounter = new AtomicInteger(3000);
  private Activity activity;
  private volatile String pendingBase64;
  private volatile String pendingFilename;
  private volatile String pendingMime;
  public FileHelper(Activity a) { this.activity = a; }
  @JavascriptInterface
  public void saveBase64File(String base64, String filename, String mimeType) {
    pendingBase64 = base64;
    pendingFilename = filename;
    pendingMime = mimeType == null || mimeType.isEmpty() ? "application/octet-stream" : mimeType;
    new Thread(this).start();
  }
  private NotificationManager getNotifManager() {
    return (NotificationManager) activity.getSystemService(NotificationManager.class);
  }
  private void ensureChannel() {
    if (Build.VERSION.SDK_INT >= 26) {
      NotificationManager nm = getNotifManager();
      if (nm.getNotificationChannel(CHANNEL_ID) == null) {
        NotificationChannel ch = new NotificationChannel(CHANNEL_ID, "Downloads", NotificationManager.IMPORTANCE_LOW);
        ch.setSound(null, null);
        nm.createNotificationChannel(ch);
      }
    }
  }
  static class ToastRunner implements Runnable {
    private Activity activity;
    private String msg;
    private boolean long_;
    ToastRunner(Activity a, String m, boolean l) { activity=a; msg=m; long_=l; }
    public void run() {
      Toast.makeText(activity, msg, long_ ? Toast.LENGTH_LONG : Toast.LENGTH_SHORT).show();
    }
  }
  public void run() {
    final String filename = pendingFilename;
    final String mime = pendingMime;
    final byte[] data;
    try {
      data = Base64.decode(pendingBase64, Base64.DEFAULT);
    } catch (Exception e) {
      activity.runOnUiThread(new ToastRunner(activity, "Download failed: " + e.getMessage(), true));
      return;
    }
    ensureChannel();
    final int notifId = notifIdCounter.getAndIncrement();
    final NotificationManager nm = getNotifManager();
    int iconRes = activity.getResources().getIdentifier("icon", "mipmap", activity.getPackageName());
    Notification.Builder builder;
    if (Build.VERSION.SDK_INT >= 26) {
      builder = new Notification.Builder(activity, CHANNEL_ID);
    } else {
      builder = new Notification.Builder(activity);
    }
    builder.setContentTitle(filename)
           .setContentText("Downloading...")
           .setSmallIcon(android.R.drawable.stat_sys_download)
           .setOngoing(true)
           .setProgress(100, 0, true);
    if (iconRes != 0) builder.setLargeIcon(android.graphics.BitmapFactory.decodeResource(activity.getResources(), iconRes));
    nm.notify(notifId, builder.build());
    try {
      if (Build.VERSION.SDK_INT >= 29) {
        ContentValues cv = new ContentValues();
        cv.put(MediaStore.Downloads.DISPLAY_NAME, filename);
        cv.put(MediaStore.Downloads.MIME_TYPE, mime);
        cv.put(MediaStore.Downloads.IS_PENDING, 1);
        android.net.Uri uri = activity.getContentResolver().insert(MediaStore.Downloads.EXTERNAL_CONTENT_URI, cv);
        if (uri != null) {
          OutputStream os = activity.getContentResolver().openOutputStream(uri);
          if (os != null) { os.write(data); os.close(); }
          cv.clear();
          cv.put(MediaStore.Downloads.IS_PENDING, 0);
          activity.getContentResolver().update(uri, cv, null, null);
        }
      } else {
        File dir = Environment.getExternalStoragePublicDirectory(Environment.DIRECTORY_DOWNLOADS);
        dir.mkdirs();
        File f = new File(dir, filename);
        FileOutputStream fos = new FileOutputStream(f);
        fos.write(data);
        fos.close();
      }
      Notification.Builder doneBuilder;
      if (Build.VERSION.SDK_INT >= 26) {
        doneBuilder = new Notification.Builder(activity, CHANNEL_ID);
      } else {
        doneBuilder = new Notification.Builder(activity);
      }
      long bytes = data.length;
      String sizeStr;
      if (bytes >= 1048576) sizeStr = String.format("%.1f MB", bytes / 1048576.0);
      else if (bytes >= 1024) sizeStr = String.format("%.1f KB", bytes / 1024.0);
      else sizeStr = bytes + " B";
      doneBuilder.setContentTitle(filename)
                 .setContentText("Download complete · " + sizeStr)
                 .setSmallIcon(android.R.drawable.stat_sys_download_done)
                 .setOngoing(false)
                 .setProgress(0, 0, false)
                 .setAutoCancel(true);
      if (iconRes != 0) doneBuilder.setLargeIcon(android.graphics.BitmapFactory.decodeResource(activity.getResources(), iconRes));
      if (Build.VERSION.SDK_INT >= 21) doneBuilder.setStyle(new Notification.BigTextStyle().bigText("Download complete · " + sizeStr));
      nm.notify(notifId, doneBuilder.build());
      activity.runOnUiThread(new ToastRunner(activity, "Saved: " + filename, false));
    } catch (Exception e) {
      nm.cancel(notifId);
      activity.runOnUiThread(new ToastRunner(activity, "Save failed: " + e.getMessage(), true));
    }
  }
}`
}

// genNotificationHelperSrc returns the full source of NotificationHelper.java.
func GenNotificationHelperSrc() string {
	return `package com.h2a;
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
}`
}

// genTTSHelperSrc returns the full source of TTSHelper.java.
func GenTTSHelperSrc() string {
	return `package com.h2a;
import android.app.Activity;
import android.speech.tts.TextToSpeech;
import android.speech.tts.UtteranceProgressListener;
import android.webkit.JavascriptInterface;
import java.util.HashMap;
import java.util.Locale;

public class TTSHelper implements TextToSpeech.OnInitListener {
  private Activity activity;
  private TextToSpeech tts;
  private boolean ready = false;
  private String pendingText = null;

  public TTSHelper(Activity a) {
    this.activity = a;
    tts = new TextToSpeech(a, this);
  }

  @Override
  public void onInit(int status) {
    if (status == TextToSpeech.SUCCESS) {
      tts.setLanguage(Locale.getDefault());
      ready = true;
      if (pendingText != null) { doSpeak(pendingText); pendingText = null; }
    }
  }

  @JavascriptInterface
  public void speak(String text) {
    if (ready) { doSpeak(text); } else { pendingText = text; }
  }

  @JavascriptInterface
  public boolean isReady() { return ready; }

  @JavascriptInterface
  public void stop() { if (tts != null) tts.stop(); }

  private void doSpeak(String text) {
    if (android.os.Build.VERSION.SDK_INT >= 21) {
      tts.speak(text, TextToSpeech.QUEUE_FLUSH, null, "h2a_" + System.currentTimeMillis());
    } else {
      HashMap<String,String> params = new HashMap<>();
      params.put(TextToSpeech.Engine.KEY_PARAM_UTTERANCE_ID, "h2a");
      tts.speak(text, TextToSpeech.QUEUE_FLUSH, params);
    }
  }
}`
}

// genShareHelperSrc returns the full source of ShareHelper.java.
func GenShareHelperSrc() string {
	return `package com.h2a;
import android.app.Activity;
import android.webkit.JavascriptInterface;

public class ShareHelper {
  private Activity activity;
  public ShareHelper(Activity a){ this.activity = a; }

  @JavascriptInterface
  public void share(String title, String text, String url) {
    StringBuilder sb = new StringBuilder();
    if (text != null && text.length() > 0) sb.append(text);
    if (url != null && url.length() > 0) { if (sb.length()>0) sb.append("\n"); sb.append(url); }
    android.content.Intent i = new android.content.Intent(android.content.Intent.ACTION_SEND);
    i.setType("text/plain");
    if (title != null && title.length() > 0) i.putExtra(android.content.Intent.EXTRA_SUBJECT, title);
    i.putExtra(android.content.Intent.EXTRA_TEXT, sb.toString());
    android.content.Intent chooser = android.content.Intent.createChooser(i, title != null ? title : "Share");
    chooser.addFlags(android.content.Intent.FLAG_ACTIVITY_NEW_TASK);
    activity.startActivity(chooser);
  }
}`
}

// genPaddingClient generates the PaddingClient.java source with three variants:
//   - no ads, no asset loader (minimal)
//   - no ads, with asset loader
//   - ad blocking (+ optional AdGuard DNS)
func GenPaddingClient(blockAds bool, adguardDNS bool, useAssetLoader bool) string {
	if !blockAds {
		if useAssetLoader {
			return `package com.h2a;
import android.webkit.WebResourceResponse;
import android.webkit.WebView;
import android.webkit.WebViewClient;
import android.util.Log;
import java.io.InputStream;
import java.io.ByteArrayInputStream;
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
      if (!url.startsWith("file:///android_asset/")) return null;
      String path = url.substring("file:///android_asset/".length());
      if (path == null || path.isEmpty() || "/".equals(path)) path = "/index.html";
      if (path.startsWith("/")) path = path.substring(1);
      if (path.endsWith("/")) {
        path = path + "index.html";
      } else {
        try {
          InputStream test = view.getContext().getAssets().open(path);
          test.close();
        } catch (Exception e) {
          path = path + "/index.html";
        }
      }
      InputStream is = view.getContext().getAssets().open(path);
      return new WebResourceResponse(guessMime(path), null, is);
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
    return true;
  }
  private boolean handleNavigation(WebView view, String url, boolean hasGesture) {
    if (!blockAds || url == null) { view.loadUrl(url); return true; }
    String cur = view.getUrl();
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
      if (!url.startsWith("file:///android_asset/")) return null;
      String path = url.substring("file:///android_asset/".length());
      if (path == null || path.isEmpty() || "/".equals(path)) path = "/index.html";
      if (path.startsWith("/")) path = path.substring(1);
      if (path.endsWith("/")) {
        path = path + "index.html";
      } else {
        try {
          InputStream test = view.getContext().getAssets().open(path);
          test.close();
        } catch (Exception e) {
          path = path + "/index.html";
        }
      }
      InputStream is = view.getContext().getAssets().open(path);
      return new WebResourceResponse(guessMime(path), null, is);
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

// adguardBlocklist returns the Java snippet that populates the AdGuard DNS blocklist,
// or an empty string when adguardDNS is false.
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
