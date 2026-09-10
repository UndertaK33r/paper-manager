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
	// "none" 直接相接（CJK 换行、英文行尾连字符断词）
	Join string `json:"join,omitempty"`
}

type ReadingParagraph struct {
	Heading  bool             `json:"heading,omitempty"`
	Segments []ReadingSegment `json:"segments"`
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
	typical := medianLineLen(items)

	pages := []ReadingPage{}
	page := ReadingPage{Number: 1, Paragraphs: []ReadingParagraph{}}
	para := ReadingParagraph{Segments: []ReadingSegment{}}
	prevText := ""
	prevEndsPara := true

	flushPara := func() {
		if len(para.Segments) > 0 {
			page.Paragraphs = append(page.Paragraphs, para)
			para = ReadingParagraph{Segments: []ReadingSegment{}}
		}
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
		if prevEndsPara || heading || looksLikeHeading(prevText) {
			flushPara()
		}

		seg := ReadingSegment{Start: ln.start, Text: ln.text}
		if len(para.Segments) > 0 {
			prev := &para.Segments[len(para.Segments)-1]
			switch {
			case shouldDropHyphen(prev.Text, ln.text):
				prev.Text = strings.TrimSuffix(prev.Text, "-")
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
		prevText = ln.text
		prevEndsPara = heading || utf16Len(ln.text) < typical*3/4
	}
	flushPage()
	return pages
}

// shouldDropHyphen 判断英文行尾断词：上一行以连字符结尾且下一行以小写字母开头。
func shouldDropHyphen(prev, cur string) bool {
	if !strings.HasSuffix(prev, "-") || utf16Len(prev) <= 2 {
		return false
	}
	head := []rune(strings.TrimSuffix(prev, "-"))
	tail := []rune(cur)
	if len(head) == 0 || len(tail) == 0 {
		return false
	}
	lc, fc := head[len(head)-1], tail[0]
	return unicode.IsLetter(lc) && unicode.IsLower(fc)
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

func looksLikeHeading(s string) bool {
	if s == "" {
		return false
	}
	lower := strings.ToLower(strings.TrimSpace(s))
	if sectionWords[strings.TrimRight(lower, ". ")] {
		return true
	}
	if utf16Len(s) <= 70 {
		fields := strings.Fields(s)
		if len(fields) >= 2 && isNumberedPrefix(fields[0]) {
			return true
		}
	}
	if n := utf16Len(s); n >= 3 && n <= 60 {
		letters, upper := 0, 0
		for _, r := range s {
			if unicode.IsLetter(r) {
				letters++
				if unicode.IsUpper(r) {
					upper++
				}
			}
		}
		if letters >= 3 && upper == letters {
			return true
		}
	}
	return false
}

func isNumberedPrefix(f string) bool {
	f = strings.TrimSuffix(f, ".")
	if f == "" {
		return false
	}
	for _, r := range f {
		if !unicode.IsDigit(r) && r != '.' {
			return false
		}
	}
	return true
}

func medianLineLen(items []srcLine) int {
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
	return lens[len(lens)/2]
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
