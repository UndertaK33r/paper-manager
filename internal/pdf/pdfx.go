package pdfmeta

import (
	"bytes"
	"compress/flate"
	"compress/zlib"
	"encoding/binary"
	"io"
	"os"
	"regexp"
	"strconv"
	"strings"
	"unicode/utf16"
)

const maxTextLen = 5 << 20 // 5 MB of extracted text per paper

// winAnsi maps bytes 0x80..0xFF (WinAnsiEncoding) to Unicode runes.
var winAnsi = []rune{
	0x20AC, 0x0081, 0x201A, 0x0192, 0x201E, 0x2026, 0x2020, 0x2021,
	0x02C6, 0x2030, 0x0160, 0x2039, 0x0152, 0x008D, 0x017D, 0x008F,
	0x0090, 0x2018, 0x2019, 0x201C, 0x201D, 0x2022, 0x2013, 0x2014,
	0x02DC, 0x2122, 0x0161, 0x203A, 0x0153, 0x009D, 0x017E, 0x0178,
	0x00A0, 0x00A1, 0x00A2, 0x00A3, 0x00A4, 0x00A5, 0x00A6, 0x00A7,
	0x00A8, 0x00A9, 0x00AA, 0x00AB, 0x00AC, 0x00AD, 0x00AE, 0x00AF,
	0x00B0, 0x00B1, 0x00B2, 0x00B3, 0x00B4, 0x00B5, 0x00B6, 0x00B7,
	0x00B8, 0x00B9, 0x00BA, 0x00BB, 0x00BC, 0x00BD, 0x00BE, 0x00BF,
	0x00C0, 0x00C1, 0x00C2, 0x00C3, 0x00C4, 0x00C5, 0x00C6, 0x00C7,
	0x00C8, 0x00C9, 0x00CA, 0x00CB, 0x00CC, 0x00CD, 0x00CE, 0x00CF,
	0x00D0, 0x00D1, 0x00D2, 0x00D3, 0x00D4, 0x00D5, 0x00D6, 0x00D7,
	0x00D8, 0x00D9, 0x00DA, 0x00DB, 0x00DC, 0x00DD, 0x00DE, 0x00DF,
	0x00E0, 0x00E1, 0x00E2, 0x00E3, 0x00E4, 0x00E5, 0x00E6, 0x00E7,
	0x00E8, 0x00E9, 0x00EA, 0x00EB, 0x00EC, 0x00ED, 0x00EE, 0x00EF,
	0x00F0, 0x00F1, 0x00F2, 0x00F3, 0x00F4, 0x00F5, 0x00F6, 0x00F7,
	0x00F8, 0x00F9, 0x00FA, 0x00FB, 0x00FC, 0x00FD, 0x00FE, 0x00FF,
}

var doiRe = regexp.MustCompile(`(?i)\b10\.\d{4,9}/[^\s"'<>()\[\]{}]+`)

// ExtractPDFText 提取 PDF 文本。返回 (全文, 大字号文本)：后者用于标题嗅探
// （论文标题通常是页面中字号最大的文本）。加密或扫描版 PDF 返回空串。
func ExtractPDFText(path string) (string, string, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return "", "", err
	}
	if bytes.Contains(data[:min(len(data), 2048)], []byte("/Encrypt")) {
		return "", "", nil
	}
	var out strings.Builder
	var big strings.Builder
	out.Grow(64 << 10)
	scanStreams(data, &out, &big)
	if out.Len() == 0 {
		return "", "", nil
	}
	text := normalizeText(out.String())
	text = cleanText(text)
	if len(text) > maxTextLen {
		text = text[:maxTextLen]
	}
	bigText := strings.TrimSpace(big.String())
	if len(bigText) > 64<<10 {
		bigText = bigText[:64<<10]
	}
	return text, bigText, nil
}

// SniffDOI returns the first DOI-looking string in the first 64KB of text.
func SniffDOI(text string) string {
	if len(text) > 64<<10 {
		text = text[:64<<10]
	}
	if m := doiRe.FindString(text); m != "" {
		return strings.TrimRight(m, ".,;:)]}")
	}
	return ""
}

// SniffTitle returns a likely title: the first plausible non-empty line.
func SniffTitle(text string) string {
	if len(text) > 8<<10 {
		text = text[:8<<10]
	}
	for _, line := range strings.Split(text, "\n") {
		line = strings.TrimSpace(line)
		if len([]rune(line)) < 8 || len([]rune(line)) > 200 {
			continue
		}
		if len(line) > 0 && line[0] >= '0' && line[0] <= '9' {
			continue // 跳过 "1 Introduction" 这类章节行
		}
		if doiRe.MatchString(line) {
			continue
		}
		low := strings.ToLower(line)
		if strings.HasPrefix(low, "http") || strings.HasPrefix(low, "arxiv") ||
			strings.HasPrefix(low, "doi:") || strings.HasPrefix(low, "©") {
			continue
		}
		if strings.Trim(line, "0123456789 ") == "" {
			continue
		}
		return line
	}
	return ""
}

// TitleFromFilename derives a title candidate from a PDF file name.
func TitleFromFilename(name string) string {
	name = strings.TrimSuffix(name, ".pdf")
	name = strings.TrimSuffix(name, ".PDF")
	name = strings.ReplaceAll(name, "_", " ")
	name = strings.ReplaceAll(name, "-", " ")
	name = regexp.MustCompile(`\s+`).ReplaceAllString(name, " ")
	return strings.TrimSpace(name)
}

func scanStreams(data []byte, out *strings.Builder, big *strings.Builder) {
	pos := 0
	for {
		i := bytes.Index(data[pos:], []byte("stream"))
		if i < 0 {
			return
		}
		start := pos + i
		dictStart := start
		if k := bytes.LastIndex(data[:start], []byte("obj")); k >= 0 {
			dictStart = k + 3
		}
		dict := data[dictStart:start]
		s := bytes.Index(data[start+6:], []byte("endstream"))
		if s < 0 {
			return
		}
		dataStart := start + 6
		for dataStart < len(data) && (data[dataStart] == '\r' || data[dataStart] == '\n') {
			dataStart++
		}
		dataEnd := start + 6 + s
		for dataEnd > dataStart && (data[dataEnd-1] == '\r' || data[dataEnd-1] == '\n') {
			dataEnd--
		}
		stream := data[dataStart:dataEnd]

		if bytes.Contains(dict, []byte("/FlateDecode")) &&
			!bytes.Contains(dict, []byte("/Predictor")) &&
			!bytes.Contains(dict, []byte("/ObjStm")) &&
			!bytes.Contains(dict, []byte("/Image")) &&
			!bytes.Contains(dict, []byte("/CMap")) &&
			!isFontStream(dict) {
			dec, err := decompressStream(stream)
			if err == nil && !isCMapContent(dec) {
				extractTextOps(dec, out, big)
			}
		} else if !hasPDFFilter(dict) && !isFontStream(dict) && !isCMapContent(stream) {
			extractTextOps(stream, out, big)
		}
		pos = start + 6 + s + len("endstream")
	}
}

// decompressStream 解压 PDF /FlateDecode 流。PDF 规范要求 zlib 包装
// (RFC1950)，但不少生成器输出 raw deflate (RFC1951)，两种都兼容。
func decompressStream(data []byte) ([]byte, error) {
	if len(data) >= 2 && data[0] == 0x78 && (uint16(data[0])<<8|uint16(data[1]))%31 == 0 {
		zr, err := zlib.NewReader(bytes.NewReader(data))
		if err != nil {
			return nil, err
		}
		defer zr.Close()
		return io.ReadAll(io.LimitReader(zr, 64<<20))
	}
	zr := flate.NewReader(bytes.NewReader(data))
	defer zr.Close()
	return io.ReadAll(io.LimitReader(zr, 64<<20))
}

// isFontStream 判断流是否为嵌入字体程序（Type1/TrueType/CFF/OpenType）。
// 这类流的 dict 含 /FontFile、/FontFile2、/FontFile3 或 Type1 特有的
// /Length1 分段长度；字体二进制被当正文提取会产生大量乱码。
func isFontStream(dict []byte) bool {
	if bytes.Contains(dict, []byte("/FontFile")) {
		return true
	}
	if bytes.Contains(dict, []byte("/FontName")) {
		return true
	}
	return bytes.Contains(dict, []byte("/Length1"))
}

// isCMapContent 按内容特征识别 CMap 字体映射流（有些 PDF 的 CMap 流
// dict 没有 /CMap 标记，只有 /Filter /Length）。CMap 内容是
// PostScript 程序（%!PS-Adobe-3.0 Resource-CMap / begincmap / CIDInit）。
func isCMapContent(content []byte) bool {
	if len(content) < 8 {
		return false
	}
	head := content[:min(len(content), 300)]
	return bytes.Contains(head, []byte("Resource-CMap")) ||
		bytes.Contains(head, []byte("begincmap")) ||
		bytes.Contains(head, []byte("/CIDInit"))
}

func hasPDFFilter(dict []byte) bool {
	for _, f := range []string{
		"/FlateDecode", "/DCTDecode", "/JPXDecode", "/CCITTFaxDecode",
		"/ASCIIHexDecode", "/ASCII85Decode", "/RunLengthDecode",
		"/LZWDecode", "/Crypt",
	} {
		if bytes.Contains(dict, []byte(f)) {
			return true
		}
	}
	return false
}

// extractTextOps 扫描内容流，提取文本操作符（Tj TJ ' "）的文本，
// 每行输出一次 show 操作。同时跟踪字号（Tf）：大字号（≥13.5pt，
// 通常是论文标题）的行额外写入 big 收集器。
func extractTextOps(dec []byte, out *strings.Builder, big *strings.Builder) {
	var pending []string
	fontSize := 0.0
	lastNum := 0.0
	emitLine := func() {
		if len(pending) > 0 {
			line := strings.Join(pending, " ")
			out.WriteString(line)
			out.WriteByte('\n')
			if fontSize >= 13.5 {
				big.WriteString(line)
				big.WriteByte('\n')
			}
			pending = pending[:0]
		}
	}
	flush := func() {
		if len(pending) > 0 {
			out.WriteString(strings.Join(pending, " "))
			pending = pending[:0]
		}
	}
	i, n := 0, len(dec)
	for i < n {
		c := dec[i]
		switch {
		case c == '(':
			raw, ni := parseParenString(dec, i)
			pending = append(pending, decodeString(raw))
			i = ni
		case c == '<':
			if i+1 < n && dec[i+1] == '<' {
				i += 2
				continue
			}
			raw, ni := parseHexString(dec, i)
			pending = append(pending, decodeString(raw))
			i = ni
		case c == '[':
			s, ni := parseTJArray(dec, i)
			pending = append(pending, s)
			i = ni
		case c == ']':
			i++
		case c == '/':
			i++
			for i < n && !isDelim(dec[i]) {
				i++
			}
		case c == '%':
			for i < n && dec[i] != '\n' {
				i++
			}
		case isNumStart(c):
			v, ni := parseNumber(dec, i)
			lastNum = v
			i = ni
		case c == 'T' || c == '\'' || c == '"':
			op := readOperator(dec, i)
			switch op {
			case "Tj", "'", "\"", "TJ":
				emitLine()
			case "Tf":
				fontSize = lastNum
				flush()
			default:
				flush()
			}
			i += len(op)
		default:
			i++
		}
	}
	flush()
}

// parseTJArray 解析一个 TJ 数组（'[' 起始），把元素文本按 kerning 阈值
// 拼接：元素间右移超过 150/1000 em（词间距）时插入空格，否则直接拼接
// （字符级 kerning，如 "the"+15+"y" -> "they"）。
func parseTJArray(dec []byte, i int) (string, int) {
	var sb strings.Builder
	lastKern := 0.0
	i++ // skip '['
	for i < len(dec) && dec[i] != ']' {
		c := dec[i]
		switch {
		case c == '(':
			raw, ni := parseParenString(dec, i)
			s := decodeString(raw)
			if sb.Len() > 0 && lastKern < -150 {
				sb.WriteByte(' ')
			}
			sb.WriteString(s)
			lastKern = 0
			i = ni
		case c == '<':
			if i+1 < len(dec) && dec[i+1] == '<' {
				i += 2
				continue
			}
			raw, ni := parseHexString(dec, i)
			s := decodeString(raw)
			if sb.Len() > 0 && lastKern < -150 {
				sb.WriteByte(' ')
			}
			sb.WriteString(s)
			lastKern = 0
			i = ni
		case isNumStart(c):
			v, ni := parseNumber(dec, i)
			lastKern = v
			i = ni
		case c == '/':
			i++
			for i < len(dec) && !isDelim(dec[i]) {
				i++
			}
		default:
			i++
		}
	}
	if i < len(dec) {
		i++ // skip ']'
	}
	return sb.String(), i
}

// parseNumber 解析十进制数（含负数/小数/指数）。
func parseNumber(dec []byte, i int) (float64, int) {
	j := i
	for j < len(dec) && (isNumStart(dec[j]) || dec[j] == 'e' || dec[j] == 'E') {
		j++
	}
	v, err := strconv.ParseFloat(string(dec[i:j]), 64)
	if err != nil {
		return 0, j
	}
	return v, j
}

func readOperator(dec []byte, i int) string {
	j := i
	for j < len(dec) {
		c := dec[j]
		if (c >= 'a' && c <= 'z') || (c >= 'A' && c <= 'Z') || c == '\'' || c == '"' {
			j++
		} else {
			break
		}
	}
	return string(dec[i:j])
}

func isNumStart(c byte) bool {
	return (c >= '0' && c <= '9') || c == '-' || c == '+' || c == '.'
}

func isDelim(c byte) bool {
	switch c {
	case ' ', '\n', '\r', '\t', '(', ')', '<', '>', '[', ']', '{', '}', '/', '%':
		return true
	}
	return false
}

// parseParenString parses a PDF literal string starting at dec[i]=='('.
// Escapes are processed; returns the raw bytes and the index after ')'.
func parseParenString(dec []byte, i int) ([]byte, int) {
	var out []byte
	depth := 1
	i++
	for i < len(dec) && depth > 0 {
		c := dec[i]
		switch c {
		case '\\':
			if i+1 >= len(dec) {
				i++
				continue
			}
			e := dec[i+1]
			switch e {
			case 'n':
				out = append(out, '\n')
				i += 2
			case 'r':
				out = append(out, '\r')
				i += 2
			case 't':
				out = append(out, '\t')
				i += 2
			case 'b':
				out = append(out, '\b')
				i += 2
			case 'f':
				out = append(out, '\f')
				i += 2
			case '(', ')', '\\':
				out = append(out, e)
				i += 2
			default:
				if e >= '0' && e <= '7' {
					// 八进制转义（最多 3 位）：i 推进到转义序列之后
					v := 0
					k := i + 1
					for k < len(dec) && k < i+4 && dec[k] >= '0' && dec[k] <= '7' {
						v = v*8 + int(dec[k]-'0')
						k++
					}
					out = append(out, byte(v))
					i = k
				} else {
					out = append(out, e)
					i += 2
				}
			}
		case '(':
			depth++
			out = append(out, c)
			i++
		case ')':
			depth--
			if depth > 0 {
				out = append(out, c)
			}
			i++
		default:
			out = append(out, c)
			i++
		}
	}
	return out, i
}

// parseHexString parses a PDF hex string starting at dec[i]=='<'.
func parseHexString(dec []byte, i int) ([]byte, int) {
	var out []byte
	i++
	hi := byte(0)
	haveHi := false
	for i < len(dec) && dec[i] != '>' {
		c := dec[i]
		if c == ' ' || c == '\n' || c == '\r' || c == '\t' {
			i++
			continue
		}
		v := hexVal(c)
		if v < 0 {
			i++
			continue
		}
		if !haveHi {
			hi = byte(v)
			haveHi = true
		} else {
			out = append(out, hi<<4|byte(v))
			haveHi = false
		}
		i++
	}
	if haveHi {
		out = append(out, hi<<4)
	}
	if i < len(dec) {
		i++
	}
	return out, i
}

func hexVal(c byte) int {
	switch {
	case c >= '0' && c <= '9':
		return int(c - '0')
	case c >= 'a' && c <= 'f':
		return int(c-'a') + 10
	case c >= 'A' && c <= 'F':
		return int(c-'A') + 10
	}
	return -1
}

// decodeString 将 PDF 字符串字节转为 Go 字符串：
//  1. 带 UTF-16 BOM 的按 UTF-16BE 解码；
//  2. 启发式判定为 UTF-16BE 的（中文 PDF 的 Type0/CID 字体常见，
//     无 BOM 双字节）按 UTF-16BE 解码；
//  3. 否则按单字节（ASCII + WinAnsi）解码。
func decodeString(bs []byte) string {
	if len(bs) >= 2 && bs[0] == 0xFE && bs[1] == 0xFF {
		return utf16be(bs[2:])
	}
	if looksUTF16BE(bs) {
		return utf16be(bs)
	}
	var sb strings.Builder
	for _, b := range bs {
		if b < 0x80 {
			if b < 0x20 {
				sb.WriteByte(' ')
			} else {
				sb.WriteByte(b)
			}
		} else {
			sb.WriteRune(winAnsi[b-0x80])
		}
	}
	return sb.String()
}

func utf16be(bs []byte) string {
	u := make([]uint16, 0, len(bs)/2)
	for i := 0; i+1 < len(bs); i += 2 {
		u = append(u, binary.BigEndian.Uint16(bs[i:i+2]))
	}
	return string(utf16.Decode(u))
}

// looksUTF16BE 启发式判断双字节文本是否为 UTF-16BE：
//
//	a) 出现 0x00 字节（单字节可打印文本几乎不会含 NUL）→ 强信号；
//	b) 无 0x00 时，要求至少 40% 的字节对含高字节（≥0x80）且可打印
//	   ASCII 对 ≤70% —— 覆盖成片中文 UTF-16BE（"深度学习""注意力机制"），
//	   同时法/德重音文本（个别高字节）不会被误判。
//
// 短串（<8 字节）且无 0x00 时不启用。
func looksUTF16BE(bs []byte) bool {
	if len(bs) < 4 || len(bs)%2 != 0 {
		return false
	}
	pairs := len(bs) / 2
	widePairs := 0
	asciiPairs := 0
	for i := 0; i+1 < len(bs); i += 2 {
		b0, b1 := bs[i], bs[i+1]
		if b0 == 0 || b1 == 0 {
			return true // 0x00 强信号
		}
		if b0 >= 0x80 || b1 >= 0x80 {
			widePairs++
		}
		if b0 >= 0x20 && b0 <= 0x7E && b1 >= 0x20 && b1 <= 0x7E {
			asciiPairs++
		}
	}
	if len(bs) < 8 {
		return false
	}
	return widePairs*10 >= pairs*4 && asciiPairs*10 <= pairs*7
}

// normalizeText collapses whitespace runs and blank lines.
func normalizeText(s string) string {
	var sb strings.Builder
	sb.Grow(len(s))
	last := byte(0)
	for _, r := range s {
		switch {
		case r == '\n':
			if last != '\n' {
				sb.WriteByte('\n')
				last = '\n'
			}
		case r == ' ' || r == '\t' || r == '\r' || r == '\f' || r == '\v':
			if last != ' ' && last != '\n' {
				sb.WriteByte(' ')
				last = ' '
			}
		default:
			sb.WriteRune(r)
			last = 0
		}
	}
	return strings.TrimSpace(sb.String())
}

var (
	cleanHyphenRe  = regexp.MustCompile(`\s+-\s*`) // " -decoder" → "-decoder"；数学 "a - b" → "a-b"
	cleanParenLRe  = regexp.MustCompile(`\(\s+`)
	cleanParenRRe  = regexp.MustCompile(`\s+\)`)
	cleanBracketRe = regexp.MustCompile(`\[\s+([^\]]+?)\s+\]`)
	cleanPunctRe   = regexp.MustCompile(`\s+([,.;!?])`)
	// "a : b" → "a: b"；" :::" 数学省略号（冒号后跟冒号）不受影响
	cleanColonRe = regexp.MustCompile(`\s+:([^:]|$)`)
	cleanSpaceRe = regexp.MustCompile(`[ \t]+`)
)

// cleanText 修复 TJ 拆分导致的排版瑕疵：
//
//	[ 13 ] → [13]（引用编号）；( x ) → (x)；encoder -decoder → encoder-decoder；
//	"word , word" → "word, word"。数学变量（h t 1）与丢字（signi cant）
//	无法可靠恢复，保持原样。
func cleanText(text string) string {
	text = cleanHyphenRe.ReplaceAllString(text, "-")
	text = cleanParenLRe.ReplaceAllString(text, "(")
	text = cleanParenRRe.ReplaceAllString(text, ")")
	text = cleanBracketRe.ReplaceAllString(text, "[$1]")
	text = cleanPunctRe.ReplaceAllString(text, "$1")
	text = cleanColonRe.ReplaceAllString(text, ":$1")
	return cleanSpaceRe.ReplaceAllString(text, " ")
}
