package web

import (
	"embed"
	"fmt"
	"io/fs"
	"mime"
	"net/http"
	"path"
	"strings"
	"time"
)

// Force embed all prefixed folders (i.e. _app)
//
//go:embed all:dist
var dist embed.FS

const (
	// vite.config fallback / entry
	indexHTML = "index.html"

	immutableDir    = "_app/immutable/"
	immutableMaxAge = 365 * 24 * time.Hour
)

func init() {
	if err := mime.AddExtensionType(".webmanifest", "application/manifest+json"); err != nil {
		panic(fmt.Sprintf("web: register .webmanifest: %v", err))
	}
}

func Handler() (http.Handler, error) {
	root, err := fs.Sub(dist, "dist/app")
	if err != nil {
		return nil, fmt.Errorf("web: %w", err)
	}

	page, err := fs.ReadFile(root, indexHTML)
	if err != nil {
		return nil, fmt.Errorf("web: no %s in the embedded build; run `pnpm build` in webui/: %w", indexHTML, err)
	}

	files := http.FileServer(http.FS(root))

	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		name := strings.TrimPrefix(path.Clean(r.URL.Path), "/")

		if name != "" && isFileExists(root, name) {
			if strings.HasPrefix(name, immutableDir) {
				w.Header().Set("Cache-Control",
					fmt.Sprintf("public, max-age=%d, immutable", int(immutableMaxAge.Seconds())))
			}
			files.ServeHTTP(w, r)
			return
		}

		if path.Ext(name) != "" {
			http.NotFound(w, r)
			return
		}

		servePage(w, page)
	}), nil
}

// Serves uncached page
func servePage(w http.ResponseWriter, page []byte) {
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.Header().Set("Cache-Control", "no-cache")
	w.WriteHeader(http.StatusOK)
	w.Write(page)
}

func isFileExists(root fs.FS, name string) bool {
	info, err := fs.Stat(root, name)
	return err == nil && !info.IsDir()
}
