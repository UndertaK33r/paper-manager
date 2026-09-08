package api

import (
	"fmt"
	"net/http"
	"path/filepath"
	"strings"
	"time"

	pdfmeta "paper-manager/internal/pdf"
)

// handlePatchPaper 部分更新（未提供的字段不覆盖），对齐老项目 PATCH 语义。
func (s *Server) handlePatchPaper(w http.ResponseWriter, r *http.Request) {
	id, ok := s.idParam(r, "id")
	if !ok {
		writeError(w, http.StatusBadRequest, "invalid paper id")
		return
	}
	p, err := s.store.GetPaper(id)
	if err != nil {
		writeError(w, http.StatusNotFound, "paper not found")
		return
	}
	var body struct {
		Title       *string  `json:"title"`
		Authors     *string  `json:"authors"`
		Year        *int     `json:"year"`
		Venue       *string  `json:"venue"`
		DOI         *string  `json:"doi"`
		Keywords    *string  `json:"keywords"`
		Link        *string  `json:"link"`
		Summary     *string  `json:"summary"`
		Notes       *string  `json:"notes"`
		Status      *string  `json:"status"`
		Read        *bool    `json:"read"`
		Starred     *bool    `json:"starred"`
		CategoryID  *int64   `json:"categoryId"`
		Tags        *[]int64 `json:"tags"`
		Collections *[]int64 `json:"collections"`
	}
	if err := readJSON(r, &body); err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	if body.Title != nil {
		p.Title = strings.TrimSpace(*body.Title)
	}
	if body.Authors != nil {
		p.Authors = strings.TrimSpace(*body.Authors)
	}
	if body.Year != nil {
		p.Year = *body.Year
	}
	if body.Venue != nil {
		p.Venue = strings.TrimSpace(*body.Venue)
	}
	if body.DOI != nil {
		p.DOI = strings.TrimSpace(*body.DOI)
	}
	if body.Keywords != nil {
		p.Keywords = strings.TrimSpace(*body.Keywords)
	}
	if body.Link != nil {
		p.Link = strings.TrimSpace(*body.Link)
	}
	if body.Summary != nil {
		p.Summary = strings.TrimSpace(*body.Summary)
	}
	if body.Notes != nil {
		p.Notes = *body.Notes
	}
	if body.Status != nil {
		st := strings.TrimSpace(*body.Status)
		if st == "unread" || st == "reading" || st == "read" {
			p.Status = st
			p.Read = st == "read"
		}
	}
	if body.Read != nil {
		p.Read = *body.Read
		if p.Read {
			p.Status = "read"
		} else if p.Status == "read" {
			p.Status = "unread"
		}
	}
	if body.Starred != nil {
		p.Starred = *body.Starred
	}
	if body.CategoryID != nil {
		p.CategoryID = body.CategoryID
	}
	if p.FullText == "" {
		p.FullText = s.store.PaperFullText(id)
	}
	if err := s.store.UpdatePaper(&p); err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	if body.Tags != nil {
		if err := s.store.SetPaperTags(id, *body.Tags); err != nil {
			writeError(w, http.StatusInternalServerError, err.Error())
			return
		}
	}
	if body.Collections != nil {
		if err := s.store.SetPaperCollections(id, *body.Collections); err != nil {
			writeError(w, http.StatusInternalServerError, err.Error())
			return
		}
	}
	paper, err := s.store.GetPaper(id)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, paper)
}

// handleSetStatus 设置阅读状态（unread / reading / read）。
func (s *Server) handleSetStatus(w http.ResponseWriter, r *http.Request) {
	id, ok := s.idParam(r, "id")
	if !ok {
		writeError(w, http.StatusBadRequest, "invalid paper id")
		return
	}
	var body struct {
		Status string `json:"status"`
	}
	if err := readJSON(r, &body); err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	if err := s.store.SetPaperStatus(id, body.Status); err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	paper, _ := s.store.GetPaper(id)
	writeJSON(w, http.StatusOK, paper)
}

// handleReExtract 重新提取 PDF 全文（修复提取器后刷新旧论文）。
func (s *Server) handleReExtract(w http.ResponseWriter, r *http.Request) {
	id, ok := s.idParam(r, "id")
	if !ok {
		writeError(w, http.StatusBadRequest, "invalid paper id")
		return
	}
	p, err := s.store.GetPaper(id)
	if err != nil {
		writeError(w, http.StatusNotFound, "paper not found")
		return
	}
	if !p.HasPDF || p.PDFPath == "" {
		writeError(w, http.StatusBadRequest, "paper has no PDF")
		return
	}
	text, err := pdfmeta.ExtractText(filepath.Join(s.uploadDir, p.PDFPath), 200000)
	if err != nil {
		writeError(w, http.StatusBadRequest, "无法提取全文: "+err.Error())
		return
	}
	p.FullText = text
	if err := s.store.UpdatePaper(&p); err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	paper, _ := s.store.GetPaper(id)
	writeJSON(w, http.StatusOK, paper)
}

// handleHealth 健康检查（登录页用它验证密码）。
func (s *Server) handleHealth(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, map[string]string{"status": "ok", "version": "1.0"})
}

// handleRedetect 用健壮提取器 + 在线补全重新识别元数据（可修正历史错误记录）。
func (s *Server) handleRedetect(w http.ResponseWriter, r *http.Request) {
	id, ok := s.idParam(r, "id")
	if !ok {
		writeError(w, http.StatusBadRequest, "invalid paper id")
		return
	}
	p, err := s.store.GetPaper(id)
	if err != nil {
		writeError(w, http.StatusNotFound, "paper not found")
		return
	}
	if !p.HasPDF || p.PDFPath == "" {
		writeError(w, http.StatusBadRequest, "paper has no PDF")
		return
	}
	meta, merr := pdfmeta.ExtractWithFallback(filepath.Join(s.uploadDir, p.PDFPath), "")
	if merr == nil {
		if strings.TrimSpace(meta.Title) != "" {
			p.Title = strings.TrimSpace(meta.Title)
		}
		if strings.TrimSpace(meta.Author) != "" && strings.TrimSpace(p.Authors) == "" {
			p.Authors = strings.TrimSpace(meta.Author)
		}
		if strings.TrimSpace(meta.Keywords) != "" && strings.TrimSpace(p.Keywords) == "" {
			p.Keywords = strings.TrimSpace(meta.Keywords)
		}
	}
	if text, terr := pdfmeta.ExtractText(filepath.Join(s.uploadDir, p.PDFPath), 200000); terr == nil {
		p.FullText = text
	}
	in := paperToInput(&p)
	_ = s.maybeEnrich(&in, p.PDFPath)
	p = s.paperFromInput(in, &p)
	if err := s.store.UpdatePaper(&p); err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	paper, _ := s.store.GetPaper(id)
	writeJSON(w, http.StatusOK, paper)
}

// handleExportNotes 导出全部论文笔记为 Markdown 文件（附件下载）。
func (s *Server) handleExportNotes(w http.ResponseWriter, r *http.Request) {
	rows, err := s.store.AllNotes()
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	fmtTime := func(s string) string {
		if t, err := time.Parse(time.RFC3339, s); err == nil {
			return t.Local().Format("2006-01-02 15:04")
		}
		return s
	}
	var b strings.Builder
	b.WriteString("# 论文笔记\n\n")
	b.WriteString(fmt.Sprintf("> 导出时间：%s · 共 %d 篇\n\n", time.Now().Local().Format("2006-01-02 15:04"), len(rows)))
	for _, row := range rows {
		b.WriteString("## " + strings.TrimSpace(row.Title) + "\n\n")
		if strings.TrimSpace(row.Authors) != "" {
			b.WriteString(strings.TrimSpace(row.Authors) + "  \n")
		}
		b.WriteString("> 最后修改：" + fmtTime(row.Updated) + "\n\n")
		b.WriteString(strings.TrimSpace(row.Notes) + "\n\n---\n\n")
	}
	w.Header().Set("Content-Type", "text/markdown; charset=utf-8")
	w.Header().Set("Content-Disposition", fmt.Sprintf("attachment; filename=\"notes-%s.md\"", time.Now().Local().Format("20060102-1504")))
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write([]byte(b.String()))
}
