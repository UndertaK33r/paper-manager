package main

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	webassets "paper-manager/web"
)

// build.sh marks release builds; these never pick a library based on shell CWD.
var releaseBuild string

type runtimePaths struct{ dataDir, uploadDir, webDir string }

func resolveRuntimePaths(cwd, executable string, lookup func(string) (string, bool), packaged bool) (runtimePaths, error) {
	p := runtimePaths{dataDir: filepath.Join(filepath.Dir(executable), "data")}
	if !packaged && sourceWorkspace(cwd) {
		p.dataDir = filepath.Join(cwd, "data")
		local := filepath.Join(cwd, "web")
		if webassets.Validate(os.DirFS(local)) == nil {
			p.webDir = local
		}
	}
	absolute := func(v string) string {
		if filepath.IsAbs(v) {
			return filepath.Clean(v)
		}
		return filepath.Join(cwd, v)
	}
	if v, ok := lookup("DATA_DIR"); ok && v != "" {
		p.dataDir = absolute(v)
	}
	p.uploadDir = filepath.Join(p.dataDir, "uploads")
	if v, ok := lookup("UPLOAD_DIR"); ok && v != "" {
		p.uploadDir = absolute(v)
	}
	if v, ok := lookup("WEB_DIR"); ok {
		if v == "" {
			return runtimePaths{}, fmt.Errorf("WEB_DIR 不能为空；不设置该变量即可使用内嵌页面")
		}
		p.webDir = absolute(v)
		if err := webassets.Validate(os.DirFS(p.webDir)); err != nil {
			return runtimePaths{}, fmt.Errorf("WEB_DIR 不可用: %w", err)
		}
	}
	return p, nil
}

func sourceWorkspace(dir string) bool {
	b, err := os.ReadFile(filepath.Join(dir, "go.mod"))
	if err != nil {
		return false
	}
	found := false
	for _, line := range strings.Split(string(b), "\n") {
		f := strings.Fields(line)
		if len(f) == 2 && f[0] == "module" && f[1] == "paper-manager" {
			found = true
			break
		}
	}
	info, err := os.Stat(filepath.Join(dir, "cmd", "server", "main.go"))
	return found && err == nil && info.Mode().IsRegular()
}
