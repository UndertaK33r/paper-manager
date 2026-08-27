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
	prompt := "你是学术论文元数据提取助手。请从下面论文文本中提取：title(标题)，authors(作者，逗号分隔)，year(发表年份数字)，venue(期刊或会议名)，doi，keywords(关键词，逗号分隔)，summary(一句话中文总结)，category(建议分类名，如 深度学习/NLP/系统/数据库)。只输出 JSON 对象，不要输出其他文字。\n\n论文文本：\n" + truncate(text)
	payload := map[string]any{
		"model": c.cfg.Model,
		"messages": []map[string]string{
			{"role": "system", "content": "你是一个严谨的学术元数据提取器。输出 JSON，字段名必须为：title, authors, year, venue, doi, keywords, summary, category。没有的字段填空字符串。"},
			{"role": "user", "content": prompt},
		},
		"temperature": 0.2,
		"max_tokens":  1000,
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
	content = strings.TrimPrefix(content, "```json")
	content = strings.TrimPrefix(content, "```")
	content = strings.TrimSuffix(content, "```")
	content = strings.TrimSpace(content)
	var meta MetaResult
	if err := json.Unmarshal([]byte(content), &meta); err != nil {
		return MetaResult{}, fmt.Errorf("parse AI JSON: %w (content=%s)", err, truncateN(content, 300))
	}
	return meta, nil
}

func truncate(s string) string { return truncateN(s, 20000) }

func truncateN(s string, max int) string {
	if len(s) <= max {
		return s
	}
	return s[:max]
}

func Defaults() Config {
	return Config{
		BaseURL: "https://tokendance.space/gateway/v1",
		Model:   "deepseek-v3.2",
		Timeout: 120 * time.Second,
	}
}

func (c *Client) Summarize(ctx context.Context, paperContext string) (string, error) {
	if strings.TrimSpace(c.cfg.APIKey) == "" {
		return "", fmt.Errorf("AI_API_KEY not configured")
	}
	url := strings.TrimRight(c.cfg.BaseURL, "/") + "/chat/completions"
	payload := map[string]any{
		"model": c.cfg.Model,
		"messages": []map[string]string{
			{"role": "system", "content": "你是学术论文阅读助手。请用中文输出结构化总结，按四段：1) 研究问题 2) 方法 3) 主要结果 4) 意义。每段 1-3 句话，语言精炼，不要输出无关内容。"},
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
