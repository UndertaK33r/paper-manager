package api

import (
	"archive/zip"
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"paper-manager/internal/models"
	"paper-manager/internal/store"
)

func newAuthedServer(t *testing.T) *Server {
	t.Helper()
	st, err := store.Open(filepath.Join(t.TempDir(), "auth.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { st.Close() })
	return New(Config{Store: st, UploadDir: t.TempDir(), AuthUser: "u", AuthPass: "secret"})
}

func TestAuthAcceptsBasicBearerAndToken(t *testing.T) {
	s := newAuthedServer(t)
	h := s.Handler()

	cases := []struct {
		name   string
		req    *http.Request
		status int
	}{
		{"no credentials", httptest.NewRequest("GET", "/api/health", nil), http.StatusUnauthorized},
		{"wrong basic", basicReq("u", "nope"), http.StatusUnauthorized},
		{"wrong bearer", bearerReq("nope"), http.StatusUnauthorized},
		{"wrong token", httptest.NewRequest("GET", "/api/health?token=nope", nil), http.StatusUnauthorized},
		{"basic ok", basicReq("u", "secret"), http.StatusOK},
		{"bearer ok", bearerReq("secret"), http.StatusOK},
		{"token ok", httptest.NewRequest("GET", "/api/health?token=secret", nil), http.StatusOK},
		// 静态壳必须免鉴权，否则浏览器的 Basic 弹窗会挡住应用内登录页
		{"static shell without credentials", httptest.NewRequest("GET", "/", nil), http.StatusOK},
		{"static asset without credentials", httptest.NewRequest("GET", "/app.js", nil), http.StatusOK},
	}
	for _, tc := range cases {
		rec := httptest.NewRecorder()
		h.ServeHTTP(rec, tc.req)
		if rec.Code != tc.status {
			t.Errorf("%s: status %d, want %d", tc.name, rec.Code, tc.status)
		}
	}
}

func basicReq(user, pass string) *http.Request {
	r := httptest.NewRequest("GET", "/api/health", nil)
	r.SetBasicAuth(user, pass)
	return r
}

func bearerReq(token string) *http.Request {
	r := httptest.NewRequest("GET", "/api/health", nil)
	r.Header.Set("Authorization", "Bearer "+token)
	return r
}

// 备份包应包含数据库快照、上传的 PDF 与恢复说明，且解出的数据库可正常打开。
func TestBackupArchiveIsRestorable(t *testing.T) {
	st, err := store.Open(filepath.Join(t.TempDir(), "backup.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { st.Close() })

	uploadDir := t.TempDir()
	s := New(Config{Store: st, UploadDir: uploadDir})

	// 一篇带 PDF 的论文
	pdfName := "abcd1234.pdf"
	if err := writeFile(filepath.Join(uploadDir, pdfName), []byte("%PDF-1.4 test")); err != nil {
		t.Fatal(err)
	}
	p := models.Paper{Title: "备份测试论文", Authors: "A", Notes: "重要笔记", PDFPath: pdfName, PDFSize: 13}
	id, err := st.CreatePaper(&p)
	if err != nil {
		t.Fatal(err)
	}

	rec := httptest.NewRecorder()
	s.handleBackup(rec, httptest.NewRequest("GET", "/api/backup", nil))
	if rec.Code != http.StatusOK {
		t.Fatalf("status %d: %s", rec.Code, rec.Body.String())
	}
	if ct := rec.Header().Get("Content-Type"); ct != "application/zip" {
		t.Fatalf("content-type = %q", ct)
	}
	if cd := rec.Header().Get("Content-Disposition"); !strings.Contains(cd, "paper-manager-backup-") {
		t.Fatalf("content-disposition = %q", cd)
	}

	zr, err := zip.NewReader(bytes.NewReader(rec.Body.Bytes()), int64(rec.Body.Len()))
	if err != nil {
		t.Fatal(err)
	}
	names := map[string]bool{}
	for _, f := range zr.File {
		names[f.Name] = true
	}
	for _, want := range []string{"data/paper-manager.db", "data/uploads/" + pdfName, "恢复说明.txt"} {
		if !names[want] {
			t.Errorf("备份包缺少 %s（实际包含: %v）", want, names)
		}
	}

	// 把快照解出来，用 store 打开并核对数据确实在里面
	restoreDir := t.TempDir()
	for _, f := range zr.File {
		if f.Name != "data/paper-manager.db" {
			continue
		}
		rc, err := f.Open()
		if err != nil {
			t.Fatal(err)
		}
		var buf bytes.Buffer
		if _, err := buf.ReadFrom(rc); err != nil {
			t.Fatal(err)
		}
		rc.Close()
		if err := writeFile(filepath.Join(restoreDir, "restored.db"), buf.Bytes()); err != nil {
			t.Fatal(err)
		}
	}
	restored, err := store.Open(filepath.Join(restoreDir, "restored.db"))
	if err != nil {
		t.Fatalf("恢复的数据库打不开: %v", err)
	}
	defer restored.Close()
	got, err := restored.GetPaper(id)
	if err != nil {
		t.Fatalf("恢复库中查不到论文: %v", err)
	}
	if got.Title != "备份测试论文" || got.Notes != "重要笔记" {
		t.Fatalf("恢复数据不一致: %+v", got)
	}
}

// 未配置密码时，任何请求都应放行（本地默认用法）。
func TestNoAuthConfiguredAllowsRequests(t *testing.T) {
	s, _ := newModelsTestServer(t)
	rec := httptest.NewRecorder()
	s.Handler().ServeHTTP(rec, httptest.NewRequest("GET", "/api/health", nil))
	if rec.Code != http.StatusOK {
		t.Fatalf("status %d, want 200", rec.Code)
	}
	var body map[string]string
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
		t.Fatal(err)
	}
	if body["version"] == "" {
		t.Fatal("health 未返回版本")
	}
}

func writeFile(path string, data []byte) error {
	return os.WriteFile(path, data, 0o644)
}
