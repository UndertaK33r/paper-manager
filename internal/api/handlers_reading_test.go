package api

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"paper-manager/internal/models"
)

// 标注接口：高亮（矩形）与文字批注（位置）的创建、校验、修改、删除
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

// 文字批注：任意位置新建、拖动改位置、编辑文字、删除
func TestNoteAnnotationEndpoints(t *testing.T) {
	s, st := newModelsTestServer(t)
	p := models.Paper{Title: "批注测试"}
	id, _ := st.CreatePaper(&p)
	base := "/api/papers/" + itoa(id) + "/annotations"
	h := s.Handler()

	// 非法位置
	for _, body := range []string{
		`{"kind":"note","page":1,"x":1.4,"y":0.2}`,
		`{"kind":"note","page":1,"x":-0.1,"y":0.2}`,
		`{"kind":"note","page":0,"x":0.1,"y":0.2}`,
	} {
		rec := httptest.NewRecorder()
		h.ServeHTTP(rec, jsonReq("POST", base, body))
		if rec.Code != http.StatusBadRequest {
			t.Fatalf("body %s: status %d, want 400", body, rec.Code)
		}
	}

	// 新建（初始可以为空文字，用户随后输入）
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, jsonReq("POST", base, `{"kind":"note","page":4,"x":0.3,"y":0.6,"color":"blue"}`))
	if rec.Code != http.StatusCreated {
		t.Fatalf("create status %d: %s", rec.Code, rec.Body.String())
	}
	var note models.Annotation
	json.Unmarshal(rec.Body.Bytes(), &note)
	if note.Kind != models.AnnoKindNote || note.Page != 4 || note.X != 0.3 || note.Y != 0.6 {
		t.Fatalf("创建结果异常: %+v", note)
	}

	// 编辑文字 + 拖动位置
	rec = httptest.NewRecorder()
	h.ServeHTTP(rec, jsonReq("PATCH", "/api/annotations/"+itoa(note.ID), `{"note":" 这里的方法值得复现 ","x":0.55,"y":0.22}`))
	if rec.Code != http.StatusOK {
		t.Fatalf("patch status %d: %s", rec.Code, rec.Body.String())
	}
	list, _ := st.ListAnnotations(id)
	if list[0].Note != "这里的方法值得复现" || list[0].X != 0.55 || list[0].Y != 0.22 {
		t.Fatalf("修改未生效: %+v", list[0])
	}

	// x/y 必须成对提供
	rec = httptest.NewRecorder()
	h.ServeHTTP(rec, jsonReq("PATCH", "/api/annotations/"+itoa(note.ID), `{"x":0.2}`))
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("只给 x want 400, got %d", rec.Code)
	}

	// 删除
	rec = httptest.NewRecorder()
	h.ServeHTTP(rec, httptest.NewRequest("DELETE", "/api/annotations/"+itoa(note.ID), nil))
	if rec.Code != http.StatusOK {
		t.Fatalf("delete status %d", rec.Code)
	}
	if n, _ := st.CountAnnotations(id); n != 0 {
		t.Fatalf("删除后仍有 %d 条", n)
	}
}
