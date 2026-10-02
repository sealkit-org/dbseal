package ui

import (
	"context"
	"embed"
	"encoding/json"
	"fmt"
	"io/fs"
	"net/http"
	"os/exec"
	"runtime"
	"time"
)

//go:embed dist/*
var embeddedDist embed.FS

// ServerOptions specifies the UI server configuration.
type ServerOptions struct {
	Port       int
	NoBrowser  bool
	ConfigPath string
}

// Start launches the local HTTP web server and serves the embedded Web UI.
func Start(ctx context.Context, opts ServerOptions) error {
	if opts.Port <= 0 {
		opts.Port = 8080
	}

	distSubFS, err := fs.Sub(embeddedDist, "dist")
	if err != nil {
		return fmt.Errorf("failed to load embedded UI dist: %w", err)
	}

	mux := http.NewServeMux()

	// 1. Static asset server (embedded SPA)
	fileServer := http.FileServer(http.FS(distSubFS))
	mux.Handle("/", fileServer)

	// 2. Health check endpoint
	mux.HandleFunc("/api/health", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{
			"status":  "ok",
			"version": "v0.1.0",
			"time":    time.Now().Format(time.RFC3339),
		})
	})

	addr := fmt.Sprintf("127.0.0.1:%d", opts.Port)
	serverURL := fmt.Sprintf("http://%s", addr)

	fmt.Printf("\n🦭 dbseal Web UI is running at %s\n", serverURL)
	fmt.Printf("   Press Ctrl+C to stop the server\n\n")

	if !opts.NoBrowser {
		go func() {
			time.Sleep(300 * time.Millisecond)
			_ = OpenBrowser(serverURL)
		}()
	}

	server := &http.Server{
		Addr:    addr,
		Handler: mux,
	}

	// Graceful shutdown on context cancellation
	go func() {
		<-ctx.Done()
		shutdownCtx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
		defer cancel()
		_ = server.Shutdown(shutdownCtx)
	}()

	if err := server.ListenAndServe(); err != nil && err != http.ErrServerClosed {
		return fmt.Errorf("UI server failed: %w", err)
	}

	return nil
}

// OpenBrowser opens the specified URL in the system's default browser.
func OpenBrowser(url string) error {
	var cmd string
	var args []string

	switch runtime.GOOS {
	case "windows":
		cmd = "rundll32"
		args = []string{"url.dll,FileProtocolHandler", url}
	case "darwin":
		cmd = "open"
		args = []string{url}
	default: // linux, freebsd, openbsd
		cmd = "xdg-open"
		args = []string{url}
	}

	return exec.Command(cmd, args...).Start()
}
