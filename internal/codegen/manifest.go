package codegen

import (
	"fmt"

	"h2apk/internal/types"
	"h2apk/internal/util"
)

// genAndroidManifest produces the AndroidManifest.xml content for the APK.
// hasIcon controls whether the android:icon attribute is included.
func GenAndroidManifest(req types.BuildRequest, hasIcon bool) string {
	iconAttr := ""
	if hasIcon {
		iconAttr = ` android:icon="@mipmap/icon"`
	}

	var splashActivity string
	if req.SplashEnabled {
		splashActivity = `    <activity android:name="com.h2a.SplashActivity" android:exported="true" android:theme="@style/AppTheme">
      <intent-filter>
        <action android:name="android.intent.action.MAIN"/>
        <category android:name="android.intent.category.LAUNCHER"/>
      </intent-filter>
    </activity>
    <activity android:name="com.h2a.WebViewActivity" android:exported="false" android:theme="@style/AppTheme" android:configChanges="orientation|screenSize">
    </activity>`
	} else {
		splashActivity = `    <activity android:name="com.h2a.WebViewActivity" android:exported="true" android:theme="@style/AppTheme" android:configChanges="orientation|screenSize">
      <intent-filter>
        <action android:name="android.intent.action.MAIN"/>
        <category android:name="android.intent.category.LAUNCHER"/>
      </intent-filter>
    </activity>`
	}

	m := fmt.Sprintf(`<?xml version="1.0" encoding="utf-8"?>
<manifest xmlns:android="http://schemas.android.com/apk/res/android" package="%s">
  <uses-permission android:name="android.permission.INTERNET"/>
  <uses-permission android:name="android.permission.WRITE_EXTERNAL_STORAGE" android:maxSdkVersion="28"/>
  <uses-permission android:name="android.permission.POST_NOTIFICATIONS"/>`, req.PackageName)

	if req.CameraPermission {
		m += "\n  <uses-permission android:name=\"android.permission.CAMERA\"/>"
	}
	if req.MicPermission {
		m += "\n  <uses-permission android:name=\"android.permission.RECORD_AUDIO\"/>"
		m += "\n  <uses-permission android:name=\"android.permission.MODIFY_AUDIO_SETTINGS\"/>"
	}
	if req.GeoPermission {
		m += "\n  <uses-permission android:name=\"android.permission.ACCESS_FINE_LOCATION\"/>"
		m += "\n  <uses-permission android:name=\"android.permission.ACCESS_COARSE_LOCATION\"/>"
	}

	m += fmt.Sprintf(`
  <application android:label="%s"%s android:usesCleartextTraffic="%t">
%s
  </application>
</manifest>`, util.XmlEscape(req.AppName), iconAttr, req.AllowCleartext, splashActivity)

	return m
}
