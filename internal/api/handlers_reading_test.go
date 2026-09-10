package api

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"paper-manager/internal/models"
)

// 阅读模式接口：返回分页/段落结构，无全文时 hasText=false
func TestPaperTextEndpoint(t *testing.T) {
	s, st := newModelsTestServer(t)
	p := models.Paper{Title: "带全文的论文"}
	id, err := st.CreatePaper(&p)
	if err != nil {
		t.Fatal(err)
	}
	if err := st.UpdateFields(id, map[string]any{"fulltext": "1\nFirst paragraph line one\nline two continues\n\n2\n第二页内容"}, nil, nil, nil); err != nil {
		t.Fatal(err)
	}

	rec := httptest.NewRecorder()
	s.Handler().ServeHTTP(rec, httptest.NewRequest("GET", "/api/papers/"+itoa(id)+"/text", nil))
	if rec.Code != http.StatusOK {
		t.Fatalf("status %d: %s", rec.Code, rec.Body.String())
	}
	var resp struct {
		HasText bool `json:"hasText"`
		Pages   []struct {
			Number     int `json:"number"`
			Paragraphs []struct {
				Heading  bool `json:"heading"`
				Segments []struct {
					Start int    `json:"start"`
					Text  string `json:"text"`
					Join  string `json:"join"`
				} `json:"segments"`
			} `json:"paragraphs"`
		} `json:"pages"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
		t.Fatal(err)
	}
	if !resp.HasText || len(resp.Pages) != 2 {
		t.Fatalf("结构异常: %+v", resp)
	}
	first := resp.Pages[0].Paragraphs[0].Segments
	if len(first) != 2 || first[1].Join != "space" {
		t.Fatalf("断行未合并或 Join 缺失: %+v", first)
	}

	// 空全文的论文
	empty := models.Paper{Title: "无全文"}
	eid, _ := st.CreatePaper(&empty)
	rec2 := httptest.NewRecorder()
	s.Handler().ServeHTTP(rec2, httptest.NewRequest("GET", "/api/papers/"+itoa(eid)+"/text", nil))
	var resp2 struct {
		HasText bool `json:"hasText"`
	}
	json.Unmarshal(rec2.Body.Bytes(), &resp2)
	if resp2.HasText {
		t.Fatal("无全文时 hasText 应为 false")
	}
}

// 标注接口：创建/校验/列表/修改/删除（位置用页码 + 归一化矩形）
func TestAnnotationEndpoints(t *testing.T) {
	s, st := newModelsTestServer(t)
	p := models.Paper{Title: "标注测试"}
	id, err := st.CreatePaper(&p)
	if err != nil {
		t.Fatal(err)
	}
	base := "/api/papers/" + itoa(id) + "/annotations"
	h := s.Handler()

	// 非法入参
	for _, body := range []string{
		`{"page":1,"rects":[],"quote":"x"}`,                              // 没有矩形
		`{"page":1,"rects":[{"p":0,"x":0.1,"y":0.1,"w":0.2,"h":0.02}]}`,  // 页码非法
		`{"page":1,"rects":[{"p":1,"x":0.1,"y":0.1,"w":0,"h":0.02}]}`,    // 宽度为 0
		`{"page":1,"rects":[{"p":1,"x":0.9,"y":0.1,"w":0.5,"h":0.02}]}`,  // 超出页面
		`{"page":1,"rects":[{"p":1,"x":-0.1,"y":0.1,"w":0.2,"h":0.02}]}`, // 负坐标
	} {
		rec := httptest.NewRecorder()
		h.ServeHTTP(rec, jsonReq("POST", base, body))
		if rec.Code != http.StatusBadRequest {
			t.Fatalf("body %s: status %d, want 400", body, rec.Code)
		}
	}

	// 正常创建（跨行两个矩形）
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, jsonReq("POST", base, `{"page":3,"rects":[{"p":3,"x":0.11,"y":0.2,"w":0.3,"h":0.02},{"p":3,"x":0.11,"y":0.23,"w":0.25,"h":0.02}],"quote":"选中文字","color":"green","note":" 我的备注 "}`))
	if rec.Code != http.StatusCreated {
		t.Fatalf("create status %d: %s", rec.Code, rec.Body.String())
	}
	var created models.Annotation
	json.Unmarshal(rec.Body.Bytes(), &created)
	if created.ID == 0 || created.Color != "green" || created.Note != "我的备注" || created.Page != 3 {
		t.Fatalf("创建结果异常: %+v", created)
	}
	if len(created.Rects) != 2 {
		t.Fatalf("矩形数量不对: %+v", created.Rects)
	}

	// 列表
	rec = httptest.NewRecorder()
	h.ServeHTTP(rec, httptest.NewRequest("GET", base, nil))
	var list struct {
		Total       int                 `json:"total"`
		Annotations []models.Annotation `json:"annotations"`
	}
	json.Unmarshal(rec.Body.Bytes(), &list)
	if list.Total != 1 || list.Annotations[0].Quote != "选中文字" || list.Annotations[0].Page != 3 {
		t.Fatalf("列表异常: %+v", list)
	}

	// 修改颜色与备注
	rec = httptest.NewRecorder()
	h.ServeHTTP(rec, jsonReq("PATCH", "/api/annotations/"+itoa(created.ID), `{"color":"blue","note":"补充"}`))
	if rec.Code != http.StatusOK {
		t.Fatalf("patch status %d", rec.Code)
	}
	after, _ := st.ListAnnotations(id)
	if after[0].Color != "blue" || after[0].Note != "补充" {
		t.Fatalf("修改未生效: %+v", after[0])
	}

	// 删除
	rec = httptest.NewRecorder()
	h.ServeHTTP(rec, httptest.NewRequest("DELETE", "/api/annotations/"+itoa(created.ID), nil))
	if rec.Code != http.StatusOK {
		t.Fatalf("delete status %d", rec.Code)
	}
	rec = httptest.NewRecorder()
	h.ServeHTTP(rec, httptest.NewRequest("DELETE", "/api/annotations/"+itoa(created.ID), nil))
	if rec.Code != http.StatusNotFound {
		t.Fatalf("重复删除 want 404, got %d", rec.Code)
	}

	// 论文不存在时创建应 404
	rec = httptest.NewRecorder()
	h.ServeHTTP(rec, jsonReq("POST", "/api/papers/99999/annotations", `{"page":1,"rects":[{"p":1,"x":0.1,"y":0.1,"w":0.2,"h":0.02}]}`))
	if rec.Code != http.StatusNotFound {
		t.Fatalf("不存在的论文 want 404, got %d", rec.Code)
	}
}

func jsonReq(method, path, body string) *http.Request {
	r := httptest.NewRequest(method, path, strings.NewReader(body))
	r.Header.Set("Content-Type", "application/json")
	return r
}

func itoa(n int64) string {
	if n == 0 {
		return "0"
	}
	digits := []byte{}
	for n > 0 {
		digits = append([]byte{byte('0' + n%10)}, digits...)
		n /= 10
	}
	return string(digits)
}
