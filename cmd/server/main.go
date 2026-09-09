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

// version 由构建脚本通过 -ldflags "-X main.version=vX.Y.Z" 注入。
var version = "dev"

func main() {
	port := env("PORT", "8080")
	cwd, err := os.Getwd()
	if err != nil {
		log.Fatal(err)
	}
	executable, err := os.Executable()
	if err != nil {
		log.Fatal(err)
	}
	if real, e := filepath.EvalSymlinks(executable); e == nil {
		executable = real
	}
	paths, err := resolveRuntimePaths(cwd, executable, os.LookupEnv, releaseBuild == "true")
	if err != nil {
		log.Fatal(err)
	}
	dataDir, uploadDir, webDir := paths.dataDir, paths.uploadDir, paths.webDir
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
		Version:     version,
	})

	addr := net.JoinHostPort(env("HOST", ""), port)
	ln, err := net.Listen("tcp", addr)
	if err != nil {
		log.Fatalf("端口 %s 被占用: %v（可用 PORT=其他端口 重新启动，或 NO_OPEN=1 关闭自动开浏览器）", port, err)
	}

	url := "http://localhost:" + port
	log.Printf("论文管理已启动: %s", url)
	log.Printf("数据目录: %s（备份前先退出程序，再完整复制数据目录和上传目录）", dataDir)
	if env("HOST", "") == "" && env("AUTH_PASSWORD", "") == "" {
		log.Printf("提示：当前允许局域网免密访问。仅本机使用可设置 HOST=127.0.0.1；共享前请配置 AUTH_USERNAME/AUTH_PASSWORD。")
	}
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
