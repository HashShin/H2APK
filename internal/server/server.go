package server

import (
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"os"
	"path/filepath"
	"strings"

	"h2apk/internal/build"
	"h2apk/internal/types"
	"h2apk/internal/util"
)

// Server holds the build registry and builder used by HTTP handlers.
type Server struct {
	Reg     *build.Registry
	Builder *build.Builder
}

// RegisterRoutes registers all API routes on mux.
func (s *Server) RegisterRoutes(mux *http.ServeMux) {
	mux.HandleFunc("/api/build", s.handleBuild)
	mux.HandleFunc("/api/status/", s.handleStatus)
	mux.HandleFunc("/api/download/", s.handleDownload)
	mux.HandleFunc("/api/log/", s.handleLogStream)
}

func (s *Server) handleBuild(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "POST only", http.StatusMethodNotAllowed)
		return
	}
	var req types.BuildRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		util.WriteJSON(w, 400, types.BuildInfo{Success: false, Error: "bad request: " + err.Error()})
		return
	}
	log.Printf("Build request: zoom=%t blockAds=%t adGuard=%t url=%q", req.ZoomEnabled, req.BlockAds, req.AdGuardDNS, req.URL)
	if strings.TrimSpace(req.HTML) == "" && strings.TrimSpace(req.URL) == "" {
		util.WriteJSON(w, 400, types.BuildInfo{Success: false, Error: "HTML or URL is required"})
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
	req.PackageName = util.CleanPkg(req.PackageName)
	if !strings.Contains(req.PackageName, ".") {
		http.Error(w, "invalid package name: must contain at least one dot (e.g. com.example.app)", 400)
		return
	}

	id := util.NewID()
	s.Reg.Create(id)

	// Auto-detect camera/mic/notification needs from content.
	if isURL {
		req.CameraPermission = true
		req.MicPermission = true
		req.NotifPermission = true
		req.GeoPermission = true
	} else {
		content := strings.ToLower(req.HTML + req.CSS + req.JS)
		for _, data := range req.AssetFiles {
			content += strings.ToLower(data)
		}
		if strings.Contains(content, "getusermedia") {
			req.CameraPermission = strings.Contains(content, "{video") || strings.Contains(content, "video:") || strings.Contains(content, "video :")
			req.MicPermission = strings.Contains(content, "{audio") || strings.Contains(content, "audio:") || strings.Contains(content, "audio :")
		}
		req.NotifPermission = strings.Contains(content, "notification.requestpermission") ||
			strings.Contains(content, "new notification(") ||
			strings.Contains(content, "notification.permission") ||
			strings.Contains(content, "h2a.shownotification") ||
			strings.Contains(content, "h2a.requestnotificationpermission") ||
			strings.Contains(content, "h2a.getnotificationpermission")
		req.GeoPermission = strings.Contains(content, "navigator.geolocation") ||
			strings.Contains(content, "getcurrentposition") ||
			strings.Contains(content, "watchposition")
	}
	log.Printf("Auto-detect: camera=%t mic=%t notif=%t geo=%t isURL=%t", req.CameraPermission, req.MicPermission, req.NotifPermission, req.GeoPermission, isURL)
	go s.Builder.Build(id, req, isURL)
	util.WriteJSON(w, 202, types.BuildInfo{Success: true, BuildID: id})
}

func (s *Server) handleStatus(w http.ResponseWriter, r *http.Request) {
	id := strings.TrimPrefix(r.URL.Path, "/api/status/")
	rec, ok := s.Reg.Get(id)
	if !ok {
		util.WriteJSON(w, 404, types.BuildInfo{Success: false, Error: "not found"})
		return
	}
	util.WriteJSON(w, 200, types.BuildInfo{
		Success: rec.Status == "done",
		BuildID: id,
		APKName: rec.APKName,
		Error:   rec.Err,
		Log:     rec.Log,
	})
}

func (s *Server) handleLogStream(w http.ResponseWriter, r *http.Request) {
	id := strings.TrimPrefix(r.URL.Path, "/api/log/")
	rec, ok := s.Reg.Get(id)
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

	for _, line := range strings.Split(rec.Log, "\n") {
		if line != "" {
			fmt.Fprintf(w, "data: %s\n\n", line)
			flusher.Flush()
		}
	}

	for line := range rec.LogCh {
		fmt.Fprintf(w, "data: %s\n\n", line)
		flusher.Flush()
	}

	if rec.Status == "done" {
		fmt.Fprintf(w, "event: done\ndata: %s\n\n", rec.APKName)
	} else {
		fmt.Fprintf(w, "event: failed\ndata: %s\n\n", rec.Err)
	}
	flusher.Flush()
}

func (s *Server) handleDownload(w http.ResponseWriter, r *http.Request) {
	id := strings.TrimPrefix(r.URL.Path, "/api/download/")
	rec, ok := s.Reg.Get(id)
	if !ok || rec.Status != "done" {
		http.Error(w, "not ready", http.StatusNotFound)
		return
	}
	p := filepath.Join(s.Builder.BaseDir, "output", rec.APKName)
	if _, err := os.Stat(p); err != nil {
		http.Error(w, "file missing", http.StatusNotFound)
		return
	}
	w.Header().Set("Content-Type", "application/vnd.android.package-archive")
	w.Header().Set("Content-Disposition", fmt.Sprintf(`attachment; filename="%s"`, rec.APKName))
	http.ServeFile(w, r, p)
}
