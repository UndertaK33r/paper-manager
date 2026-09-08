package api

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
	"time"

	"paper-manager/internal/store"
)

func newModelsTestServer(t *testing.T) (*Server, *store.Store) {
	t.Helper()
	st, err := store.Open(t.TempDir() + "/test.db")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { st.Close() })
	return New(Config{Store: st}), st
}

func doModelsRequest(t *testing.T, s *Server, method, target, body string) (*httptest.ResponseRecorder, map[string]any) {
	t.Helper()
	var req *http.Request
	if body != "" {
		req = httptest.NewRequest(method, target, strings.NewReader(body))
	} else {
		req = httptest.NewRequest(method, target, nil)
	}
	rec := httptest.NewRecorder()
	s.handleAIModels(rec, req)
	var out map[string]any
	if err := json.Unmarshal(rec.Body.Bytes(), &out); err != nil {
		t.Fatalf("response not JSON: %v (%q)", err, rec.Body.String())
	}
	return rec, out
}

// 认证拉取：配置地址 + 存储Key → GET 不带参数时 Key 只发给一致地址。
func TestAIModelsSendsStoredKeyOnlyToMatchingBase(t *testing.T) {
	var gotAuth []string
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotAuth = append(gotAuth, r.Header.Get("Authorization"))
		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte(`{"data":[{"id":"m1"},{"id":"m2"}]}`))
	}))
	defer upstream.Close()

	s, st := newModelsTestServer(t)
	st.SetSetting("ai_base_url", upstream.URL)
	st.SetSetting("ai_api_key", "sk-secret")

	rec, out := doModelsRequest(t, s, http.MethodGet, "/api/ai/models", "")
	if rec.Code != http.StatusOK {
		t.Fatalf("status %d", rec.Code)
	}
	if out["source"] != "upstream" {
		t.Fatalf("source = %v, want upstream", out["source"])
	}
	models, _ := out["models"].([]any)
	if len(models) != 2 {
		t.Fatalf("want 2 models, got %d", len(models))
	}
	if len(gotAuth) != 1 || gotAuth[0] != "Bearer sk-secret" {
		t.Fatalf("auth header wrong: %v", gotAuth)
	}

	// 不同地址：不带存储 Key
	gotAuth = nil
	other := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotAuth = append(gotAuth, r.Header.Get("Authorization"))
		w.Write([]byte(`{"data":[{"id":"x"}]}`))
	}))
	defer other.Close()

	_, out = doModelsRequest(t, s, http.MethodGet, "/api/ai/models?base="+other.URL, "")
	if out["source"] != "upstream" {
		t.Fatalf("cross-base source = %v", out["source"])
	}
	if len(gotAuth) != 1 || gotAuth[0] != "" {
		t.Fatalf("cross-base must not leak stored key, got %v", gotAuth)
	}
}

// POST {base?, apiKey?}：显式匿名 / 显式 Key / 未提供时复用配置 Key。
func TestAIModelsPostKeyPolicy(t *testing.T) {
	var gotAuth []string
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotAuth = append(gotAuth, r.Header.Get("Authorization"))
		w.Write([]byte(`{"data":[{"id":"m1"}]}`))
	}))
	defer upstream.Close()

	s, st := newModelsTestServer(t)
	st.SetSetting("ai_base_url", upstream.URL)
	st.SetSetting("ai_api_key", "sk-stored")

	// apiKey 未提供 → 复用存储 Key
	gotAuth = nil
	doModelsRequest(t, s, http.MethodPost, "/api/ai/models", `{}`)
	if len(gotAuth) != 1 || gotAuth[0] != "Bearer sk-stored" {
		t.Fatalf("no apiKey should reuse stored key, got %v", gotAuth)
	}

	// apiKey:"" → 显式匿名
	gotAuth = nil
	doModelsRequest(t, s, http.MethodPost, "/api/ai/models", `{"apiKey":""}`)
	if len(gotAuth) != 1 || gotAuth[0] != "" {
		t.Fatalf("empty apiKey should be anonymous, got %v", gotAuth)
	}

	// apiKey 显式提供 → 仅本次使用
	gotAuth = nil
	doModelsRequest(t, s, http.MethodPost, "/api/ai/models", `{"apiKey":"sk-fresh"}`)
	if len(gotAuth) != 1 || gotAuth[0] != "Bearer sk-fresh" {
		t.Fatalf("explicit apiKey missing, got %v", gotAuth)
	}

	// base 与配置不同且未提供 apiKey → 不带 Key
	other := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotAuth = append(gotAuth, r.Header.Get("Authorization"))
		w.Write([]byte(`{"data":[{"id":"x"}]}`))
	}))
	defer other.Close()
	gotAuth = nil
	doModelsRequest(t, s, http.MethodPost, "/api/ai/models", `{"base":"`+other.URL+`"}`)
	if len(gotAuth) != 1 || gotAuth[0] != "" {
		t.Fatalf("cross-base POST must not carry stored key, got %v", gotAuth)
	}
}

// 兼容标准 OpenAI（仅 id）、协议筛选、按 id 去重排序。
func TestAIModelsParseFilterDedupeSort(t *testing.T) {
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte(`{"data":[
			{"id":"zeta","name":"Z","context_length":8192},
			{"id":"alpha"},
			{"id":"zeta","name":"dup"},
			{"id":"embed-only","supported_protocols":["openai:embeddings"]},
			{"id":"chat2","supported_protocols":["openai:chat-completions","openai:embeddings"]},
			{"id":"","name":"broken"}
		]}`))
	}))
	defer upstream.Close()

	s, st := newModelsTestServer(t)
	st.SetSetting("ai_base_url", upstream.URL)
	_, out := doModelsRequest(t, s, http.MethodGet, "/api/ai/models", "")
	if out["source"] != "upstream" {
		t.Fatalf("source = %v", out["source"])
	}
	raw, _ := json.Marshal(out["models"])
	var models []aiModelItem
	if err := json.Unmarshal(raw, &models); err != nil {
		t.Fatal(err)
	}
	if len(models) != 3 {
		t.Fatalf("want 3 models (dedupe+filter), got %d: %s", len(models), raw)
	}
	if models[0].ID != "alpha" || models[0].Name != "alpha" {
		t.Fatalf("id-only entry should fall back name=id: %+v", models[0])
	}
	if models[1].ID != "chat2" || models[2].ID != "zeta" || models[2].Name != "Z" {
		t.Fatalf("sort/dedupe wrong: %+v", models)
	}

	// 裸数组响应（部分网关不包 envelope）
	bare := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte(`[{"id":"only"}]`))
	}))
	defer bare.Close()
	st.SetSetting("ai_base_url", bare.URL)
	_, out = doModelsRequest(t, s, http.MethodGet, "/api/ai/models", "")
	raw, _ = json.Marshal(out["models"])
	if !strings.Contains(string(raw), `"only"`) {
		t.Fatalf("bare array not accepted: %s", raw)
	}
}

// 上游各种失败都返回 source=unavailable + 空列表，HTTP 200，且错误信息不含 URL。
func TestAIModelsUpstreamFailureIsUnavailable(t *testing.T) {
	cases := []struct {
		name    string
		handler http.HandlerFunc
	}{
		{"401", func(w http.ResponseWriter, r *http.Request) { w.WriteHeader(http.StatusUnauthorized) }},
		{"500", func(w http.ResponseWriter, r *http.Request) { w.WriteHeader(http.StatusInternalServerError) }},
		{"badjson", func(w http.ResponseWriter, r *http.Request) { w.Write([]byte(`<html>oops</html>`)) }},
		{"redirect", func(w http.ResponseWriter, r *http.Request) {
			http.Redirect(w, r, "http://evil.example.com/steal", http.StatusFound)
		}},
		{"toobig", func(w http.ResponseWriter, r *http.Request) {
			w.Write([]byte(`{"data":["` + strings.Repeat("x", 2<<20) + `"]}`))
		}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			redirectHits := 0
			h := tc.handler
			if tc.name == "redirect" {
				evil := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
					redirectHits++
				}))
				defer evil.Close()
				h = func(w http.ResponseWriter, r *http.Request) {
					http.Redirect(w, r, evil.URL+"/steal", http.StatusFound)
				}
			}
			upstream := httptest.NewServer(h)
			defer upstream.Close()

			s, st := newModelsTestServer(t)
			st.SetSetting("ai_base_url", upstream.URL)
			st.SetSetting("ai_api_key", "sk-secret")

			rec, out := doModelsRequest(t, s, http.MethodGet, "/api/ai/models", "")
			if rec.Code != http.StatusOK {
				t.Fatalf("status %d, want 200", rec.Code)
			}
			if out["source"] != "unavailable" {
				t.Fatalf("source = %v, want unavailable", out["source"])
			}
			models, _ := out["models"].([]any)
			if len(models) != 0 {
				t.Fatalf("models should be empty, got %d", len(models))
			}
			if s := rec.Body.String(); strings.Contains(s, upstream.URL) || strings.Contains(s, "sk-secret") {
				t.Fatalf("response leaks URL or key: %s", s)
			}
			if tc.name == "redirect" && redirectHits != 0 {
				t.Fatal("redirect target was contacted")
			}
		})
	}
}

// 客户端取消后应立即返回，不等待上游。
func TestAIModelsCancelReturnsImmediately(t *testing.T) {
	release := make(chan struct{})
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		<-release
	}))
	defer upstream.Close()
	defer close(release)

	s, st := newModelsTestServer(t)
	st.SetSetting("ai_base_url", upstream.URL)

	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	req := httptest.NewRequest(http.MethodGet, "/api/ai/models", nil).WithContext(ctx)
	rec := httptest.NewRecorder()

	start := time.Now()
	s.handleAIModels(rec, req)
	if elapsed := time.Since(start); elapsed > time.Second {
		t.Fatalf("cancel did not return promptly: %v", elapsed)
	}
	if !strings.Contains(rec.Body.String(), "unavailable") {
		t.Fatalf("expected unavailable, got %s", rec.Body.String())
	}
}

// 地址校验表：显式传入的非法地址一律 400；合法地址进入拉取流程（本地拒连端口，秒回 200/unavailable）。
func TestAIModelsRejectsInvalidBase(t *testing.T) {
	s, _ := newModelsTestServer(t)
	for _, base := range []string{
		"ftp://x.com/v1", "not a url", "/only/path", "http://", "javascript:alert(1)",
		"http://user:pass@x.com/v1", "http://x.com/v1?x=1", "http://x.com/v1#frag",
	} {
		rec, _ := doModelsRequest(t, s, http.MethodGet, "/api/ai/models?base="+url.QueryEscape(base), "")
		if rec.Code != http.StatusBadRequest {
			t.Errorf("base %q: status %d, want 400", base, rec.Code)
		}
	}
	for _, base := range []string{"http://127.0.0.1:9/v1/", "http://127.0.0.1:9/v1"} {
		rec, out := doModelsRequest(t, s, http.MethodGet, "/api/ai/models?base="+url.QueryEscape(base), "")
		if rec.Code != http.StatusOK {
			t.Errorf("base %q: status %d, want 200", base, rec.Code)
		}
		if out["source"] != "unavailable" {
			t.Errorf("base %q: source %v, want unavailable", base, out["source"])
		}
	}
}

// POST 请求体限制与格式校验。
func TestAIModelsRequestBodyGuard(t *testing.T) {
	s, _ := newModelsTestServer(t)
	rec, _ := doModelsRequest(t, s, http.MethodPost, "/api/ai/models", `{not json}`)
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("bad json: status %d, want 400", rec.Code)
	}
	big := `{"base":"` + strings.Repeat("a", aiModelsBodyLimit+1) + `"}`
	rec, _ = doModelsRequest(t, s, http.MethodPost, "/api/ai/models", big)
	if rec.Code != http.StatusRequestEntityTooLarge {
		t.Fatalf("oversize: status %d, want 413", rec.Code)
	}
}

// 默认值优先级：设置 > 环境变量 > 内置默认。
func TestAIModelsCredentialPrecedence(t *testing.T) {
	s, st := newModelsTestServer(t)
	t.Setenv("AI_BASE_URL", "http://env.example.com/v1")
	t.Setenv("AI_API_KEY", "env-key")

	base, key := s.aiModelsCredentials()
	if base != "http://env.example.com/v1" || key != "env-key" {
		t.Fatalf("env fallback wrong: %q %q", base, key)
	}

	st.SetSetting("ai_base_url", "http://setting.example.com/v1")
	st.SetSetting("ai_api_key", "setting-key")
	base, key = s.aiModelsCredentials()
	if base != "http://setting.example.com/v1" || key != "setting-key" {
		t.Fatalf("setting should win over env: %q %q", base, key)
	}

	st.DeleteSetting("ai_base_url")
	st.DeleteSetting("ai_api_key")
	t.Setenv("AI_BASE_URL", "")
	t.Setenv("AI_API_KEY", "")
	base, key = s.aiModelsCredentials()
	if base != aiModelsDefaultBase {
		t.Fatalf("default base wrong: %q", base)
	}
	if key != "" {
		t.Fatalf("default should have no key: %q", key)
	}
}
