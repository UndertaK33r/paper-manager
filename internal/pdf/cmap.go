package pdfmeta

import (
	"io"
	"strings"
	"unicode/utf16"

	"github.com/ledongthuc/pdf"
)

// toUnicodeMap 保存一个字体 ToUnicode CMap 的 code → Unicode 映射。
type toUnicodeMap struct {
	width int               // 每个 code 的字节数（1 或 2，来自 CMap 源码宽度）
	m     map[uint64]string // code 数值 → 文本
}

// fontDecoder 负责把内容流里的字符串字节按该字体的编码转成 Unicode。
type fontDecoder struct {
	width int
	tu    *toUnicodeMap
	enc   pdf.TextEncoding // 无 ToUnicode 时的兜底（库按 /Encoding Differences 解码）
}

// newFontDecoder 根据字体字典构建解码器。优先 ToUnicode CMap
// （覆盖 CID/Identity-H 与子集字体），否则退回库的 Encoder
// （处理 /Differences、WinAnsi/MacRoman 名称），最后退回启发式。
func newFontDecoder(f pdf.Font) *fontDecoder {
	w := 1
	if f.V.Key("Subtype").Name() == "Type0" {
		w = 2
	}
	d := &fontDecoder{width: w}
	if tv := f.V.Key("ToUnicode"); tv.Kind() == pdf.Stream {
		if rd := tv.Reader(); rd != nil {
			if data, err := io.ReadAll(rd); err == nil {
				if m := parseToUnicode(data); m != nil && len(m.m) > 0 {
					d.tu = m
					d.width = m.width
				}
			}
		}
	}
	if d.tu == nil && d.width == 1 {
		d.enc = f.Encoder()
	}
	return d
}

func (d *fontDecoder) decode(raw string) string {
	if raw == "" {
		return ""
	}
	// 去 UTF-16 BOM
	if len(raw) >= 2 && raw[0] == 0xFE && raw[1] == 0xFF {
		raw = raw[2:]
		if d.width < 2 {
			d.width = 2
		}
	}
	switch {
	case d.tu != nil:
		return d.decodeWithMap(raw)
	case d.width == 2:
		return decodeString([]byte(raw)) // 启发式 UTF-16BE / WinAnsi
	default:
		return d.decodeWithLib(raw)
	}
}

// decodeWithMap 按 d.width 字节步进查 ToUnicode 映射。
func (d *fontDecoder) decodeWithMap(raw string) string {
	var sb strings.Builder
	bs := []byte(raw)
	w := d.width
	for i := 0; i+w <= len(bs); {
		var code uint64
		for k := 0; k < w; k++ {
			code = code<<8 | uint64(bs[i+k])
		}
		i += w
		if s, ok := d.tu.m[code]; ok {
			sb.WriteString(s)
			continue
		}
		// 映射缺失：单字节字体退回 WinAnsi；CID 字体丢弃该码
		if w == 1 {
			sb.WriteString(winAnsiByte(bs[i-1]))
		}
	}
	return sb.String()
}

// decodeWithLib 逐字节走库的 Encoder（含 /Differences）。
func (d *fontDecoder) decodeWithLib(raw string) string {
	var sb strings.Builder
	for _, b := range []byte(raw) {
		s := d.enc.Decode(string(b))
		if s == "" {
			continue
		}
		r := []rune(s)
		if len(r) == 1 && r[0] == rune(b) && b >= 0x80 {
			// 库未映射的高字节：按 WinAnsi 兜底
			sb.WriteString(winAnsiByte(b))
			continue
		}
		sb.WriteString(s)
	}
	return sb.String()
}

func winAnsiByte(b byte) string {
	if b < 0x80 {
		if b < 0x20 {
			return " "
		}
		return string(rune(b))
	}
	return string(winAnsi[b-0x80])
}

// parseToUnicode 解析 ToUnicode CMap 流（bfchar / bfrange 段）。
func parseToUnicode(data []byte) *toUnicodeMap {
	toks := cmapTokens(data)
	if toks == nil {
		return nil
	}
	m := &toUnicodeMap{m: map[uint64]string{}}
	widthSet := false
	setWidth := func(w int) {
		if !widthSet {
			m.width = w
			widthSet = true
		}
	}
	for i := 0; i < len(toks); i++ {
		switch toks[i].s {
		case "beginbfchar":
			// <src> <dst> 直到 endbfchar
			for i+2 < len(toks) && toks[i+1].s != "endbfchar" {
				if toks[i+1].kind != 'h' || toks[i+2].kind != 'h' {
					i++
					continue
				}
				src, dst := toks[i+1], toks[i+2]
				if code, ok := hexCode(src.s); ok {
					setWidth(len(src.s) / 2)
					if txt := utf16beString(dst.s); txt != "" {
						m.m[code] = txt
					}
				}
				i += 2
			}
		case "beginbfrange":
			// <lo> <hi> <dst> | <lo> <hi> [<d1> <d2> ...] 直到 endbfrange
			for i+3 < len(toks) && toks[i+1].s != "endbfrange" {
				if toks[i+1].kind != 'h' || toks[i+2].kind != 'h' {
					i++
					continue
				}
				loT, hiT := toks[i+1], toks[i+2]
				lo, ok1 := hexCode(loT.s)
				hi, ok2 := hexCode(hiT.s)
				if !ok1 || !ok2 || hi < lo || hi-lo > 65535 {
					// 跳过这一条目
					i += 3
					continue
				}
				setWidth(len(loT.s) / 2)
				j := i + 3
				if j < len(toks) && toks[j].kind == '[' {
					// 数组形式：lo→d1, lo+1→d2 ...
					j++
					code := lo
					for j < len(toks) && toks[j].kind != ']' {
						if toks[j].kind == 'h' {
							if txt := utf16beString(toks[j].s); txt != "" {
								m.m[code] = txt
							}
							code++
						}
						j++
					}
					i = j
					continue
				}
				if j < len(toks) && toks[j].kind == 'h' {
					if base := utf16beString(toks[j].s); len([]rune(base)) == 1 {
						// 连续区间，dst 是起点
						r := []rune(base)[0]
						for c := lo; c <= hi; c++ {
							m.m[c] = string(r + rune(c-lo))
						}
					}
					i = j
					continue
				}
				i += 3
			}
		}
	}
	if !widthSet || len(m.m) == 0 {
		return nil
	}
	return m
}

// cmapTokens 把 CMap 流切成 token。kind：'h' hex 串、'n' 数字、
// 'k' 关键字/名字、'['、']'。
type cmapToken struct {
	kind byte
	s    string
}

func cmapTokens(data []byte) []cmapToken {
	var out []cmapToken
	i, n := 0, len(data)
	for i < n {
		c := data[i]
		switch {
		case c == ' ' || c == '\n' || c == '\r' || c == '\t' || c == '\f' || c == 0:
			i++
		case c == '%': // 注释
			for i < n && data[i] != '\n' {
				i++
			}
		case c == '<':
			if i+1 < n && data[i+1] == '<' {
				i += 2
				continue
			}
			i++
			var hex []byte
			for i < n && data[i] != '>' {
				c := data[i]
				if isHexDigit(c) {
					hex = append(hex, c)
				}
				i++
			}
			i++ // skip '>'
			if len(hex)%2 == 1 {
				hex = append(hex, '0')
			}
			out = append(out, cmapToken{'h', string(hex)})
		case c == '[':
			out = append(out, cmapToken{'[', "["})
			i++
		case c == ']':
			out = append(out, cmapToken{']', "]"})
			i++
		case c >= '0' && c <= '9' || c == '-' || c == '+' || c == '.':
			j := i
			for j < n && (data[j] >= '0' && data[j] <= '9' || data[j] == '.' || data[j] == '-' || data[j] == '+') {
				j++
			}
			out = append(out, cmapToken{'n', string(data[i:j])})
			i = j
		case c == '/':
			i++
			j := i
			for j < n && !isCMapDelim(data[j]) {
				j++
			}
			out = append(out, cmapToken{'k', string(data[i:j])})
			i = j
		case (c >= 'a' && c <= 'z') || (c >= 'A' && c <= 'Z'):
			j := i
			for j < n && !isCMapDelim(data[j]) {
				j++
			}
			out = append(out, cmapToken{'k', string(data[i:j])})
			i = j
		default:
			i++
		}
	}
	return out
}

func isCMapDelim(c byte) bool {
	switch c {
	case ' ', '\n', '\r', '\t', '\f', '%', '<', '>', '[', ']', '{', '}', '(', ')', '/':
		return true
	}
	return false
}

func isHexDigit(c byte) bool {
	return c >= '0' && c <= '9' || c >= 'a' && c <= 'f' || c >= 'A' && c <= 'F'
}

// hexCode 把 hex token（已去掉 <>，偶数长度）转成数值。
func hexCode(hex string) (uint64, bool) {
	if hex == "" || len(hex) > 8 {
		return 0, false
	}
	var v uint64
	for i := 0; i < len(hex); i += 2 {
		hi := hexVal(hex[i])
		lo := hexVal(hex[i+1])
		if hi < 0 || lo < 0 {
			return 0, false
		}
		v = v<<8 | uint64(hi<<4|lo)
	}
	return v, true
}

// utf16beString 把 hex token 当 UTF-16BE 解码为文本。
func utf16beString(hex string) string {
	if hex == "" || len(hex)%4 != 0 {
		return ""
	}
	u := make([]uint16, 0, len(hex)/4)
	for i := 0; i+3 < len(hex); i += 4 {
		b0, b1 := hexVal(hex[i]), hexVal(hex[i+1])
		b2, b3 := hexVal(hex[i+2]), hexVal(hex[i+3])
		if b0 < 0 || b1 < 0 || b2 < 0 || b3 < 0 {
			return ""
		}
		u = append(u, uint16(b0<<4|b1)<<8|uint16(b2<<4|b3))
	}
	return string(utf16.Decode(u))
}
