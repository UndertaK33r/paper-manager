package api

import (
	"encoding/json"
	"errors"
	"net/http"
	"strconv"
	"strings"
	"time"

	"paper-manager/internal/models"
	"paper-manager/internal/store"
)

type Config struct {
	Store       *store.Store
	UploadDir   string
	WebDir      string
	MaxUploadMB int64
	AuthUser    string
	AuthPass    string
	// Version 由构建时注入（-X main.version），未注入时为 dev
	Version string
}

type Server struct {
	store       *store.Store
	uploadDir   string
	webDir      string
	maxUploadMB int64
	authUser    string
	authPass    string
	version     string
}

func New(cfg Config) *Server {
	max := cfg.MaxUploadMB
	if max <= 0 {
		max = 100
	}
	version := cfg.Version
	if version == "" {
		version = "dev"
	}
	return &Server{
		store:       cfg.Store,
		uploadDir:   cfg.UploadDir,
		webDir:      cfg.WebDir,
		maxUploadMB: max,
		authUser:    cfg.AuthUser,
		authPass:    cfg.AuthPass,
		version:     version,
	}
}

func (s *Server) Handler() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /api/papers", s.handleListPapers)
	mux.HandleFunc("POST /api/papers", s.handleCreatePaper)
	mux.HandleFunc("POST /api/papers/extract-pdf", s.handleExtractPDF)
	mux.HandleFunc("GET /api/papers/{id}", s.handleGetPaper)
	mux.HandleFunc("PUT /api/papers/{id}", s.handleUpdatePaper)
	mux.HandleFunc("PATCH /api/papers/{id}", s.handlePatchPaper)
	mux.HandleFunc("DELETE /api/papers/{id}", s.handleDeletePaper)
	mux.HandleFunc("GET /api/papers/{id}/pdf", s.handleGetPDF)
	mux.HandleFunc("POST /api/papers/{id}/toggle-read", s.handleToggleRead)
	mux.HandleFunc("POST /api/papers/{id}/re-extract", s.handleReExtract)
	mux.HandleFunc("POST /api/papers/{id}/re-detect", s.handleRedetect)
	mux.HandleFunc("POST /api/ai/translate-text", s.handleTranslateText)
	mux.HandleFunc("POST /api/papers/{id}/tags", s.handleAddPaperTag)
	mux.HandleFunc("DELETE /api/papers/{id}/tags/{tagID}", s.handleRemovePaperTag)
	mux.HandleFunc("POST /api/papers/{id}/collections", s.handleAddPaperCollection)
	mux.HandleFunc("DELETE /api/papers/{id}/collections/{collectionID}", s.handleRemovePaperCollection)
	mux.HandleFunc("POST /api/papers/{id}/summarize", s.handleSummarize)
	mux.HandleFunc("POST /api/papers/{id}/ai-extract", s.handleAIExtract)

	mux.HandleFunc("GET /api/categories", s.handleListCategories)
	mux.HandleFunc("POST /api/categories", s.handleCreateCategory)
	mux.HandleFunc("DELETE /api/categories/{id}", s.handleDeleteCategory)

	mux.HandleFunc("GET /api/tags", s.handleListTags)
	mux.HandleFunc("POST /api/tags", s.handleCreateTag)
	mux.HandleFunc("DELETE /api/tags/{id}", s.handleDeleteTag)

	mux.HandleFunc("GET /api/collections", s.handleListCollections)
	mux.HandleFunc("POST /api/collections", s.handleCreateCollection)
	mux.HandleFunc("DELETE /api/collections/{id}", s.handleDeleteCollection)

	mux.HandleFunc("GET /api/stats", s.handleStats)
	mux.HandleFunc("GET /api/health", s.handleHealth)
	mux.HandleFunc("GET /api/papers/export.bib", s.handleExportBib)
	mux.HandleFunc("GET /api/papers/export/notes", s.handleExportNotes)
	mux.HandleFunc("GET /api/backup", s.handleBackup)
	// 阅读模式与标注
	mux.HandleFunc("GET /api/papers/{id}/text", s.handlePaperText)
	mux.HandleFunc("GET /api/papers/{id}/annotations", s.handleListAnnotations)
	mux.HandleFunc("POST /api/papers/{id}/annotations", s.handleCreateAnnotation)
	mux.HandleFunc("PATCH /api/annotations/{id}", s.handleUpdateAnnotation)
	mux.HandleFunc("DELETE /api/annotations/{id}", s.handleDeleteAnnotation)
	// 回收站：删除是软删除，彻底删除需要显式 purge
	mux.HandleFunc("GET /api/trash", s.handleListTrash)
	mux.HandleFunc("DELETE /api/trash", s.handleEmptyTrash)
	mux.HandleFunc("POST /api/papers/{id}/restore", s.handleRestorePaper)
	mux.HandleFunc("DELETE /api/papers/{id}/purge", s.handlePurgePaper)
	mux.HandleFunc("POST /api/ask", s.handleAsk)
	mux.HandleFunc("GET /api/graph", s.handleGraph)
	mux.HandleFunc("GET /api/settings", s.handleGetSettings)
	mux.HandleFunc("PUT /api/settings", s.handleUpdateSettings)
	mux.HandleFunc("GET /api/ai/models", s.handleAIModels)
	mux.HandleFunc("POST /api/ai/models", s.handleAIModels)
	mux.HandleFunc("POST /api/ai/test", s.handleAITest)

	mux.Handle("GET /", s.staticHandler())

	var h http.Handler = mux
	if s.authUser != "" || s.authPass != "" {
		h = s.basicAuth(h)
	}
	return noCache(h)
}

func noCache(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/api" || strings.HasPrefix(r.URL.Path, "/api/") {
			w.Header().Set("Cache-Control", "no-store")
		} else {
			w.Header().Set("Cache-Control", "no-cache")
		}
		next.ServeHTTP(w, r)
	})
}

func (s *Server) basicAuth(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// 静态壳（HTML/CSS/JS）不校验：未登录时也要能加载出应用内的登录页。
		// 这些资源随开源仓库公开、不含数据；数据只经 /api/ 暴露。
		if !strings.HasPrefix(r.URL.Path, "/api/") {
			next.ServeHTTP(w, r)
			return
		}
		if !s.authorized(r) {
			w.Header().Set("WWW-Authenticate", `Basic realm="paper-manager"`)
			writeError(w, http.StatusUnauthorized, "unauthorized")
			return
		}
		next.ServeHTTP(w, r)
	})
}

// authorized 接受三种凭据：
//   - HTTP Basic：浏览器原生弹窗（也覆盖直接访问 URL 的场景）
//   - Bearer <密码>：前端登录页与 api() 调用（前端历史上只发 Bearer，
//     而后端只认 Basic，导致开启密码后网页登录不可用）
//   - ?token=<密码>：下载/iframe 等无法自定义请求头的场景
func (s *Server) authorized(r *http.Request) bool {
	if user, pass, ok := r.BasicAuth(); ok && user == s.authUser && pass == s.authPass {
		return true
	}
	if h := r.Header.Get("Authorization"); strings.HasPrefix(h, "Bearer ") {
		if token := strings.TrimSpace(strings.TrimPrefix(h, "Bearer ")); token != "" && token == s.authPass {
			return true
		}
	}
	if token := r.URL.Query().Get("token"); token != "" && token == s.authPass {
		return true
	}
	return false
}

func (s *Server) idParam(r *http.Request, name string) (int64, bool) {
	v := r.PathValue(name)
	id, err := strconv.ParseInt(v, 10, 64)
	if err != nil || id <= 0 {
		return 0, false
	}
	return id, true
}

func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(v)
}

func writeError(w http.ResponseWriter, status int, msg string) {
	writeJSON(w, status, map[string]string{"error": msg})
}

func readJSON(r *http.Request, v any) error {
	if r.Body == nil {
		return errors.New("empty body")
	}
	return json.NewDecoder(r.Body).Decode(v)
}

func parseBoolParam(v string) *bool {
	switch strings.ToLower(v) {
	case "1", "true", "yes":
		b := true
		return &b
	case "0", "false", "no":
		b := false
		return &b
	}
	return nil
}

func parseIntParam(v string) *int {
	if v == "" {
		return nil
	}
	n, err := strconv.Atoi(v)
	if err != nil {
		return nil
	}
	return &n
}

func csvSplit(s string) []string {
	out := []string{}
	for _, p := range strings.Split(s, ",") {
		p = strings.TrimSpace(p)
		if p != "" {
			out = append(out, p)
		}
	}
	return out
}

func (s *Server) buildQuery(r *http.Request) models.PaperQuery {
	q := r.URL.Query()
	query := models.PaperQuery{
		Search:        strings.TrimSpace(q.Get("search")),
		TagIDs:        parseIDList(q.Get("tags")),
		CollectionIDs: parseIDList(q.Get("collections")),
		Sort:          q.Get("sort"),
		Order:         q.Get("order"),
		Read:          parseBoolParam(q.Get("read")),
		Starred:       parseBoolParam(q.Get("starred")),
		Status:        q.Get("status"),
	}
	if v := parseIntPtr(q.Get("category")); v != nil {
		id := *v
		query.CategoryID = &id
	}
	query.YearFrom = parseIntParam(q.Get("yearFrom"))
	query.YearTo = parseIntParam(q.Get("yearTo"))
	page, _ := strconv.Atoi(q.Get("page"))
	size, _ := strconv.Atoi(q.Get("pageSize"))
	if page == 0 {
		page = 1
	}
	if size == 0 {
		size = 20
	}
	query.Page = page
	query.PageSize = size
	return query
}

func (s *Server) paperFromInput(in models.PaperInput, existing *models.Paper) models.Paper {
	p := models.Paper{}
	if existing != nil {
		p = *existing
	}
	p.Title = strings.TrimSpace(in.Title)
	p.Authors = strings.TrimSpace(in.Authors)
	p.Year = in.Year
	p.Venue = strings.TrimSpace(in.Venue)
	p.DOI = strings.TrimSpace(in.DOI)
	p.Keywords = strings.TrimSpace(in.Keywords)
	p.Link = strings.TrimSpace(in.Link)
	p.Summary = strings.TrimSpace(in.Summary)
	p.Notes = in.Notes
	p.CategoryID = in.CategoryID
	p.Read = in.Read
	if in.Status != "" {
		p.Status = in.Status
	}
	p.Starred = in.Starred
	return p
}

func parseIntPtr(s string) *int64 {
	if s == "" {
		return nil
	}
	v, err := strconv.ParseInt(s, 10, 64)
	if err != nil {
		return nil
	}
	return &v
}

func parseIDList(s string) []int64 {
	if s == "" {
		return nil
	}
	out := []int64{}
	for _, p := range strings.Split(s, ",") {
		p = strings.TrimSpace(p)
		if p == "" {
			continue
		}
		v, err := strconv.ParseInt(p, 10, 64)
		if err == nil {
			out = append(out, v)
		}
	}
	return out
}

var _ = time.Now
