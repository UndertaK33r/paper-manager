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
	"paper-manager/internal/meta"
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
		base = "https://tokendance.space/gateway/v1"
	}
	model := s.store.GetSetting("ai_model")
	if model == "" {
		model = os.Getenv("AI_MODEL")
	}
	if model == "" {
		model = "deepseek-v3.2"
	}
	return ai.Config{BaseURL: base, APIKey: key, Model: model, Timeout: 45 * time.Second}, true
}

func (s *Server) maybeEnrich(in *models.PaperInput, pdfPath string) error {
	if pdfPath == "" {
		return nil
	}
	text, err := pdfmeta.ExtractText(filepath.Join(s.uploadDir, pdfPath), 20000)
	if err != nil || strings.TrimSpace(text) == "" {
		return nil
	}
	if strings.TrimSpace(in.DOI) == "" {
		in.DOI = meta.SniffDOI(text)
	}
	if strings.TrimSpace(in.Title) == "" {
		in.Title = meta.SniffTitle(text)
	}
	localTitle := strings.TrimSpace(in.Title)
	var info *meta.Info
	if strings.TrimSpace(in.DOI) != "" {
		// 带上本地标题做校验：DOI 对不上号（多半来自参考文献）就不采信
		info, _ = meta.EnrichByDOI(context.Background(), in.DOI, localTitle)
		if info == nil {
			// DOI 不可信时不写入，避免留下错误溯源信息
			if meta.TitlePlausible(localTitle) {
				in.DOI = ""
			}
		}
	}
	if info == nil && meta.TitlePlausible(localTitle) && meta.TitleTokens(localTitle) >= 3 {
		info, _ = meta.SearchByTitle(context.Background(), localTitle)
	}
	if info == nil {
		return nil
	}
	if strings.TrimSpace(in.Title) == "" {
		in.Title = strings.TrimSpace(info.Title)
	}
	if strings.TrimSpace(in.Authors) == "" && len(info.Authors) > 0 {
		in.Authors = strings.Join(info.Authors, ", ")
	}
	if in.Year == 0 && info.Year > 0 {
		in.Year = info.Year
	}
	if strings.TrimSpace(in.Venue) == "" {
		in.Venue = strings.TrimSpace(info.Venue)
	}
	if strings.TrimSpace(in.DOI) == "" {
		in.DOI = strings.TrimSpace(info.DOI)
	}
	return nil
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
	ctx, cancel := context.WithTimeout(context.Background(), 120*time.Second)
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
	if base == "" {
		base = "https://tokendance.space/gateway/v1"
	}
	model := s.store.GetSetting("ai_model")
	if model == "" {
		model = os.Getenv("AI_MODEL")
	}
	if model == "" {
		model = "deepseek-v3.2"
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
	ctx, cancel := context.WithTimeout(context.Background(), 120*time.Second)
	defer cancel()
	result, err := client.ExtractMeta(ctx, text)
	if err != nil {
		writeError(w, http.StatusBadGateway, "AI 提取失败: "+err.Error())
		return
	}
	if p.FullText == "" {
		p.FullText = s.store.PaperFullText(id)
	}
	in := paperToInput(&p)
	if err := s.applyAIResult(&in, result); err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	updated := s.paperFromInput(in, &p)
	// AI 提取只写元数据字段，避免慢请求期间用旧记录覆盖并发保存的笔记/状态
	if err := s.store.UpdateMetadata(&updated, false); err != nil {
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
	_ = s.maybeEnrich(&in, name)
	aiUsed := false
	if useAI {
		if cfg, ok := s.aiConfig(); ok {
			text, terr := pdfmeta.ExtractText(filepath.Join(s.uploadDir, name), 20000)
			if terr == nil && strings.TrimSpace(text) != "" {
				client := ai.NewClient(cfg)
				ctx, cancel := context.WithTimeout(context.Background(), 120*time.Second)
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

func (s *Server) handleSummarize(w http.ResponseWriter, r *http.Request) {
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
	cfg, ok := s.aiConfig()
	if !ok {
		writeError(w, http.StatusServiceUnavailable, "未配置 AI API Key")
		return
	}
	text := p.FullText
	if text == "" && p.HasPDF {
		text, _ = pdfmeta.ExtractText(filepath.Join(s.uploadDir, p.PDFPath), 8000)
	}
	if len(text) > 8000 {
		text = text[:8000]
	}
	if strings.TrimSpace(text) == "" {
		writeError(w, http.StatusBadRequest, "论文没有可用的正文文本")
		return
	}
	paperContext := "标题：" + p.Title + "\n作者：" + p.Authors + "\n年份：" + strconv.Itoa(p.Year) + "\n期刊/会议：" + p.Venue + "\n现有关键词：" + p.Keywords + "\n正文片段：\n" + text
	client := ai.NewClient(cfg)
	ctx, cancel := context.WithTimeout(context.Background(), 120*time.Second)
	defer cancel()
	summary, serr := client.Summarize(ctx, paperContext)
	if serr != nil {
		writeError(w, http.StatusBadGateway, "AI 总结失败: "+serr.Error())
		return
	}
	if err := s.store.UpdateSummary(id, summary); err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	paper, _ := s.store.GetPaper(id)
	writeJSON(w, http.StatusOK, paper)
}

// handleAITest 测试 AI 连接（模型 + API Key 是否可用）。
func (s *Server) handleAITest(w http.ResponseWriter, r *http.Request) {
	cfg, ok := s.aiConfig()
	if !ok {
		writeError(w, http.StatusServiceUnavailable, "未配置 API Key")
		return
	}
	client := ai.NewClient(cfg)
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	reply, err := client.Test(ctx)
	if err != nil {
		writeError(w, http.StatusBadGateway, "连接失败："+err.Error())
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"ok": true, "reply": reply, "model": cfg.Model, "baseUrl": cfg.BaseURL})
}

// handleGetTranslation 返回论文的已存译文。
func (s *Server) handleGetTranslation(w http.ResponseWriter, r *http.Request) {
	id, ok := s.idParam(r, "id")
	if !ok {
		writeError(w, http.StatusBadRequest, "invalid paper id")
		return
	}
	text, err := s.store.GetTranslation(id)
	if err != nil {
		text = ""
	}
	writeJSON(w, http.StatusOK, map[string]any{"translation": text})
}

// handleTranslatePaper 把论文全文分块交给 AI 翻译成中文并存储。
// 单块失败自动重试一次；仍失败时保存已完成部分并带 warning 返回。
func (s *Server) handleTranslatePaper(w http.ResponseWriter, r *http.Request) {
	id, ok := s.idParam(r, "id")
	if !ok {
		writeError(w, http.StatusBadRequest, "invalid paper id")
		return
	}
	cfg, ok := s.aiConfig()
	if !ok {
		writeError(w, http.StatusServiceUnavailable, "未配置 AI API Key")
		return
	}
	cfg.Timeout = 180 * time.Second // 单块翻译输出较长，放宽超时

	text := s.store.PaperFullText(id)
	if strings.TrimSpace(text) == "" {
		if p, perr := s.store.GetPaper(id); perr == nil && p.HasPDF && p.PDFPath != "" {
			text, _ = pdfmeta.ExtractText(filepath.Join(s.uploadDir, p.PDFPath), 200000)
		}
	}
	if strings.TrimSpace(text) == "" {
		writeError(w, http.StatusBadRequest, "论文没有可用全文，请先上传 PDF 或重新提取全文")
		return
	}
	const (
		chunkSize = 6000
		maxTotal  = 60000 // 防 token 失控
	)
	if len(text) > maxTotal {
		text = text[:maxTotal]
	}
	chunks := ai.SplitChunks(text, chunkSize)

	client := ai.NewClient(cfg)
	ctx, cancel := context.WithTimeout(r.Context(), 30*time.Minute)
	defer cancel()

	done := make([]string, 0, len(chunks))
	var firstErr error
	for _, chunk := range chunks {
		out, err := client.Translate(ctx, chunk)
		if err != nil {
			time.Sleep(time.Second)
			out, err = client.Translate(ctx, chunk) // 失败重试一次
		}
		if err != nil {
			firstErr = err
			break
		}
		done = append(done, out)
	}
	if len(done) == 0 {
		writeError(w, http.StatusBadGateway, "AI 翻译失败: "+errString(firstErr))
		return
	}
	translated := strings.Join(done, "\n\n")
	if err := s.store.UpdateTranslation(id, translated); err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	resp := map[string]any{"translation": translated, "chunks": len(chunks), "translated": len(done)}
	if firstErr != nil {
		resp["warning"] = "部分内容翻译失败，已保存完成部分；可点「重新翻译」重试"
	}
	writeJSON(w, http.StatusOK, resp)
}

func errString(err error) string {
	if err == nil {
		return ""
	}
	return err.Error()
}

// handleTranslateText 即时翻译一小段文本（粘贴难句）。
func (s *Server) handleTranslateText(w http.ResponseWriter, r *http.Request) {
	cfg, ok := s.aiConfig()
	if !ok {
		writeError(w, http.StatusServiceUnavailable, "未配置 AI API Key")
		return
	}
	var body struct {
		Text string `json:"text"`
	}
	if err := readJSON(r, &body); err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	text := strings.TrimSpace(body.Text)
	if text == "" {
		writeError(w, http.StatusBadRequest, "text required")
		return
	}
	if len(text) > 4000 {
		text = text[:4000]
	}
	cfg.Timeout = 60 * time.Second
	client := ai.NewClient(cfg)
	ctx, cancel := context.WithTimeout(r.Context(), 90*time.Second)
	defer cancel()
	out, err := client.Translate(ctx, text)
	if err != nil {
		writeError(w, http.StatusBadGateway, "AI 翻译失败: "+err.Error())
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"translation": out})
}
