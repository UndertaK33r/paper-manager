package api

import (
	"bytes"
	"crypto/sha256"
	"fmt"
	"io/fs"
	"net/http"
	"os"
	"path"
	"strings"

	webassets "paper-manager/web"
)

// Serve only public frontend resources, not Go sources or directory listings.
func publicStaticPath(name string) bool {
	if name == "index.html" || name == "app.js" || name == "style.css" {
		return true
	}
	if !fs.ValidPath(name) || !strings.HasPrefix(name, "assets/") {
		return false
	}
	for _, part := range strings.Split(name, "/") {
		if strings.HasPrefix(part, ".") {
			return false
		}
	}
	switch path.Ext(name) {
	case ".js", ".css", ".json", ".png", ".jpg", ".jpeg", ".gif", ".svg", ".ico", ".webp", ".woff", ".woff2", ".ttf", ".wasm":
		return true
	}
	return false
}

func (s *Server) staticHandler() http.Handler {
	var files fs.FS = webassets.Files
	if s.webDir != "" {
		files = os.DirFS(s.webDir)
	}
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		name := strings.TrimPrefix(r.URL.Path, "/")
		if name == "" {
			name = "index.html"
		}
		if !publicStaticPath(name) {
			http.NotFound(w, r)
			return
		}
		info, err := fs.Stat(files, name)
		if err != nil || !info.Mode().IsRegular() {
			http.NotFound(w, r)
			return
		}
		content, err := fs.ReadFile(files, name)
		if err != nil {
			http.NotFound(w, r)
			return
		}
		// Mutable UI files revalidate; only vendored libraries may stay cached.
		if name == "assets/vue.global.prod.js" || strings.HasPrefix(name, "assets/pdfjs/") {
			w.Header().Set("Cache-Control", "public, max-age=86400")
		} else {
			w.Header().Set("Cache-Control", "no-cache")
		}
		sum := sha256.Sum256(content)
		w.Header().Set("ETag", fmt.Sprintf(`"%x"`, sum))
		http.ServeContent(w, r, name, info.ModTime(), bytes.NewReader(content))
	})
}
