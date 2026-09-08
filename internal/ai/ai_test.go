package ai

import (
	"encoding/json"
	"testing"
)

func jsonUnmarshal(data []byte, v any) error { return json.Unmarshal(data, v) }

func TestUnmarshalFlexTypes(t *testing.T) {
	// year 数字、authors 数组、keywords 数组、null 字段——LLM 输出的常见抖动
	data := []byte(`{
		"title": "Some Paper",
		"authors": ["Ran Zhang", "Xuanhua He"],
		"year": 2025,
		"venue": "arXiv",
		"doi": null,
		"keywords": ["Image Fusion", 123, "LLM"],
		"summary": "一句话",
		"category": "深度学习"
	}`)
	m, err := parseTest(data)
	if err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if m.Year != "2025" {
		t.Fatalf("year = %q want 2025", m.Year)
	}
	if m.Authors != "Ran Zhang, Xuanhua He" {
		t.Fatalf("authors = %q", m.Authors)
	}
	if m.DOI != "" {
		t.Fatalf("doi = %q want empty", m.DOI)
	}
	if m.Keywords != "Image Fusion, 123, LLM" {
		t.Fatalf("keywords = %q", m.Keywords)
	}
}

func TestUnmarshalNormalStrings(t *testing.T) {
	m, err := parseTest([]byte(`{"title":"A","authors":"张三, 李四","year":"2024"}`))
	if err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if m.Year != "2024" || m.Authors != "张三, 李四" {
		t.Fatalf("got %q / %q", m.Year, m.Authors)
	}
}

func TestExtractJSONObject(t *testing.T) {
	cases := []struct {
		in, want string
	}{
		{`{"a":1}`, `{"a":1}`},
		{"好的，结果如下：\n```json\n{\"a\": \"含}花括号\"}\n```", `{"a": "含}花括号"}`},
		{"前置说明 {\"a\":1} 后置文字", `{"a":1}`},
		{`{"a":{"b":2},"c":3}`, `{"a":{"b":2},"c":3}`},
		{"没有对象", ""},
	}
	for _, c := range cases {
		got := extractJSONObject(c.in)
		if got != c.want {
			t.Errorf("extractJSONObject(%q) = %q want %q", c.in, got, c.want)
		}
	}
}

func TestRepairTruncatedJSON(t *testing.T) {
	cases := []struct {
		name string
		in   string
	}{
		{"字符串值截断", `{"title": "A", "year": 2025, "keywords": "Image Fusion, Multi-modali`},
		{"数组值截断", `{"title": "X", "authors": ["A", "B`},
		{"键名截断", `{"title": "X", "key`},
		{"嵌套对象完整后截断", `{"meta": {"a": 1}, "title": "T`,},
	}
	for _, c := range cases {
		repaired := repairTruncatedJSON(c.in)
		if repaired == "" {
			t.Errorf("%s: repair 返回空", c.name)
			continue
		}
		var m MetaResult
		if err := json.Unmarshal([]byte(repaired), &m); err != nil {
			t.Errorf("%s: 修复后仍非法: %v → %q", c.name, err, repaired)
		}
	}
	// 用户实际报错场景：修复后应保住 title 和 year
	m := MetaResult{}
	repaired := repairTruncatedJSON(`{"title": "Distilling Textual Priors", "authors": "Ran Zhang, Xuanhua He", "year": 2025, "keywords": "Image Fusion, Knowledge Distillation, Multi-modali`)
	if err := json.Unmarshal([]byte(repaired), &m); err != nil {
		t.Fatalf("修复失败: %v → %q", err, repaired)
	}
	if m.Title != "Distilling Textual Priors" || m.Year != "2025" || m.Authors != "Ran Zhang, Xuanhua He" {
		t.Fatalf("字段保全不对: %+v", m)
	}
	// 完整对象：无需修复
	if r := repairTruncatedJSON(`{"a":1}`); r != "" {
		t.Fatalf("完整对象不应修复, got %q", r)
	}
}

func parseTest(data []byte) (MetaResult, error) {
	var m MetaResult
	err := jsonUnmarshal(data, &m)
	return m, err
}
