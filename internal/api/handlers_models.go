package api

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/url"
	"os"
	"sort"
	"strings"
	"time"
)

// 模型列表端点安全约束：上游响应上限、请求体上限、超时。
const (
	aiModelsDefaultBase   = "https://tokendance.space/gateway/v1"
	aiModelsUpstreamLimit = 1 << 20  // 1 MiB
	aiModelsBodyLimit     = 16 << 10 // 16 KiB
	aiModelsTimeout       = 8 * time.Second
)

var errAIModelsUpstream = errors.New("upstream unavailable")

type aiModelItem struct {
	ID            string `json:"id"`
	Name          string `json:"name"`
	ContextLength int    `json:"context_length"`
}

type aiModelsResponse struct {
	Models  []aiModelItem `json:"models"`
	Source  string        `json:"source"` // upstream | unavailable
	Warning string        `json:"warning,omitempty"`
	BaseURL string        `json:"baseUrl,omitempty"`
}

// handleAIModels 拉取 OpenAI 兼容网关的模型列表。
// 存储的 API Key 只发给与配置地址完全一致的规范化地址；
// 显式传入的 apiKey 只用于本次请求，"" 表示匿名拉取。
func (s *Server) handleAIModels(w http.ResponseWriter, r *http.Request) {
	var body struct {
		Base   string  `json:"base"`
		APIKey *string `json:"apiKey"`
	}
	if r.Method == http.MethodPost {
		r.Body = http.MaxBytesReader(w, r.Body, aiModelsBodyLimit)
		data, err := io.ReadAll(r.Body)
		if err != nil {
			writeError(w, http.StatusRequestEntityTooLarge, "请求体过大")
			return
		}
		if len(bytes.TrimSpace(data)) > 0 {
			if err := json.Unmarshal(data, &body); err != nil {
				writeError(w, http.StatusBadRequest, "请求体不是合法 JSON")
				return
			}
		}
	}

	cfgBase, cfgKey := s.aiModelsCredentials()
	rawTarget := strings.TrimSpace(body.Base)
	if rawTarget == "" {
		rawTarget = strings.TrimSpace(r.URL.Query().Get("base"))
	}

	normCfg, cfgErr := normalizeAIModelBase(cfgBase)
	var target string
	if rawTarget != "" {
		norm, err := normalizeAIModelBase(rawTarget)
		if err != nil {
			writeError(w, http.StatusBadRequest, "AI 服务地址无效")
			return
		}
		target = norm
	} else if cfgErr == nil {
		target = normCfg
	} else {
		writeJSON(w, http.StatusOK, aiModelsResponse{
			Models: []aiModelItem{}, Source: "unavailable",
			Warning: "AI 服务地址无效，请先在设置中检查服务地址",
		})
		return
	}

	key := ""
	switch {
	case body.APIKey != nil:
		key = strings.TrimSpace(*body.APIKey)
	case cfgErr == nil && normCfg == target:
		key = cfgKey
	}

	items, err := fetchAIModelList(r.Context(), target, key)
	if err != nil {
		// 失败响应不带 baseUrl：错误信息一律不回显地址。
		writeJSON(w, http.StatusOK, aiModelsResponse{
			Models: []aiModelItem{}, Source: "unavailable",
			Warning: "模型列表获取失败，请检查服务地址与 API Key 是否正确",
		})
		return
	}
	writeJSON(w, http.StatusOK, aiModelsResponse{Models: items, Source: "upstream", BaseURL: target})
}

func (s *Server) aiModelsCredentials() (base, key string) {
	base = s.store.GetSetting("ai_base_url")
	if base == "" {
		base = os.Getenv("AI_BASE_URL")
	}
	if base == "" {
		base = aiModelsDefaultBase
	}
	key = s.store.GetSetting("ai_api_key")
	if key == "" {
		key = os.Getenv("AI_API_KEY")
	}
	return base, key
}

// normalizeAIModelBase 规范化服务地址用于严格比较：
// 小写 scheme/host、去掉末尾斜杠、保留完整路径；拒绝非 HTTP(S)、缺 host、
// 携带用户信息或查询/片段的地址。
func normalizeAIModelBase(raw string) (string, error) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return "", errors.New("empty base url")
	}
	u, err := url.Parse(raw)
	if err != nil {
		return "", err
	}
	if (u.Scheme != "http" && u.Scheme != "https") || u.Host == "" || u.User != nil ||
		u.RawQuery != "" || u.Fragment != "" {
		return "", errors.New("unsupported base url")
	}
	u.Scheme = strings.ToLower(u.Scheme)
	u.Host = strings.ToLower(u.Host)
	u.Path = strings.TrimRight(u.Path, "/")
	u.RawPath = ""
	return u.String(), nil
}

type aiModelEntry struct {
	ID                 string   `json:"id"`
	Name               string   `json:"name"`
	ContextLength      int      `json:"context_length"`
	SupportedProtocols []string `json:"supported_protocols"`
}

// acceptsChatProtocol：协议字段缺失视为标准 OpenAI 兼容；
// 存在时必须声明 chat-completions 能力。
func acceptsChatProtocol(protocols []string) bool {
	if len(protocols) == 0 {
		return true
	}
	for _, p := range protocols {
		switch strings.TrimSpace(p) {
		case "openai:chat-completions", "openai:chat":
			return true
		}
	}
	return false
}

// fetchAIModelList 请求 {base}/models。上游地址从不进入错误信息：
// 任何失败（网络、状态码、体积、格式）都只返回哨兵错误。
func fetchAIModelList(ctx context.Context, base, key string) ([]aiModelItem, error) {
	ctx, cancel := context.WithTimeout(ctx, aiModelsTimeout)
	defer cancel()
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, base+"/models", nil)
	if err != nil {
		return nil, errAIModelsUpstream
	}
	req.Header.Set("Accept", "application/json")
	req.Header.Set("User-Agent", "paper-manager")
	if key != "" {
		req.Header.Set("Authorization", "Bearer "+key)
	}
	// 禁止跟随重定向：3xx 一律按失败处理，避免 Key 被带到其他主机。
	client := &http.Client{
		CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse },
	}
	resp, err := client.Do(req)
	if err != nil {
		return nil, errAIModelsUpstream
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil, errAIModelsUpstream
	}
	data, err := io.ReadAll(io.LimitReader(resp.Body, aiModelsUpstreamLimit+1))
	if err != nil || len(data) > aiModelsUpstreamLimit {
		return nil, errAIModelsUpstream
	}
	return parseAIModelList(data)
}

func parseAIModelList(data []byte) ([]aiModelItem, error) {
	var envelope struct {
		Data   []aiModelEntry `json:"data"`
		Models []aiModelEntry `json:"models"`
	}
	if err := json.Unmarshal(data, &envelope); err != nil {
		var plain []aiModelEntry
		if err := json.Unmarshal(data, &plain); err != nil {
			return nil, errAIModelsUpstream
		}
		envelope.Data = plain
	}
	entries := envelope.Data
	if len(entries) == 0 {
		entries = envelope.Models
	}
	out := make([]aiModelItem, 0, len(entries))
	seen := make(map[string]bool, len(entries))
	for _, m := range entries {
		id := strings.TrimSpace(m.ID)
		if id == "" || seen[id] || !acceptsChatProtocol(m.SupportedProtocols) {
			continue
		}
		seen[id] = true
		name := strings.TrimSpace(m.Name)
		if name == "" {
			name = id
		}
		out = append(out, aiModelItem{ID: id, Name: name, ContextLength: m.ContextLength})
	}
	sort.Slice(out, func(i, j int) bool { return out[i].ID < out[j].ID })
	return out, nil
}
