package pdfmeta

import (
	"fmt"
	"regexp"
	"sort"
	"strings"
	"unicode"

	"github.com/ledongthuc/pdf"
)

// Line 是按页提取的一行文本，带字号（用于标题嗅探）。
type Line struct {
	Page int
	Size float64
	Text string
}

// Doc 是结构化提取结果：行级信息 + 规整后的全文。
type Doc struct {
	Lines []Line
	Text  string
}

// ExtractStructured 用字体感知解码按页提取文本。
// 需要可读的 xref/页树；解析失败时返回错误，调用方应退回
// 旧版流扫描器（ExtractPDFText）。
func ExtractStructured(path string) (*Doc, error) {
	f, r, err := pdf.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	n := r.NumPage()
	if n <= 0 {
		return nil, fmt.Errorf("no pages")
	}
	doc := &Doc{}
	var sb strings.Builder
	for i := 1; i <= n; i++ {
		lines := pageLines(r.Page(i), i)
		doc.Lines = append(doc.Lines, lines...)
		for _, l := range lines {
			sb.WriteString(l.Text)
			sb.WriteByte('\n')
		}
	}
	text := cleanText(normalizeText(sb.String()))
	if len(text) > maxTextLen {
		text = text[:maxTextLen]
	}
	doc.Text = text
	return doc, nil
}

// pageLines 解释一个页面的内容流，产出按绘制顺序的行。
func pageLines(p pdf.Page, pageNum int) (lines []Line) {
	lines = []Line{}
	defer func() {
		// 库在遇到非法操作符时会 panic，吞掉以保证其余页面可用
		_ = recover()
	}()
	fonts := map[string]*fontDecoder{}
	for _, name := range p.Fonts() {
		fonts[name] = newFontDecoder(p.Font(name))
	}
	var (
		cur      *fontDecoder
		size     float64
		lineSize float64
		lastTmY  *float64
		buf      strings.Builder
	)
	endLine := func() {
		if buf.Len() == 0 {
			return
		}
		t := strings.TrimSpace(buf.String())
		buf.Reset()
		if t == "" {
			return
		}
		lines = append(lines, Line{Page: pageNum, Size: lineSize, Text: t})
	}
	writeText := func(raw string) {
		if raw == "" {
			return
		}
		var s string
		if cur != nil {
			s = cur.decode(raw)
		} else {
			s = decodeString([]byte(raw))
		}
		if s == "" {
			return
		}
		if buf.Len() == 0 {
			lineSize = size
		}
		buf.WriteString(s)
	}
	interpretPage(p, func(stk *pdf.Stack, op string) {
		n := stk.Len()
		args := make([]pdf.Value, n)
		for i := n - 1; i >= 0; i-- {
			args[i] = stk.Pop()
		}
		switch op {
		case "BT", "ET", "T*", "Td", "TD":
			endLine()
		case "Tm": // a b c d e f：部分生成器只换 Tm 不发换行符
			if len(args) == 6 {
				ty := args[5].Float64()
				if buf.Len() > 0 && (lastTmY == nil || ty != *lastTmY) {
					endLine()
				}
				lastTmY = &ty
			}
		case "TL", "Tc", "Tw", "Tz", "Ts", "Tr":
			// 文本参数，不换行
		case "Tf": // /Font size
			if len(args) == 2 {
				size = args[1].Float64()
				if fd, ok := fonts[args[0].Name()]; ok {
					cur = fd
				} else {
					cur = nil
				}
			}
		case "Tj":
			if len(args) == 1 {
				writeText(args[0].RawString())
			}
		case "'":
			endLine()
			if len(args) == 1 {
				writeText(args[0].RawString())
			}
		case "\"":
			endLine()
			if len(args) == 3 {
				writeText(args[2].RawString())
			}
		case "TJ":
			if len(args) == 0 {
				return
			}
			v := args[0]
			for i := 0; i < v.Len(); i++ {
				x := v.Index(i)
				switch x.Kind() {
				case pdf.String:
					writeText(x.RawString())
				case pdf.Integer, pdf.Real:
					// 字距调整：明显后移视为词间空格
					if x.Float64() < -150 && buf.Len() > 0 {
						buf.WriteByte(' ')
					}
				}
			}
		}
	})
	endLine()
	// 合并竖排 CJK 短行（逐字一行），改善标题嗅探与全文质量
	return mergeVerticalRuns(lines)
}

// interpretPage 在 recover 保护下运行 Interpret。
func interpretPage(p pdf.Page, fn func(stk *pdf.Stack, op string)) {
	defer func() { _ = recover() }()
	if p.V.IsNull() {
		return
	}
	contents := p.V.Key("Contents")
	if contents.IsNull() {
		return
	}
	pdf.Interpret(contents, fn)
}

// SniffTitleFromLines 用字号聚类从行里挑标题：
// 第一页字号最大的合理行优先，其次第二页（封面场景），逐级降档最多 3 层。
func SniffTitleFromLines(lines []Line) string {
	for _, pg := range []int{1, 2} {
		if t := sniffTitleOnPage(lines, pg); t != "" {
			return t
		}
	}
	return ""
}

func sniffTitleOnPage(lines []Line, page int) string {
	var cands []Line
	for _, l := range lines {
		if l.Page != page {
			continue
		}
		t := strings.Join(strings.Fields(l.Text), " ")
		if t == "" {
			continue
		}
		cands = append(cands, Line{l.Page, l.Size, t})
	}
	if len(cands) == 0 {
		return ""
	}
	// 先合并竖排短行，再做合理性过滤（单字行先合并才够判定的长度）
	var filtered []Line
	for _, c := range mergeVerticalRuns(cands) {
		if plausibleLine(c.Text) {
			filtered = append(filtered, c)
		}
	}
	cands = filtered
	if len(cands) == 0 {
		return ""
	}
	// 字号降序作为档位
	sizes := []float64{}
	seen := map[float64]bool{}
	for _, c := range cands {
		if !seen[c.Size] {
			seen[c.Size] = true
			sizes = append(sizes, c.Size)
		}
	}
	sort.Float64s(sizes)
	for tier := 0; tier < 3 && len(sizes) > 0; tier++ {
		top := sizes[len(sizes)-1]
		sizes = sizes[:len(sizes)-1]
		joined, ok := joinTitleTier(cands, top)
		if ok && plausibleTitle(joined) {
			return joined
		}
	}
	return ""
}

// mergeVerticalRuns 合并竖排标题：中文论文常见每个字单独一行（1-2 字符、
// 相同字号、连续出现）。把这样的连续 CJK 短行拼成一行。只对 CJK 内容生效，
// 避免误并英文短词或页码。
func mergeVerticalRuns(cands []Line) []Line {
	out := []Line{}
	i := 0
	for i < len(cands) {
		cur := cands[i]
		if isShortCJK(cur.Text) {
			j := i + 1
			var sb strings.Builder
			sb.WriteString(cur.Text)
			for j < len(cands) {
				if !isShortCJK(cands[j].Text) {
					break
				}
				sameSize := cands[j].Size <= cur.Size+0.2 && cands[j].Size >= cur.Size-0.2
				if !sameSize {
					break
				}
				sb.WriteString(cands[j].Text)
				j++
			}
			if sb.Len() > len(cur.Text) {
				cur = Line{cur.Page, cur.Size, sb.String()}
			}
			out = append(out, cur)
			i = j
			continue
		}
		out = append(out, cur)
		i++
	}
	return out
}

// isShortCJK 判断是否为"竖排单字"式短行：≤2 字符且含 CJK 字符/标点。
func isShortCJK(s string) bool {
	rs := []rune(s)
	if len(rs) == 0 || len(rs) > 2 {
		return false
	}
	for _, r := range rs {
		if isCJK(r) {
			return true
		}
	}
	return false
}

func isCJK(r rune) bool {
	return r >= 0x2E80 && r <= 0x9FFF || // CJK 部首/汉字
		r >= 0x3000 && r <= 0x303F || // CJK 标点
		r >= 0xFF00 && r <= 0xFFEF || // 全角字符
		r >= 0x20000 && r <= 0x2FA1F // 扩展区
}

// joinTitleTier 把同字号档位的行拼成标题。遇到"像作者列表"的行即停止，
// 避免把作者行粘进标题；总词数 40 上限兜底。
// 例外：上一行以介词/冠词结尾（如 "...for"）时语法未完结，无条件续接。
func joinTitleTier(cands []Line, top float64) (string, bool) {
	var parts []string
	words := 0
	for _, c := range cands {
		if c.Size < top-0.5 {
			continue
		}
		if len(parts) > 0 {
			prev := parts[len(parts)-1]
			if !endsWithFunctionWord(prev) && authorLike(c.Text) {
				break
			}
		}
		parts = append(parts, c.Text)
		words += len(strings.Fields(c.Text))
		if words > 40 {
			break
		}
	}
	if len(parts) == 0 {
		return "", false
	}
	return strings.Join(parts, " "), true
}

// functionWords 英文标题虚词：以这些词结尾说明句子没说完。
var functionWords = map[string]bool{
	"for": true, "of": true, "and": true, "with": true, "in": true,
	"on": true, "to": true, "by": true, "a": true, "an": true,
	"the": true, "via": true, "using": true, "from": true, "as": true,
	"at": true, "into": true, "onto": true, "over": true, "under": true,
}

func endsWithFunctionWord(line string) bool {
	fields := strings.Fields(strings.TrimSpace(line))
	if len(fields) == 0 {
		return false
	}
	last := strings.Trim(fields[len(fields)-1], ".,;:!?—–-()[]\"'")
	return functionWords[strings.ToLower(last)]
}

// authorLike 粗判一行是否为作者列表（如 "Bingxin Ke Anton Obukhov" 或
// "Jinyuan Liu, Member, IEEE"）：全部词首字母大写、不含小写虚词。
// 英文标题几乎总含 for/of/and 之类小写词，中文标题切词后往往不足 2 词。
func authorLike(line string) bool {
	words := strings.Fields(line)
	if len(words) < 2 || len(words) > 14 {
		return false
	}
	for _, w := range words {
		w = strings.Trim(w, ".,;:·")
		if w == "" {
			continue
		}
		rs := []rune(w)
		if len(rs) > 1 && unicode.IsLower(rs[0]) {
			return false // 小写开头的词（for/of/and/…）说明是标题延续
		}
		if len(rs) > 14 {
			return false // 过长词更可能是标题内容
		}
	}
	return true
}

// plausibleLine 判断一行是否可能是标题组成（排除页眉/DOI/URL/纯符号等）。
func plausibleLine(s string) bool {
	return plausibleText(s, 4, 300, 0.5)
}

// plausibleTitle 在行级条件之上再加整体约束。
func plausibleTitle(s string) bool {
	return plausibleText(s, 8, 300, 0.45)
}

// plausibleText 通用合理性检查：长度区间 + 字母/数字占比。
func plausibleText(s string, minRunes, maxRunes int, minRatio float64) bool {
	s = strings.TrimSpace(s)
	rs := []rune(s)
	if len(rs) < minRunes || len(rs) > maxRunes {
		return false
	}
	low := strings.ToLower(s)
	for _, bad := range []string{
		"arxiv:", "doi:", "doi.org", "http://", "https://", "www.",
		"©", "copyright", "isbn", "issn",
	} {
		if strings.Contains(low, bad) {
			return false
		}
	}
	if isJunkLine(low) {
		return false
	}
	alnum := 0
	for _, r := range rs {
		if isLetterOrDigit(r) {
			alnum++
		}
	}
	return float64(alnum) >= float64(len(rs))*minRatio
}

func isLetterOrDigit(r rune) bool {
	return r >= 'a' && r <= 'z' || r >= 'A' && r <= 'Z' ||
		r >= '0' && r <= '9' || r >= 0x80 // 非 ASCII 一律按字母类（中日韩等）
}

func isJunkLine(low string) bool {
	junk := []string{
		"abstract", "contents", "table of contents", "references",
		"acknowledg", "keywords", "index terms", "vol.", "no. ",
		"introduction", "conclusion", "related work",
		// 中文论文常见栏目
		"摘要", "关键词", "参考文献", "目录", "附录",
	}
	for _, j := range junk {
		if strings.HasPrefix(low, j) {
			return true
		}
	}
	// "page 3" 这类页脚，但不影响 "Page Rank" 之类的标题
	return pageJunkRe.MatchString(low)
}

var pageJunkRe = regexp.MustCompile(`^page \d`)
