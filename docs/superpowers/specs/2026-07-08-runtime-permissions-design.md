# Runtime Permissions — Camera, Microphone, Notifications

**Date:** 2026-07-08
**Status:** approved

## Summary

Add three runtime permission toggles to the Advanced section: Camera, Microphone, and Notifications. The generated APK declares the corresponding manifest permissions, requests them at runtime on startup, and re-prompts when WebView JS triggers a permission that was previously denied.

## Motivation

H2APK currently only declares `INTERNET` and `WRITE_EXTERNAL_STORAGE` (capped at SDK 28). There is no way to build an APK that can use the camera (e.g., `getUserMedia` for barcode scanning, video calls), microphone, or post system notifications. These are common web app needs that require Android manifest declarations plus runtime permission handling.

## Design

### UI — three new checkboxes in Advanced

Insert between "Disable copy text" and "Splash screen":

| ID | Label | Default |
|---|---|---|
| `cameraPermission` | Camera access | unchecked |
| `micPermission` | Microphone access | unchecked |
| `notifPermission` | Notification permission | unchecked |

### Backend — main.go changes

**BuildRequest struct** — three new bool fields:
- `CameraPermission bool` (JSON: `camera_permission`)
- `MicrophonePermission bool` (JSON: `mic_permission`)
- `NotificationPermission bool` (JSON: `notif_permission`)

**Manifest template** — conditionally inject `<uses-permission>` lines:
- `android.permission.CAMERA`
- `android.permission.RECORD_AUDIO`
- `android.permission.POST_NOTIFICATIONS`

**WebViewActivity** — runtime permission logic:
- In `onCreate`: build a list of needed permissions based on build flags; check each with `checkSelfPermission`; call `requestPermissions` for any not yet granted
- `onRequestPermissionsResult` override: track which were granted; if a pending WebView `PermissionRequest` exists, grant or deny it
- Method `reRequestPermission(String perm, int code)`: re-request a single permission, called by ChromeClient when JS triggers a previously-denied permission

**H2AChromeClient** — override `onPermissionRequest`:
- Check `checkSelfPermission` for the corresponding Android permission
- If granted: call `request.grant(request.getResources())`
- If denied: store the `PermissionRequest`, call back to Activity to re-request
- Method `onPermissionResult(boolean granted)`: grant or deny the stored request

### Permission-specific behavior

**Camera:** `CAMERA` permission maps to `PermissionRequest.RESOURCE_VIDEO_CAPTURE`
**Microphone:** `RECORD_AUDIO` permission maps to `PermissionRequest.RESOURCE_AUDIO_CAPTURE`
**Both checked together:** ChromeClient handles both resources in one `onPermissionRequest` call

**Notifications (Android 13+):** `POST_NOTIFICATIONS` is requested at startup only. No WebView JS path triggers it — it is not a `PermissionRequest` resource. Guarded by `Build.VERSION.SDK_INT >= 33`.

### Edge cases

- **"Never ask again":** `shouldShowRequestPermissionRationale` returns false. Activity shows a brief Toast explaining why the permission is needed, then still calls `requestPermissions` (system will show settings redirect dialog).
- **Camera without Microphone:** ChromeClient `onPermissionRequest` checks resources array — only requests the Android permissions that match the requested WebView resources.
- **Notifications on API < 33:** silently skipped, no-op. The checkbox still works but produces no manifest entry or runtime dialog.
- **Multiple rapid `onPermissionRequest` calls:** only one pending `PermissionRequest` stored at a time; newer replaces older.

### Files changed

| File | Changes |
|---|---|
| `assets/static/index.html` | 3 new checkboxes + form submission |
| `main.go` | BuildRequest fields, manifest template, WebViewActivity template, H2AChromeClient template |
