package pdfmeta

import (
	"sort"
	"strings"
	"unicode"
	"unicode/utf16"
)

// 阅读模式的结构化文本：把 PDF 抽出的纯文本重排成「页 → 段落 → 片段」。
//
// 偏移量单位是 UTF-16 码元（与浏览器 JS 的字符串下标一致），这样前端选区算出的
// 偏移可以直接存库、比较，不需要再做编码转换。
// 每个 Segment 对应原文里一段连续字符；段内片段之间的连接方式由 Join 决定
// （原文那里是换行，没有对应字符，因此不参与选区映射）。
type ReadingSegment struct {
	Start int    `json:"start"`
	Text  string `json:"text"`
	// Join 表示与「前一个片段」的连接方式："" / "space" 补一个空格，
	// "none" 直接相接（CJK 换行、英文断词、首字母下沉）
	Join string `json:"join,omitempty"`
}

type ReadingParagraph struct {
	Heading  bool             `json:"heading,omitempty"`
	Segments []ReadingSegment `json:"segments"`
	// Furniture 标记页眉页脚/版权行（跨页重复出现），前端弱化显示
	Furniture bool `json:"furniture,omitempty"`
}

type ReadingPage struct {
	Number     int                `json:"number"`
	Paragraphs []ReadingParagraph `json:"paragraphs"`
}

type srcLine struct {
	start   int
	text    string
	pageNum bool
}

// 已知的小节标题词：这些行单独成段并标记为标题
var sectionWords = map[string]bool{
	"abstract": true, "introduction": true, "related work": true, "background": true,
	"method": true, "methods": true, "methodology": true, "approach": true,
	"experiments": true, "experimental setup": true, "results": true, "discussion": true,
	"conclusion": true, "conclusions": true, "future work": true, "references": true,
	"acknowledgment": true, "acknowledgments": true, "index terms": true, "appendix": true,
}

// hyphenSuffixFragments 是「只出现在词尾、不会独立成词」的常见后缀片段。
// 行尾连字符后面以这些片段开头，基本可断定是排版断词（computa-tional → computational）；
// 否则视为本来就带连字符的复合词（visible-light），保留连字符。
var hyphenSuffixFragments = []string{
	"tion", "sion", "tional", "sional", "ment", "ments", "ingly", "ing", "ings",
	"ally", "ially", "ity", "ities", "ous", "ious", "eous", "ive", "sive", "tive",
	"ance", "ence", "able", "ible", "ably", "ibly", "cial", "tial",
	"cient", "ciency", "mance", "ture", "tures", "ular", "ularly", "ural",
	"graph", "graphy", "logy", "nomy", "metry", "lysis",
}

// SplitReading 把抽取出的全文重排为阅读视图结构。
// 输入是内嵌的纯文本（页码单独成行、正文按 PDF 列宽硬换行）。
func SplitReading(text string) []ReadingPage {
	if strings.TrimSpace(text) == "" {
		return nil
	}
	raw := strings.Split(strings.ReplaceAll(text, "\r\n", "\n"), "\n")
	items := make([]srcLine, 0, len(raw))
	offset := 0
	for _, line := range raw {
		lead := utf16Len(line) - utf16Len(strings.TrimLeft(line, " \t"))
		trimmed := strings.TrimSpace(line)
		items = append(items, srcLine{start: offset + lead, text: trimmed, pageNum: isPageNumber(trimmed)})
		offset += utf16Len(line) + 1 // +1：换行符
	}
	typical := typicalLineLen(items)
	furniture := repeatedLines(items)

	pages := []ReadingPage{}
	page := ReadingPage{Number: 1, Paragraphs: []ReadingParagraph{}}
	para := ReadingParagraph{Segments: []ReadingSegment{}}
	prevText := ""
	prevEndsPara := true
	prevHeadingCaps := false // 上一行是「全大写标题」，其续行应合并

	flushPara := func() {
		if len(para.Segments) == 0 {
			return
		}
		// 合并后再判一次标题：'I. I' + 'NTRODUCTION' 合成 'I. INTRODUCTION' 才算标题
		if !para.Heading && !para.Furniture && looksLikeHeading(segmentsText(para.Segments)) {
			para.Heading = true
		}
		page.Paragraphs = append(page.Paragraphs, para)
		para = ReadingParagraph{Segments: []ReadingSegment{}}
	}
	flushPage := func() {
		flushPara()
		if len(page.Paragraphs) > 0 {
			pages = append(pages, page)
		}
		page = ReadingPage{Number: page.Number + 1, Paragraphs: []ReadingParagraph{}}
	}

	for _, ln := range items {
		if ln.pageNum { // 页码行 = 分页
			flushPage()
			page.Number = atoiSafe(ln.text)
			prevText, prevEndsPara = "", true
			continue
		}
		if ln.text == "" { // 空行 = 段落分隔
			flushPara()
			prevText, prevEndsPara = "", true
			continue
		}

		heading := looksLikeHeading(ln.text)
		// 首字母下沉的续行（'I. I' + 'NTRODUCTION'）必须与上一行同段，
		// 即便它自己看起来像标题（全大写）
		dropCapCont := false
		var prev *ReadingSegment
		if len(para.Segments) > 0 {
			prev = &para.Segments[len(para.Segments)-1]
			dropCapCont = endsWithDropCap(prev.Text) && startsUpper(ln.text)
		}
		// 全大写标题换行（'II. RELATED' + 'WORKS'）两行都是全大写时合并
		capsHeadingCont := prevHeadingCaps && isAllCaps(ln.text)
		// 孤立的单字母行（'R'、'I. I'）是首字母下沉的**开头**，必须另起一段：
		// 它后面那行才是同一个词的剩余部分
		dropCapStart := isDropCapStart(ln.text)
		if !dropCapCont && !capsHeadingCont && (dropCapStart || prevEndsPara || heading || looksLikeHeading(prevText)) {
			flushPara()
			prev = nil
		}

		seg := ReadingSegment{Start: ln.start, Text: ln.text}
		if prev != nil {
			hyphenBreak := strings.HasSuffix(prev.Text, "-") && utf16Len(prev.Text) > 2 && startsLower(ln.text)
			switch {
			case hyphenBreak && looksLikeSuffixFragment(ln.text):
				prev.Text = strings.TrimSuffix(prev.Text, "-") // 排版断词：去掉连字符
				seg.Join = "none"
			case hyphenBreak:
				seg.Join = "none" // 复合词：保留连字符，同样不插空格
			case dropCapCont:
				seg.Join = "none"
			case bothCJK(prev.Text, ln.text):
				seg.Join = "none"
			default:
				seg.Join = "space"
			}
		}
		para.Segments = append(para.Segments, seg)
		if heading {
			para.Heading = true
		}
		if furniture[normalizeLine(ln.text)] {
			para.Furniture = true
			para.Heading = false // 页眉页脚不是小节标题
		}
		prevText = ln.text
		prevHeadingCaps = heading && isAllCaps(ln.text)
		// 短行通常是段末；但以下两种情况后面还要接着合并：
		//   · 首字母下沉的短行（单独一行 'I'）
		//   · 行尾带连字符的断词行（'computa-'）
		bindNext := endsWithDropCap(ln.text) || strings.HasSuffix(ln.text, "-")
		prevEndsPara = heading || (utf16Len(ln.text) < typical*3/4 && !bindNext)
	}
	flushPage()
	return pages
}

// segmentsText 按 Join 语义把段落片段拼回文本（与前端渲染规则一致）
func segmentsText(segs []ReadingSegment) string {
	var b strings.Builder
	for i, seg := range segs {
		if i > 0 && seg.Join != "none" {
			b.WriteString(" ")
		}
		b.WriteString(seg.Text)
	}
	return b.String()
}

// isDropCapStart 判断这一行是否是首字母下沉的「开头」：
// 形如单独的 'R'、'I'，或 'I. I'（罗马数字小节号 + 首字母）。
func isDropCapStart(s string) bool {
	fields := strings.Fields(s)
	switch len(fields) {
	case 1:
		r := []rune(fields[0])
		return len(r) == 1 && unicode.IsLetter(r[0])
	case 2:
		second := []rune(fields[1])
		return len(second) == 1 && unicode.IsLetter(second[0]) &&
			len(fields[0]) <= 4 && strings.HasSuffix(fields[0], ".")
	}
	return false
}

// endsWithDropCap 判断一行是否以「单个字母」结尾（PDF 首字母下沉的痕迹）：
// 常见形态是 "I. I" 或单独一行 "I"、"R"。
func endsWithDropCap(s string) bool {
	fields := strings.Fields(s)
	if len(fields) == 0 {
		return false
	}
	last := []rune(fields[len(fields)-1])
	return len(last) == 1 && unicode.IsLetter(last[0])
}

func startsUpper(s string) bool {
	for _, r := range s {
		return unicode.IsUpper(r)
	}
	return false
}

// isAllCaps 判断是否是「全大写且至少两个字母」的行（用于识别换行的标题）
func isAllCaps(s string) bool {
	letters, upper := 0, 0
	for _, r := range s {
		if unicode.IsLetter(r) {
			letters++
			if unicode.IsUpper(r) {
				upper++
			}
		}
	}
	return letters >= 2 && letters == upper
}

func startsLower(s string) bool {
	for _, r := range s {
		return unicode.IsLower(r)
	}
	return false
}

func looksLikeSuffixFragment(s string) bool {
	lower := strings.ToLower(s)
	for _, frag := range hyphenSuffixFragments {
		if strings.HasPrefix(lower, frag) {
			return true
		}
	}
	return false
}

// bothCJK 上一行末尾与下一行开头都是中日韩字符时不插空格。
func bothCJK(prev, cur string) bool {
	p, c := []rune(prev), []rune(cur)
	if len(p) == 0 || len(c) == 0 {
		return false
	}
	return isCJK(p[len(p)-1]) && isCJK(c[0])
}

func isPageNumber(s string) bool {
	if s == "" || utf16Len(s) > 4 {
		return false
	}
	for _, r := range s {
		if !unicode.IsDigit(r) {
			return false
		}
	}
	return true
}

// captionWords：图表/算法标题的开头词，这类行不是小节标题
var captionWords = map[string]bool{
	"table": true, "tab": true, "fig": true, "figure": true, "figs": true,
	"algorithm": true, "alg": true, "eq": true, "equation": true, "listing": true,
}

func looksLikeHeading(s string) bool {
	if s == "" {
		return false
	}
	lower := strings.ToLower(strings.TrimSpace(s))
	if sectionWords[strings.TrimRight(lower, ". ")] {
		return true
	}
	// 含逗号/分号的行基本是作者行、表格行、图注，不是小节标题
	if strings.ContainsAny(lower, ",;") {
		return false
	}
	if fields := strings.Fields(lower); len(fields) > 0 {
		if captionWords[strings.Trim(fields[0], ".:")] {
			return false
		}
	}
	// 「编号 + 大写开头的词」：1. Introduction / 2.1. Method / III. METHOD
	// 编号必须短（≤6 字符）且后一个词以大写字母开头，否则会把表格里的
	// 数字行（"14146 / 6"、"1.91 0.61"）误判成标题。
	if fields := strings.Fields(s); len(fields) >= 2 && utf16Len(s) <= 70 {
		if isSectionNumber(fields[0]) && startsUpper(fields[1]) {
			return true
		}
	}
	return isAllCapsHeading(s)
}

// isSectionNumber 判断是否是章节编号：1 / 12 / 2.1 / 3.4.1 / I. / III.
func isSectionNumber(f string) bool {
	if utf16Len(f) > 6 {
		return false
	}
	body := strings.TrimSuffix(f, ".")
	if body == "" {
		return false
	}
	digits, roman, dots := 0, 0, 0
	for _, r := range body {
		switch {
		case unicode.IsDigit(r):
			digits++
		case r == '.':
			dots++
		case r == 'I' || r == 'V' || r == 'X' || r == 'L' || r == 'C':
			roman++
		default:
			return false
		}
	}
	if digits == 0 && roman == 0 {
		return false
	}
	// 纯数字编号最多两位一段（避免 "14146"、"6.633"）；罗马数字必须带点
	if digits > 0 && roman > 0 {
		return false
	}
	if roman > 0 {
		return strings.HasSuffix(f, ".")
	}
	for _, part := range strings.Split(body, ".") {
		if utf16Len(part) > 2 {
			return false
		}
	}
	return true
}

// isAllCapsHeading 全大写标题（RELATED WORKS）：只允许字母，且至少两个词，
// 以免把 IVIF、ZOO、SDDGAN [74]、C (SSAB 这类缩写或表格行当成标题。
func isAllCapsHeading(s string) bool {
	n := utf16Len(s)
	if n < 3 || n > 45 {
		return false
	}
	fields := strings.Fields(s)
	if len(fields) < 2 {
		return false
	}
	for _, f := range fields {
		core := strings.Trim(f, ".,;:")
		for _, r := range core {
			if !unicode.IsLetter(r) {
				return false
			}
		}
	}
	letters, upper := 0, 0
	for _, r := range s {
		if unicode.IsLetter(r) {
			letters++
			if unicode.IsUpper(r) {
				upper++
			}
		}
	}
	return letters >= 4 && letters == upper
}

func typicalLineLen(items []srcLine) int {
	lens := []int{}
	for _, it := range items {
		if it.pageNum || it.text == "" {
			continue
		}
		lens = append(lens, utf16Len(it.text))
	}
	if len(lens) == 0 {
		return 60
	}
	sort.Ints(lens)
	idx := len(lens) * 3 / 4
	if idx >= len(lens) {
		idx = len(lens) - 1
	}
	return lens[idx]
}

// repeatedLines 找出跨页重复出现的行（页眉、页脚、版权行）。
// 判据：长度 ≥20 且出现次数 ≥3（同一行在正文里重复三次以上几乎只可能是版面装饰）。
func repeatedLines(items []srcLine) map[string]bool {
	counts := map[string]int{}
	for _, it := range items {
		if it.pageNum || utf16Len(it.text) < 20 {
			continue
		}
		counts[normalizeLine(it.text)]++
	}
	out := map[string]bool{}
	for line, n := range counts {
		if n >= 3 {
			out[line] = true
		}
	}
	return out
}

func normalizeLine(s string) string {
	return strings.ToLower(strings.Join(strings.Fields(s), " "))
}

func utf16Len(s string) int { return len(utf16.Encode([]rune(s))) }

func atoiSafe(s string) int {
	n := 0
	for _, r := range s {
		if !unicode.IsDigit(r) {
			return n
		}
		n = n*10 + int(r-'0')
	}
	return n
}
