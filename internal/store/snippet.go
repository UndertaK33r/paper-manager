package store

import (
	"strings"
	"unicode"
)

// queryTokens 把查询拆成 token：中文逐字、拉丁词整词。
func queryTokens(q string) []string {
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
		if unicode.IsLetter(r) || unicode.IsNumber(r) {
			if r > 0x2e80 {
				flush()
				out = append(out, string(r))
			} else {
				cur.WriteRune(r)
			}
		} else {
			flush()
		}
	}
	flush()
	return out
}

// buildSnippet 在正文中找第一个命中 token 的窗口，返回带省略号的片段。
func buildSnippet(text, query string, size int) string {
	tokens := queryTokens(query)
	if len(tokens) == 0 || text == "" {
		return ""
	}
	low := strings.ToLower(text)
	best := -1
	for _, t := range tokens {
		if i := strings.Index(low, t); i >= 0 && (best < 0 || i < best) {
			best = i
		}
	}
	if best < 0 {
		return ""
	}
	if size <= 0 {
		size = 160
	}
	start := best - size/2
	if start < 0 {
		start = 0
	}
	end := start + size
	if end > len(text) {
		end = len(text)
	}
	s := text[start:end]
	if start > 0 {
		s = "…" + s
	}
	if end < len(text) {
		s = s + "…"
	}
	return strings.TrimSpace(s)
}
