package api

import (
	"errors"
	"net/http"
	"strings"

	"paper-manager/internal/models"
	"paper-manager/internal/store"
)

const (
	maxAnnotationQuote = 4000 // 单条标注引文上限
	maxAnnotationNote  = 8000 // 单条批注文字上限
	maxAnnotationRects = 400  // 单条高亮覆盖的矩形上限（大段选择会被拆成很多行矩形）
)

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
// 位置一律用归一化坐标（0..1 相对页面），不改动原 PDF 文件：
//   - highlight：{page, rects:[{p,x,y,w,h}...], quote}
//   - note：{page, x, y, note}
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
		Kind  string            `json:"kind"`
		Page  int               `json:"page"`
		Rects []models.AnnoRect `json:"rects"`
		X     float64           `json:"x"`
		Y     float64           `json:"y"`
		Quote string            `json:"quote"`
		Color string            `json:"color"`
		Note  string            `json:"note"`
	}
	if err := readJSON(r, &body); err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	if body.Page < 1 {
		writeError(w, http.StatusBadRequest, "页码无效")
		return
	}
	a := models.Annotation{
		PaperID: id,
		Kind:    body.Kind,
		Page:    body.Page,
		Color:   body.Color,
		Note:    strings.TrimSpace(body.Note),
	}
	switch body.Kind {
	case models.AnnoKindNote:
		if body.X < 0 || body.Y < 0 || body.X > 1 || body.Y > 1 {
			writeError(w, http.StatusBadRequest, "批注位置无效")
			return
		}
		a.X, a.Y = body.X, body.Y
		if len(a.Note) > maxAnnotationNote {
			a.Note = a.Note[:maxAnnotationNote]
		}
	default: // highlight
		if len(body.Rects) == 0 {
			writeError(w, http.StatusBadRequest, "没有选中内容")
			return
		}
		if len(body.Rects) > maxAnnotationRects {
			writeError(w, http.StatusBadRequest, "选中的范围过大，请分段标注")
			return
		}
		for _, rc := range body.Rects {
			if rc.Page < 1 || rc.W <= 0 || rc.H <= 0 ||
				rc.X < 0 || rc.Y < 0 || rc.X+rc.W > 1.001 || rc.Y+rc.H > 1.001 {
				writeError(w, http.StatusBadRequest, "标注坐标无效")
				return
			}
		}
		a.Kind = models.AnnoKindHighlight
		a.Rects = body.Rects
		a.Quote = strings.TrimSpace(body.Quote)
		if len(a.Quote) > maxAnnotationQuote {
			a.Quote = a.Quote[:maxAnnotationQuote]
		}
	}
	if _, err := s.store.CreateAnnotation(&a); err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusCreated, a)
}

// handleUpdateAnnotation 修改标注：颜色 / 备注（高亮）/ 正文（批注）/ 位置（拖动批注框）。
func (s *Server) handleUpdateAnnotation(w http.ResponseWriter, r *http.Request) {
	id, ok := s.idParam(r, "id")
	if !ok {
		writeError(w, http.StatusBadRequest, "invalid annotation id")
		return
	}
	var body struct {
		Color *string  `json:"color"`
		Note  *string  `json:"note"`
		X     *float64 `json:"x"`
		Y     *float64 `json:"y"`
	}
	if err := readJSON(r, &body); err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	if body.Note != nil {
		trimmed := strings.TrimSpace(*body.Note)
		if len(trimmed) > maxAnnotationNote {
			trimmed = trimmed[:maxAnnotationNote]
		}
		body.Note = &trimmed
	}
	if (body.X == nil) != (body.Y == nil) {
		writeError(w, http.StatusBadRequest, "x 与 y 必须同时提供")
		return
	}
	if body.X != nil && (*body.X < 0 || *body.Y < 0 || *body.X > 1 || *body.Y > 1) {
		writeError(w, http.StatusBadRequest, "位置无效")
		return
	}
	if err := s.store.UpdateAnnotation(id, body.Color, body.Note, body.X, body.Y); err != nil {
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
