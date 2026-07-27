package types

// Config holds paths to build tool JARs loaded from config.json.
type Config struct {
	D8Jar        string `json:"d8_jar"`
	ApkSignerJar string `json:"apksigner_jar"`
	AndroidJar   string `json:"android_jar"`
}

// BuildRequest is the JSON body sent to POST /api/build.
type BuildRequest struct {
	AppName     string `json:"app_name"`
	PackageName string `json:"package_name"`
	HTML        string `json:"html"`
	CSS         string `json:"css"`
	JS          string `json:"js"`
	URL         string `json:"url"`
	Icon        string `json:"icon"` // base64-encoded PNG

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

	// Auto-detected from HTML content; not user-supplied.
	CameraPermission bool `json:"-"`
	MicPermission    bool `json:"-"`
	NotifPermission  bool `json:"-"`
	GeoPermission    bool `json:"-"`

	KeystoreBase64 string            `json:"keystore"`
	KeystorePass   string            `json:"ks_pass"`
	KeyAlias       string            `json:"key_alias"`
	KeyPass        string            `json:"key_pass"`
	AssetFiles     map[string]string `json:"asset_files"` // filename -> base64
}

// BuildInfo is returned by /api/build and /api/status.
type BuildInfo struct {
	Success bool   `json:"success"`
	BuildID string `json:"build_id,omitempty"`
	APKName string `json:"apk_name,omitempty"`
	Error   string `json:"error,omitempty"`
	Log     string `json:"log,omitempty"`
}
