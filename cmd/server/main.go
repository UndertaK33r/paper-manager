package main

import (
	"log"
	"net/http"
	"os"
	"path/filepath"
	"strconv"

	"paper-manager/internal/api"
	"paper-manager/internal/store"
)

func main() {
	port := env("PORT", "8080")
	dataDir := env("DATA_DIR", "./data")
	uploadDir := env("UPLOAD_DIR", filepath.Join(dataDir, "uploads"))
	webDir := env("WEB_DIR", "web")
	maxMB := int64(100)
	if v, err := strconv.ParseInt(env("MAX_UPLOAD_MB", "100"), 10, 64); err == nil && v > 0 {
		maxMB = v
	}

	if err := os.MkdirAll(dataDir, 0o755); err != nil {
		log.Fatalf("create data dir: %v", err)
	}
	if err := os.MkdirAll(uploadDir, 0o755); err != nil {
		log.Fatalf("create upload dir: %v", err)
	}

	st, err := store.Open(filepath.Join(dataDir, "paper-manager.db"))
	if err != nil {
		log.Fatalf("open store: %v", err)
	}
	defer st.Close()

	srv := api.New(api.Config{
		Store:       st,
		UploadDir:   uploadDir,
		WebDir:      webDir,
		MaxUploadMB: maxMB,
		AuthUser:    env("AUTH_USERNAME", ""),
		AuthPass:    env("AUTH_PASSWORD", ""),
	})

	addr := ":" + port
	log.Printf("paper-manager listening on %s", addr)
	if err := http.ListenAndServe(addr, srv.Handler()); err != nil {
		log.Fatal(err)
	}
}

func env(key, def string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return def
}
