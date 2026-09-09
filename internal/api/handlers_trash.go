package api

import (
	"errors"
	"net/http"
	"os"
	"path/filepath"

	"paper-manager/internal/store"
)

// handleListTrash 列出回收站中的论文。
func (s *Server) handleListTrash(w http.ResponseWriter, r *http.Request) {
	papers, err := s.store.ListTrash()
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"papers": papers, "total": len(papers)})
}

// handleRestorePaper 从回收站恢复论文（PDF 一直在，无需还原文件）。
func (s *Server) handleRestorePaper(w http.ResponseWriter, r *http.Request) {
	id, ok := s.idParam(r, "id")
	if !ok {
		writeError(w, http.StatusBadRequest, "invalid paper id")
		return
	}
	if err := s.store.RestorePaper(id); err != nil {
		if errors.Is(err, store.ErrConflict) {
			writeError(w, http.StatusNotFound, "回收站中没有这篇论文")
			return
		}
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	paper, err := s.store.GetPaper(id)
	if err != nil {
		writeJSON(w, http.StatusOK, map[string]bool{"ok": true})
		return
	}
	writeJSON(w, http.StatusOK, paper)
}

// handlePurgePaper 彻底删除（同时删掉 PDF 文件），不可恢复。
func (s *Server) handlePurgePaper(w http.ResponseWriter, r *http.Request) {
	id, ok := s.idParam(r, "id")
	if !ok {
		writeError(w, http.StatusBadRequest, "invalid paper id")
		return
	}
	pdfPath, err := s.store.PurgePaper(id)
	if err != nil {
		if errors.Is(err, store.ErrConflict) {
			writeError(w, http.StatusNotFound, "回收站中没有这篇论文")
			return
		}
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	if pdfPath != "" {
		os.Remove(filepath.Join(s.uploadDir, pdfPath))
	}
	writeJSON(w, http.StatusOK, map[string]bool{"ok": true})
}

// handleEmptyTrash 清空回收站。
func (s *Server) handleEmptyTrash(w http.ResponseWriter, r *http.Request) {
	paths, err := s.store.EmptyTrash()
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	for _, name := range paths {
		os.Remove(filepath.Join(s.uploadDir, name))
	}
	writeJSON(w, http.StatusOK, map[string]any{"ok": true, "removed": len(paths)})
}
