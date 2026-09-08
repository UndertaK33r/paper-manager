package main

import (
	"log"
	"net"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strconv"
	"time"

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
	ln, err := net.Listen("tcp", addr)
	if err != nil {
		log.Fatalf("端口 %s 被占用: %v（可用 PORT=其他端口 重新启动，或 NO_OPEN=1 关闭自动开浏览器）", port, err)
	}

	url := "http://localhost:" + port
	log.Printf("论文管理已启动: %s", url)
	log.Printf("数据目录: %s（备份直接拷贝这个文件夹）", dataDir)
	log.Printf("退出：按 Ctrl+C，或直接关闭本窗口")
	if os.Getenv("NO_OPEN") == "" {
		go func() {
			time.Sleep(400 * time.Millisecond)
			openBrowser(url)
		}()
	}
	if err := http.Serve(ln, srv.Handler()); err != nil {
		log.Fatal(err)
	}
}

// openBrowser 用系统默认浏览器打开页面（NO_OPEN=1 禁用）。
func openBrowser(url string) {
	var cmd *exec.Cmd
	switch runtime.GOOS {
	case "darwin":
		cmd = exec.Command("open", url)
	case "windows":
		cmd = exec.Command("cmd", "/c", "start", "", url)
	default:
		cmd = exec.Command("xdg-open", url)
	}
	_ = cmd.Start()
}

func env(key, def string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return def
}
