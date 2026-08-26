package api

import (
	"context"
	"fmt"
	"net/http"
	"strings"
	"time"

	"paper-manager/internal/ai"
	"paper-manager/internal/models"
)

type askSource struct {
	ID      int64  `json:"id"`
	Title   string `json:"title"`
	Excerpt string `json:"excerpt,omitempty"`
}

func (s *Server) handleAsk(w http.ResponseWriter, r *http.Request) {
	var body struct {
		Query string `json:"query"`
	}
	if err := readJSON(r, &body); err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	query := strings.TrimSpace(body.Query)
	if query == "" {
		writeError(w, http.StatusBadRequest, "query required")
		return
	}
	items, err := s.store.SearchRelaxed(query, 3)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	fallback := false
	if len(items) == 0 {
		// 未命中时回退到最近论文，避免中文提问英文库直接空召回
		res, lerr := s.store.ListPapers(models.PaperQuery{Page: 1, PageSize: 3, Sort: "created", Order: "desc"})
		if lerr != nil {
			writeError(w, http.StatusInternalServerError, lerr.Error())
			return
		}
		items = res.Papers
		fallback = true
	}
	if len(items) == 0 {
		writeJSON(w, http.StatusOK, map[string]any{
			"answer":  "论文库里没有找到与这个问题相关的论文。换个问法试试，或先上传相关论文。",
			"sources": []askSource{},
		})
		return
	}
	cfg, ok := s.aiConfig()
	if !ok {
		writeError(w, http.StatusServiceUnavailable, "未配置 AI API Key")
		return
	}
	var ctx strings.Builder
	sources := []askSource{}
	for i, p := range items {
		text := s.store.PaperFullText(p.ID)
		excerpts := excerptsAround(text, query, 3, 500)
		if len(excerpts) == 0 && len(text) > 800 {
			excerpts = []string{text[:800]}
		}
		excerpt := strings.Join(excerpts, "\n…\n")
		if len(excerpt) > 1800 {
			excerpt = excerpt[:1800]
		}
		fmt.Fprintf(&ctx, "[%d] 标题：%s\n作者：%s\n年份：%d\n摘要：%s\n论文片段：\n%s\n\n",
			i+1, p.Title, p.Authors, p.Year, truncateStr(p.Summary, 800), excerpt)
		sources = append(sources, askSource{ID: p.ID, Title: p.Title, Excerpt: truncateStr(excerpt, 1000)})
	}
	client := ai.NewClient(cfg)
	ctx2, cancel := context.WithTimeout(context.Background(), 120*time.Second)
	defer cancel()
	note := ""
	if fallback {
		note = "\n（提示：以下是最新的几篇论文，并不一定与问题直接相关；若它们没有提到你问的内容，请明确说明论文库中没有找到相关内容，不要编造。）"
	}
	answer, aerr := client.Ask(ctx2, fmt.Sprintf("论文库相关片段：\n%s\n%s\n\n问题：%s", ctx.String(), note, query))
	if aerr != nil {
		writeError(w, http.StatusBadGateway, "AI 问答失败: "+aerr.Error())
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"answer": answer, "sources": sources})
}

// excerptsAround 返回文本中最多 count 个包含查询关键词的上下文片段。
func excerptsAround(text, query string, count, length int) []string {
	low := strings.ToLower(text)
	tokens := apiTokens(query)
	if len(tokens) == 0 {
		return nil
	}
	var out []string
	searchFrom := 0
	for len(out) < count {
		best := -1
		for _, tok := range tokens {
			if i := strings.Index(low[searchFrom:], tok); i >= 0 {
				pos := searchFrom + i
				if best < 0 || pos < best {
					best = pos
				}
			}
		}
		if best < 0 {
			break
		}
		start := best - length/2
		if start < 0 {
			start = 0
		}
		end := start + length
		if end > len(text) {
			end = len(text)
		}
		out = append(out, strings.TrimSpace(text[start:end]))
		searchFrom = end
	}
	return out
}

func apiTokens(q string) []string {
	q = strings.ToLower(strings.TrimSpace(q))
	out := []string{}
	var cur strings.Builder
	flush := func() {
		if cur.Len() > 0 {
			out = append(out, cur.String())
			cur.Reset()
		}
	}
	for _, r := range q {
		if (r >= 'a' && r <= 'z') || (r >= '0' && r <= '9') {
			cur.WriteRune(r)
		} else if r > 0x2e80 {
			flush()
			out = append(out, string(r))
		} else {
			flush()
		}
	}
	flush()
	return out
}

func truncateStr(s string, max int) string {
	if len(s) <= max {
		return s
	}
	return s[:max]
}
