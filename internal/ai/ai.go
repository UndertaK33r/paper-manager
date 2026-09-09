package ai

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"
)

type Config struct {
	BaseURL string
	APIKey  string
	Model   string
	Timeout time.Duration
}

type Client struct {
	cfg  Config
	http *http.Client
}

type MetaResult struct {
	Title    string `json:"title"`
	Authors  string `json:"authors"`
	Year     string `json:"year"`
	Venue    string `json:"venue"`
	DOI      string `json:"doi"`
	Keywords string `json:"keywords"`
	Summary  string `json:"summary"`
	Category string `json:"category"`
}

func NewClient(cfg Config) *Client {
	if cfg.Timeout <= 0 {
		cfg.Timeout = 120 * time.Second
	}
	return &Client{cfg: cfg, http: &http.Client{Timeout: cfg.Timeout}}
}

func (c *Client) ExtractMeta(ctx context.Context, text string) (MetaResult, error) {
	if strings.TrimSpace(c.cfg.APIKey) == "" {
		return MetaResult{}, fmt.Errorf("AI_API_KEY not configured")
	}
	url := strings.TrimRight(c.cfg.BaseURL, "/") + "/chat/completions"
	prompt := "你是学术论文元数据提取助手。请从下面论文文本中提取：title(标题)，authors(作者，逗号分隔)，year(发表年份，如 \"2025\")，venue(期刊或会议名)，doi，keywords(关键词，逗号分隔)，summary(一句话中文总结)，category(建议分类名，如 深度学习/NLP/系统/数据库)。只输出 JSON 对象，不要输出其他文字。\n\n论文文本：\n" + truncate(text)
	payload := map[string]any{
		"model": c.cfg.Model,
		"messages": []map[string]string{
			{"role": "system", "content": "你是一个严谨的学术元数据提取器。输出 JSON，字段名必须为：title, authors, year, venue, doi, keywords, summary, category。没有的字段填空字符串。"},
			{"role": "user", "content": prompt},
		},
		"temperature": 0.2,
		"max_tokens":  2000,
	}
	data, err := json.Marshal(payload)
	if err != nil {
		return MetaResult{}, err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(data))
	if err != nil {
		return MetaResult{}, err
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+c.cfg.APIKey)
	resp, err := c.http.Do(req)
	if err != nil {
		return MetaResult{}, err
	}
	defer resp.Body.Close()
	body, err := io.ReadAll(io.LimitReader(resp.Body, 4<<20))
	if err != nil {
		return MetaResult{}, err
	}
	if resp.StatusCode != http.StatusOK {
		return MetaResult{}, fmt.Errorf("AI API %d: %s", resp.StatusCode, strings.TrimSpace(string(body)))
	}
	var envelope struct {
		Choices []struct {
			Message struct {
				Content string `json:"content"`
			} `json:"message"`
		} `json:"choices"`
	}
	if err := json.Unmarshal(body, &envelope); err != nil {
		return MetaResult{}, err
	}
	if len(envelope.Choices) == 0 {
		return MetaResult{}, fmt.Errorf("AI API returned no choices")
	}
	content := strings.TrimSpace(envelope.Choices[0].Message.Content)
	// 模型可能在 JSON 前后附说明文字或 markdown 围栏，按括号配平截取对象
	if obj := extractJSONObject(content); obj != "" {
		content = obj
	}
	var meta MetaResult
	if err := json.Unmarshal([]byte(content), &meta); err != nil {
		// 输出被 max_tokens 截断时 JSON 不完整：截到最后一个完整的
		// 顶层字段并补上闭合括号，保住已生成的字段（宁缺毋全崩）
		if repaired := repairTruncatedJSON(content); repaired != "" {
			if err2 := json.Unmarshal([]byte(repaired), &meta); err2 == nil {
				return meta, nil
			}
		}
		return MetaResult{}, fmt.Errorf("parse AI JSON: %w (content=%s)", err, truncateN(content, 300))
	}
	return meta, nil
}

// repairTruncatedJSON 修复被截断的 JSON 对象：扫到最后一个"完整的顶层
// 成员"的结尾，裁掉其后未写完的部分，再补 "}"。无法定位时返回空串。
func repairTruncatedJSON(s string) string {
	start := strings.Index(s, "{")
	if start < 0 {
		return ""
	}
	valueEnd := -1 // 最近一个顶层成员 value 结束的位置（含闭合符）
	depth := 0
	inStr := false
	esc := false
	pendingValue := false // 冒号之后：下一个顶层字符串是值而不是键名
	for i := start; i < len(s); i++ {
		c := s[i]
		if inStr {
			switch {
			case esc:
				esc = false
			case c == '\\':
				esc = true
			case c == '"':
				inStr = false
				if depth == 1 && pendingValue {
					valueEnd = i // 顶层字符串值结束
					pendingValue = false
				}
			}
			continue
		}
		switch c {
		case '"':
			inStr = true
		case '{', '[':
			depth++
		case '}', ']':
			depth--
			if depth == 1 {
				valueEnd = i // 完整的嵌套值（对象/数组）结束
				pendingValue = false
			}
			if depth == 0 {
				return "" // 本来就是完整对象，无需修复
			}
		case ':':
			if depth == 1 {
				pendingValue = true
			}
		case ',':
			if depth == 1 {
				pendingValue = false // 成员结束，之后是下一个键名
			}
		default:
			// 数字 / true / false / null 的内容字符推进边界（仅在值的位置）；
			// 空白不推进，避免冒号后、值开始前把边界推过头
			if depth == 1 && pendingValue && c != ' ' && c != '\n' && c != '\r' && c != '\t' {
				valueEnd = i
			}
		}
	}
	if valueEnd <= start {
		return ""
	}
	cut := strings.TrimRight(s[start:valueEnd+1], ", \n\r\t")
	return cut + "}"
}

// extractJSONObject 从任意文本中截取第一个完整的 JSON 对象（括号配平，
// 忽略字符串内的花括号）。找不到时返回空串。
func extractJSONObject(s string) string {
	start := strings.Index(s, "{")
	if start < 0 {
		return ""
	}
	depth := 0
	inStr := false
	esc := false
	for i := start; i < len(s); i++ {
		c := s[i]
		if inStr {
			switch {
			case esc:
				esc = false
			case c == '\\':
				esc = true
			case c == '"':
				inStr = false
			}
			continue
		}
		switch c {
		case '"':
			inStr = true
		case '{':
			depth++
		case '}':
			depth--
			if depth == 0 {
				return s[start : i+1]
			}
		}
	}
	return ""
}

// UnmarshalJSON 宽容地解析模型输出：LLM 的 JSON 字段类型经常抖动
// （year 可能是数字、authors/keywords 可能是数组），统一转成字符串，
// 避免 "cannot unmarshal number into Go struct field" 之类失败。
func (m *MetaResult) UnmarshalJSON(data []byte) error {
	var raw map[string]json.RawMessage
	if err := json.Unmarshal(data, &raw); err != nil {
		return err
	}
	m.Title = flexString(raw["title"])
	m.Authors = flexString(raw["authors"])
	m.Year = flexString(raw["year"])
	m.Venue = flexString(raw["venue"])
	m.DOI = flexString(raw["doi"])
	m.Keywords = flexString(raw["keywords"])
	m.Summary = flexString(raw["summary"])
	m.Category = flexString(raw["category"])
	return nil
}

// flexString 把任意 JSON 值宽容地转成字符串：字符串原样、数字/布尔
// 转文本、数组（字符串元素）逗号拼接、null/对象返回空串。
func flexString(raw json.RawMessage) string {
	if len(raw) == 0 {
		return ""
	}
	var s string
	if err := json.Unmarshal(raw, &s); err == nil {
		return strings.TrimSpace(s)
	}
	var n json.Number
	if err := json.Unmarshal(raw, &n); err == nil {
		return n.String()
	}
	var arr []json.RawMessage
	if err := json.Unmarshal(raw, &arr); err == nil {
		parts := make([]string, 0, len(arr))
		for _, it := range arr {
			if v := flexString(it); v != "" {
				parts = append(parts, v)
			}
		}
		return strings.Join(parts, ", ")
	}
	var b bool
	if err := json.Unmarshal(raw, &b); err == nil && b {
		return "true"
	}
	return ""
}

func truncate(s string) string { return truncateN(s, 20000) }

func truncateN(s string, max int) string {
	if len(s) <= max {
		return s
	}
	return s[:max]
}

func (c *Client) Summarize(ctx context.Context, paperContext string) (string, error) {
	if strings.TrimSpace(c.cfg.APIKey) == "" {
		return "", fmt.Errorf("AI_API_KEY not configured")
	}
	url := strings.TrimRight(c.cfg.BaseURL, "/") + "/chat/completions"
	payload := map[string]any{
		"model": c.cfg.Model,
		"messages": []map[string]string{
			{"role": "system", "content": "你是学术论文阅读助手。请用中文、以 Markdown 格式输出结构化总结，包含四个小节，用 ### 标题：研究问题、方法、主要结果、意义；要点用 - 列表逐条列出，关键术语与重要结论用 **加粗**。语言精炼，不要输出无关内容。"},
			{"role": "user", "content": paperContext},
		},
		"temperature": 0.3,
		"max_tokens":  800,
	}
	data, err := json.Marshal(payload)
	if err != nil {
		return "", err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(data))
	if err != nil {
		return "", err
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+c.cfg.APIKey)
	resp, err := c.http.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()
	body, err := io.ReadAll(io.LimitReader(resp.Body, 4<<20))
	if err != nil {
		return "", err
	}
	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("AI API %d: %s", resp.StatusCode, strings.TrimSpace(string(body)))
	}
	var envelope struct {
		Choices []struct {
			Message struct {
				Content string `json:"content"`
			} `json:"message"`
		} `json:"choices"`
	}
	if err := json.Unmarshal(body, &envelope); err != nil {
		return "", err
	}
	if len(envelope.Choices) == 0 {
		return "", fmt.Errorf("AI API returned no choices")
	}
	return strings.TrimSpace(envelope.Choices[0].Message.Content), nil
}

func (c *Client) Ask(ctx context.Context, prompt string) (string, error) {
	return c.chat(ctx, []map[string]string{
		{"role": "system", "content": "你是一名严谨的学术助手，正在帮助用户梳理其私人论文库。回答要求：1. 只依据提供的论文片段回答，用 [1]、[2] 标注引用来源编号；2. 回答要具体、有针对性：直接引用论文中的方法名称、实验数据、结论或原文表述，不要泛泛而谈；3. 若片段信息不足以回答某个方面，明确说明论文库片段未提及；4. 用中文回答，先给结论再展开，适当使用小标题与要点。"},
		{"role": "user", "content": prompt},
	}, 1200)
}

func (c *Client) chat(ctx context.Context, messages []map[string]string, maxTokens int) (string, error) {
	if strings.TrimSpace(c.cfg.APIKey) == "" {
		return "", fmt.Errorf("AI_API_KEY not configured")
	}
	url := strings.TrimRight(c.cfg.BaseURL, "/") + "/chat/completions"
	payload := map[string]any{
		"model":       c.cfg.Model,
		"messages":    messages,
		"temperature": 0.3,
		"max_tokens":  maxTokens,
	}
	data, err := json.Marshal(payload)
	if err != nil {
		return "", err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(data))
	if err != nil {
		return "", err
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+c.cfg.APIKey)
	resp, err := c.http.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()
	body, err := io.ReadAll(io.LimitReader(resp.Body, 4<<20))
	if err != nil {
		return "", err
	}
	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("AI API %d: %s", resp.StatusCode, strings.TrimSpace(string(body)))
	}
	var envelope struct {
		Choices []struct {
			Message struct {
				Content string `json:"content"`
			} `json:"message"`
		} `json:"choices"`
	}
	if err := json.Unmarshal(body, &envelope); err != nil {
		return "", err
	}
	if len(envelope.Choices) == 0 {
		return "", fmt.Errorf("AI API returned no choices")
	}
	return strings.TrimSpace(envelope.Choices[0].Message.Content), nil
}

func (c *Client) Test(ctx context.Context) (string, error) {
	return c.chat(ctx, []map[string]string{
		{"role": "user", "content": "请只回复两个字：正常"},
	}, 20)
}

// Translate 把英文学术论文片段翻译成简体中文。
func (c *Client) Translate(ctx context.Context, text string) (string, error) {
	if strings.TrimSpace(c.cfg.APIKey) == "" {
		return "", fmt.Errorf("AI_API_KEY not configured")
	}
	url := strings.TrimRight(c.cfg.BaseURL, "/") + "/chat/completions"
	payload := map[string]any{
		"model": c.cfg.Model,
		"messages": []map[string]string{
			{"role": "system", "content": "你是专业的学术翻译。把用户给出的英文学术论文片段翻译成简体中文：忠实原文、术语准确、语句通顺；保持原文段落顺序；数学符号、变量名、模型名/数据集名等专有名词保留英文。只输出译文，不要解释。"},
			{"role": "user", "content": text},
		},
		"temperature": 0.2,
		"max_tokens":  4000,
	}
	data, err := json.Marshal(payload)
	if err != nil {
		return "", err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(data))
	if err != nil {
		return "", err
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+c.cfg.APIKey)
	resp, err := c.http.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()
	body, err := io.ReadAll(io.LimitReader(resp.Body, 8<<20))
	if err != nil {
		return "", err
	}
	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("AI API %d: %s", resp.StatusCode, strings.TrimSpace(string(body)))
	}
	var envelope struct {
		Choices []struct {
			Message struct {
				Content string `json:"content"`
			} `json:"message"`
		} `json:"choices"`
	}
	if err := json.Unmarshal(body, &envelope); err != nil {
		return "", err
	}
	if len(envelope.Choices) == 0 {
		return "", fmt.Errorf("AI API returned no choices")
	}
	return strings.TrimSpace(envelope.Choices[0].Message.Content), nil
}
