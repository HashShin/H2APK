package main

import (
	"bufio"
	"fmt"
	"io"
	"log"
	"net"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"time"

	"h2apk/internal/assets"
	"h2apk/internal/build"
	"h2apk/internal/config"
	"h2apk/internal/server"
)

func main() {
	baseDir, _ := os.Getwd()

	cfg := &config.Resolver{BaseDir: baseDir}
	cfg.CheckDeps()

	os.MkdirAll(filepath.Join(baseDir, "output"), 0755)
	cleanOldBuilds(baseDir)
	go func() {
		for range time.Tick(1 * time.Hour) {
			cleanOldBuilds(baseDir)
		}
	}()

	reg := build.NewRegistry()
	builder := &build.Builder{BaseDir: baseDir, Cfg: cfg, Reg: reg}
	srv := &server.Server{Reg: reg, Builder: builder}

	mux := http.NewServeMux()
	srv.RegisterRoutes(mux)
	mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/" {
			w.Header().Set("Content-Type", "text/html; charset=utf-8")
			io.WriteString(w, assets.IndexHTML)
			return
		}
		http.NotFound(w, r)
	})
	mux.HandleFunc("/instapay.png", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "image/png")
		w.Write(assets.InstapayPNG)
	})
	mux.HandleFunc("/binance.png", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "image/png")
		w.Write(assets.BinancePNG)
	})

	port := env("PORT", "")
	if p := readEnvPort(baseDir); p != "" {
		port = p
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
			saveEnvPort(baseDir, port)
			continue
		}
		if err != nil {
			log.Fatal(err)
		}
		fmt.Printf("  H2APK — HTML/URL to APK\n")
		localURL := "http://localhost:" + port
		fmt.Printf("  %s\n", localURL)
		if ip := localIP(); ip != "" {
			fmt.Printf("  http://%s:%s\n", ip, port)
		}
		fmt.Println()
		saveEnvPort(baseDir, port)
		openBrowser(localURL)
		log.Fatal(http.Serve(listener, mux))
	}
}

func openBrowser(url string) {
	var commands [][]string
	switch runtime.GOOS {
	case "windows":
		commands = [][]string{{"rundll32", "url.dll,FileProtocolHandler", url}}
	case "darwin":
		commands = [][]string{{"open", url}}
	default:
		commands = [][]string{
			{"termux-open-url", url},
			{"xdg-open", url},
		}
	}
	for _, command := range commands {
		if _, err := exec.LookPath(command[0]); err != nil {
			continue
		}
		if err := exec.Command(command[0], command[1:]...).Start(); err == nil {
			return
		}
	}
}

func cleanOldBuilds(baseDir string) {
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

func readEnvPort(baseDir string) string {
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

func saveEnvPort(baseDir, port string) {
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
