package api

import (
	"errors"
	"net/http"
	"strings"

	"paper-manager/internal/models"
	pdfmeta "paper-manager/internal/pdf"
	"paper-manager/internal/store"
)

const maxAnnotationQuote = 4000 // 单条标注引文上限（防超大 payload）

// handlePaperText 返回阅读模式需要的结构化全文（服务端已做断行重排与标题识别）。
// fulltext 很大（可达 200K 字符），因此独立于详情接口按需拉取。
func (s *Server) handlePaperText(w http.ResponseWriter, r *http.Request) {
	id, ok := s.idParam(r, "id")
	if !ok {
		writeError(w, http.StatusBadRequest, "invalid paper id")
		return
	}
	if _, err := s.store.GetPaper(id); err != nil {
		writeError(w, http.StatusNotFound, "paper not found")
		return
	}
	text := s.store.PaperFullText(id)
	pages := pdfmeta.SplitReading(text)
	writeJSON(w, http.StatusOK, map[string]any{
		"paperId":  id,
		"chars":    len([]rune(text)),
		"pages":    pages,
		"hasText":  strings.TrimSpace(text) != "",
		"rawChars": len(text),
	})
}

// handleListAnnotations 列出某篇论文的标注。
func (s *Server) handleListAnnotations(w http.ResponseWriter, r *http.Request) {
	id, ok := s.idParam(r, "id")
	if !ok {
		writeError(w, http.StatusBadRequest, "invalid paper id")
		return
	}
	list, err := s.store.ListAnnotations(id)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"annotations": list, "total": len(list)})
}

// handleCreateAnnotation 新建标注。
func (s *Server) handleCreateAnnotation(w http.ResponseWriter, r *http.Request) {
	id, ok := s.idParam(r, "id")
	if !ok {
		writeError(w, http.StatusBadRequest, "invalid paper id")
		return
	}
	if _, err := s.store.GetPaper(id); err != nil {
		writeError(w, http.StatusNotFound, "paper not found")
		return
	}
	var body struct {
		Start int    `json:"start"`
		End   int    `json:"end"`
		Quote string `json:"quote"`
		Color string `json:"color"`
		Note  string `json:"note"`
	}
	if err := readJSON(r, &body); err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	if body.Start < 0 || body.End <= body.Start {
		writeError(w, http.StatusBadRequest, "标注范围无效")
		return
	}
	if body.End-body.Start > maxAnnotationQuote {
		writeError(w, http.StatusBadRequest, "选中的内容过长，请分段标注")
		return
	}
	a := models.Annotation{
		PaperID: id,
		Start:   body.Start,
		End:     body.End,
		Quote:   body.Quote,
		Color:   body.Color,
		Note:    strings.TrimSpace(body.Note),
	}
	if _, err := s.store.CreateAnnotation(&a); err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusCreated, a)
}

// handleUpdateAnnotation 修改标注颜色或备注。
func (s *Server) handleUpdateAnnotation(w http.ResponseWriter, r *http.Request) {
	id, ok := s.idParam(r, "id")
	if !ok {
		writeError(w, http.StatusBadRequest, "invalid annotation id")
		return
	}
	var body struct {
		Color *string `json:"color"`
		Note  *string `json:"note"`
	}
	if err := readJSON(r, &body); err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	if body.Note != nil {
		trimmed := strings.TrimSpace(*body.Note)
		body.Note = &trimmed
	}
	if err := s.store.UpdateAnnotation(id, body.Color, body.Note); err != nil {
		if errors.Is(err, store.ErrConflict) {
			writeError(w, http.StatusNotFound, "标注不存在")
			return
		}
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, map[string]bool{"ok": true})
}

// handleDeleteAnnotation 删除标注。
func (s *Server) handleDeleteAnnotation(w http.ResponseWriter, r *http.Request) {
	id, ok := s.idParam(r, "id")
	if !ok {
		writeError(w, http.StatusBadRequest, "invalid annotation id")
		return
	}
	if err := s.store.DeleteAnnotation(id); err != nil {
		if errors.Is(err, store.ErrConflict) {
			writeError(w, http.StatusNotFound, "标注不存在")
			return
		}
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, map[string]bool{"ok": true})
}
