package api

import (
	"context"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"paper-manager/internal/ai"
	"paper-manager/internal/models"
	pdfmeta "paper-manager/internal/pdf"
)

func (s *Server) aiConfig() (ai.Config, bool) {
	key := s.store.GetSetting("ai_api_key")
	if key == "" {
		key = os.Getenv("AI_API_KEY")
	}
	if key == "" {
		return ai.Config{}, false
	}
	base := s.store.GetSetting("ai_base_url")
	if base == "" {
		base = os.Getenv("AI_BASE_URL")
	}
	if base == "" {
		base = "https://api.openai.com/v1"
	}
	model := s.store.GetSetting("ai_model")
	if model == "" {
		model = os.Getenv("AI_MODEL")
	}
	if model == "" {
		model = "gpt-4o-mini"
	}
	return ai.Config{BaseURL: base, APIKey: key, Model: model, Timeout: 45 * time.Second}, true
}

func (s *Server) maybeAIExtract(in *models.PaperInput, pdfPath string) error {
	if pdfPath == "" || !in.UseAI {
		return nil
	}
	cfg, ok := s.aiConfig()
	if !ok {
		return nil // 未配置 API Key 时不发送 AI 请求
	}
	text, err := pdfmeta.ExtractText(filepath.Join(s.uploadDir, pdfPath), 20000)
	text = strings.ReplaceAll(text, "\u0000", "")
	if err != nil || strings.TrimSpace(text) == "" {
		return nil
	}
	client := ai.NewClient(cfg)
	ctx, cancel := context.WithTimeout(context.Background(), 45*time.Second)
	defer cancel()
	result, err := client.ExtractMeta(ctx, text)
	if err != nil {
		return nil // AI 失败不阻断保存
	}
	return s.applyAIResult(in, result)
}

func (s *Server) applyAIResult(in *models.PaperInput, r ai.MetaResult) error {
	if strings.TrimSpace(in.Title) == "" {
		in.Title = strings.TrimSpace(r.Title)
	}
	if strings.TrimSpace(in.Authors) == "" {
		in.Authors = strings.TrimSpace(r.Authors)
	}
	if in.Year == 0 {
		if y, err := strconv.Atoi(strings.TrimSpace(r.Year)); err == nil {
			in.Year = y
		}
	}
	if strings.TrimSpace(in.Venue) == "" {
		in.Venue = strings.TrimSpace(r.Venue)
	}
	if strings.TrimSpace(in.DOI) == "" {
		in.DOI = strings.TrimSpace(r.DOI)
	}
	if strings.TrimSpace(in.Keywords) == "" {
		in.Keywords = strings.TrimSpace(r.Keywords)
	}
	if strings.TrimSpace(in.Summary) == "" {
		in.Summary = strings.TrimSpace(r.Summary)
	}
	if in.CategoryID == nil && strings.TrimSpace(r.Category) != "" {
		c, err := s.store.EnsureCategory(r.Category)
		if err == nil {
			in.CategoryID = &c.ID
		}
	}
	return nil
}

func (s *Server) handleGetSettings(w http.ResponseWriter, r *http.Request) {
	base := s.store.GetSetting("ai_base_url")
	if base == "" {
		base = os.Getenv("AI_BASE_URL")
	}
	model := s.store.GetSetting("ai_model")
	if model == "" {
		model = os.Getenv("AI_MODEL")
	}
	key := s.store.GetSetting("ai_api_key")
	if key == "" {
		key = os.Getenv("AI_API_KEY")
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"aiBaseUrl": base,
		"aiModel":   model,
		"hasApiKey": key != "",
	})
}

func (s *Server) handleUpdateSettings(w http.ResponseWriter, r *http.Request) {
	var body struct {
		BaseURL     string `json:"aiBaseUrl"`
		Model       string `json:"aiModel"`
		APIKey      string `json:"aiApiKey"`
		ClearAPIKey bool   `json:"clearApiKey"`
	}
	if err := readJSON(r, &body); err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	if strings.TrimSpace(body.BaseURL) != "" {
		_ = s.store.SetSetting("ai_base_url", strings.TrimSpace(body.BaseURL))
	}
	if strings.TrimSpace(body.Model) != "" {
		_ = s.store.SetSetting("ai_model", strings.TrimSpace(body.Model))
	}
	if body.ClearAPIKey {
		_ = s.store.SetSetting("ai_api_key", "")
	} else if strings.TrimSpace(body.APIKey) != "" {
		_ = s.store.SetSetting("ai_api_key", strings.TrimSpace(body.APIKey))
	}
	s.handleGetSettings(w, r)
}

func (s *Server) handleAIExtract(w http.ResponseWriter, r *http.Request) {
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
	cfg, ok := s.aiConfig()
	if !ok {
		writeJSON(w, http.StatusOK, map[string]any{"skipped": true, "reason": "no_api_key"})
		return
	}
	text, err := pdfmeta.ExtractText(filepath.Join(s.uploadDir, p.PDFPath), 20000)
	if err != nil || strings.TrimSpace(text) == "" {
		writeError(w, http.StatusBadRequest, "无法从 PDF 提取文本")
		return
	}
	client := ai.NewClient(cfg)
	ctx, cancel := context.WithTimeout(context.Background(), 45*time.Second)
	defer cancel()
	result, err := client.ExtractMeta(ctx, text)
	if err != nil {
		writeError(w, http.StatusBadGateway, "AI 提取失败: "+err.Error())
		return
	}
	in := paperToInput(&p)
	if err := s.applyAIResult(&in, result); err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	updated := s.paperFromInput(in, &p)
	if err := s.store.UpdatePaper(&updated); err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	if err := s.applyRelations(id, in); err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	paper, _ := s.store.GetPaper(id)
	writeJSON(w, http.StatusOK, paper)
}

func paperToInput(p *models.Paper) models.PaperInput {
	in := models.PaperInput{
		Title:      p.Title,
		Authors:    p.Authors,
		Year:       p.Year,
		Venue:      p.Venue,
		DOI:        p.DOI,
		Keywords:   p.Keywords,
		Link:       p.Link,
		Summary:    p.Summary,
		Notes:      p.Notes,
		CategoryID: p.CategoryID,
		Read:       p.Read,
		Starred:    p.Starred,
	}
	for _, t := range p.Tags {
		in.Tags = append(in.Tags, t.ID)
	}
	for _, c := range p.Collections {
		in.Collections = append(in.Collections, c.ID)
	}
	return in
}

func fillAIMeta(in *models.PaperInput, r ai.MetaResult) {
	if strings.TrimSpace(in.Title) == "" {
		in.Title = strings.TrimSpace(r.Title)
	}
	if strings.TrimSpace(in.Authors) == "" {
		in.Authors = strings.TrimSpace(r.Authors)
	}
	if in.Year == 0 {
		if y, err := strconv.Atoi(strings.TrimSpace(r.Year)); err == nil {
			in.Year = y
		}
	}
	if strings.TrimSpace(in.Venue) == "" {
		in.Venue = strings.TrimSpace(r.Venue)
	}
	if strings.TrimSpace(in.DOI) == "" {
		in.DOI = strings.TrimSpace(r.DOI)
	}
	if strings.TrimSpace(in.Keywords) == "" {
		in.Keywords = strings.TrimSpace(r.Keywords)
	}
	if strings.TrimSpace(in.Summary) == "" {
		in.Summary = strings.TrimSpace(r.Summary)
	}
}

func (s *Server) handleExtractPDF(w http.ResponseWriter, r *http.Request) {
	size := s.maxUploadMB * 1024 * 1024
	r.Body = http.MaxBytesReader(w, r.Body, size)
	if err := r.ParseMultipartForm(size); err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	useAI := r.FormValue("useAI") == "1" || strings.EqualFold(r.FormValue("useAI"), "true")
	name, _, origName, err := s.savePdf(r)
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	if name == "" {
		writeError(w, http.StatusBadRequest, "pdf file required")
		return
	}
	defer os.Remove(filepath.Join(s.uploadDir, name))
	meta, err := pdfmeta.ExtractWithFallback(filepath.Join(s.uploadDir, name), origName)
	if err != nil {
		writeError(w, http.StatusBadRequest, "无法解析 PDF: "+err.Error())
		return
	}
	in := models.PaperInput{
		Title:    meta.Title,
		Authors:  meta.Author,
		Keywords: meta.Keywords,
		UseAI:    useAI,
	}
	aiUsed := false
	if useAI {
		if cfg, ok := s.aiConfig(); ok {
			text, terr := pdfmeta.ExtractText(filepath.Join(s.uploadDir, name), 20000)
			if terr == nil && strings.TrimSpace(text) != "" {
				client := ai.NewClient(cfg)
				ctx, cancel := context.WithTimeout(context.Background(), 45*time.Second)
				defer cancel()
				if result, aerr := client.ExtractMeta(ctx, text); aerr == nil {
					fillAIMeta(&in, result)
					aiUsed = true
				}
			}
		}
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"title":    in.Title,
		"authors":  in.Authors,
		"year":     in.Year,
		"venue":    in.Venue,
		"doi":      in.DOI,
		"keywords": in.Keywords,
		"summary":  in.Summary,
		"aiUsed":   aiUsed,
	})
}
