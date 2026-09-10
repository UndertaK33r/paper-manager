package pdfmeta

import (
	"strings"
	"testing"
)

// 页码行分页 + 硬换行重排为段落
func TestSplitReadingPagesAndParagraphs(t *testing.T) {
	text := strings.Join([]string{
		"1",
		"Distilling Textual Priors from LLM to Efficient Image Fusion",
		"Ran Zhang, Xuanhua He, Ke Cao",
		"Abstract",
		"—Multi-modality image fusion aims to synthesize a single, comprehensive image from",
		"multiple source inputs. Traditional approaches, such as CNNs and GANs, offer efficiency",
		"but struggle to handle low-quality inputs.",
		"1 Introduction",
		"Recent advances in text-guided methods leverage large model priors to overcome these",
		"limitations, but at the cost of significant computa-",
		"tional overhead, both in memory and inference time.",
		"2",
		"2 Method",
		"We propose a novel framework for distilling large model priors.",
	}, "\n")

	pages := SplitReading(text)
	if len(pages) != 2 {
		t.Fatalf("want 2 pages, got %d", len(pages))
	}
	if pages[0].Number != 1 || pages[1].Number != 2 {
		t.Fatalf("页码解析错误: %d, %d", pages[0].Number, pages[1].Number)
	}

	// 第一页：标题、作者、Abstract 标题、摘要正文
	var headings []string
	for _, p := range pages[0].Paragraphs {
		if p.Heading {
			headings = append(headings, paraText(p))
		}
	}
	joined := strings.Join(headings, "|")
	if !strings.Contains(joined, "Abstract") || !strings.Contains(joined, "1 Introduction") {
		t.Fatalf("标题识别不全: %q", joined)
	}

	// 摘要正文应被合并成一段，且去掉了行尾连字符
	var abstract string
	for i, p := range pages[0].Paragraphs {
		if paraText(p) == "Abstract" && i+1 < len(pages[0].Paragraphs) {
			abstract = paraText(pages[0].Paragraphs[i+1])
		}
	}
	if !strings.Contains(abstract, "single, comprehensive image from multiple source inputs.") {
		t.Fatalf("断行未合并: %q", abstract)
	}

	// 正文段落里的行尾断词应还原（"computa-" + "tional" → "computational"）
	var intro string
	for i, p := range pages[0].Paragraphs {
		if paraText(p) == "1 Introduction" && i+1 < len(pages[0].Paragraphs) {
			intro = paraText(pages[0].Paragraphs[i+1])
		}
	}
	if strings.Contains(intro, "computa-") {
		t.Fatalf("连字符未处理: %q", intro)
	}
	if !strings.Contains(intro, "computational overhead") {
		t.Fatalf("去连字符结果不对: %q", intro)
	}

	// 片段 Join 语义：合并处应为 "space"，断词处应为 "none"
	for _, p := range pages[0].Paragraphs {
		for i, seg := range p.Segments {
			if i == 0 && seg.Join != "" {
				t.Fatalf("首片段不应有 Join: %+v", seg)
			}
			if i > 0 && seg.Join != "space" && seg.Join != "none" {
				t.Fatalf("Join 取值异常: %+v", seg)
			}
		}
	}
}

// 偏移量必须能在原文中定位到对应内容（UTF-16 码元）
func TestSplitReadingOffsetsMatchSource(t *testing.T) {
	text := "1\nFirst line here\nsecond wrapped line\n\n2\n另一个段落\n继续换行"
	units := utf16units(text)
	for _, page := range SplitReading(text) {
		for _, para := range page.Paragraphs {
			for _, seg := range para.Segments {
				n := len(utf16units(seg.Text))
				if seg.Start+n > len(units) {
					t.Fatalf("偏移越界: %+v", seg)
				}
				got := utf16toString(units[seg.Start : seg.Start+n])
				if got != seg.Text {
					t.Fatalf("偏移对不上: start=%d 期望 %q 实际 %q", seg.Start, seg.Text, got)
				}
			}
		}
	}
}

// 中文换行合并时不应插入空格
func TestSplitReadingCJKNoSpace(t *testing.T) {
	pages := SplitReading("1\n红外与可见光图像融合是\n一个重要的研究方向。")
	if len(pages) != 1 || len(pages[0].Paragraphs) != 1 {
		t.Fatalf("结构异常: %+v", pages)
	}
	got := paraText(pages[0].Paragraphs[0])
	if got != "红外与可见光图像融合是一个重要的研究方向。" {
		t.Fatalf("中文合并结果错误: %q", got)
	}
	seg := pages[0].Paragraphs[0].Segments[1]
	if seg.Join != "none" {
		t.Fatalf("中文片段之间应为 none: %+v", seg)
	}
}

// 没有页码行时也应正常返回单页
func TestSplitReadingWithoutPageNumbers(t *testing.T) {
	pages := SplitReading("Just some text without page markers.\nSecond line.")
	if len(pages) != 1 {
		t.Fatalf("want 1 page, got %d", len(pages))
	}
	if pages[0].Number != 1 {
		t.Fatalf("默认页码应为 1: %d", pages[0].Number)
	}
	full := paraText(pages[0].Paragraphs[0])
	if !strings.Contains(full, "Just some text without page markers. Second line.") {
		t.Fatalf("短行未合并: %q", full)
	}
}

func TestSplitReadingEmpty(t *testing.T) {
	if got := SplitReading("   \n  "); got != nil {
		t.Fatalf("空文本应返回 nil, got %+v", got)
	}
}

// paraText 按 Join 语义把段落片段拼回文本（与前端渲染规则一致）
func paraText(p ReadingParagraph) string {
	var b strings.Builder
	for i, seg := range p.Segments {
		if i > 0 && seg.Join != "none" {
			b.WriteString(" ")
		}
		b.WriteString(seg.Text)
	}
	return b.String()
}

func utf16toString(u []uint16) string {
	return string(utf16Decode(u))
}

// utf16Decode 把 UTF-16 码元解回字符串（处理代理对）
func utf16Decode(u []uint16) []rune {
	out := []rune{}
	for i := 0; i < len(u); i++ {
		c := u[i]
		if c >= 0xD800 && c < 0xDC00 && i+1 < len(u) && u[i+1] >= 0xDC00 && u[i+1] < 0xE000 {
			out = append(out, rune(c-0xD800)<<10|rune(u[i+1]-0xDC00)+0x10000)
			i++
			continue
		}
		out = append(out, rune(c))
	}
	return out
}

func utf16units(s string) []uint16 {
	out := []uint16{}
	for _, r := range s {
		if r > 0xFFFF {
			r -= 0x10000
			out = append(out, uint16(0xD800+(r>>10)), uint16(0xDC00+(r&0x3FF)))
			continue
		}
		out = append(out, uint16(r))
	}
	return out
}

// 首字母下沉（drop cap）：PDF 常把首字母单独抽出来
// 实测形态："I. I" + "NTRODUCTION" → "I. INTRODUCTION"；"I" + "MAGE fusion" → "IMAGE fusion"
func TestSplitReadingDropCap(t *testing.T) {
	text := strings.Join([]string{
		"1",
		"Some paper title here",
		"I. I",
		"NTRODUCTION",
		"I",
		"MAGE fusion plays an important role in visual enhancement within digital image",
		"processing. For example, visible-",
		"light images are captured in low-light conditions.",
		"R",
		"EFERENCES",
		"[1] J. Zhang, K. Cao, K. Yan",
	}, "\n")

	pages := SplitReading(text)
	if len(pages) != 1 {
		t.Fatalf("want 1 page, got %d", len(pages))
	}
	var texts []string
	var headings []string
	for _, para := range pages[0].Paragraphs {
		txt := paraText(para)
		texts = append(texts, txt)
		if para.Heading {
			headings = append(headings, txt)
		}
	}
	joined := strings.Join(texts, "\n")

	if !strings.Contains(joined, "I. INTRODUCTION") {
		t.Fatalf("标题首字母未拼回: %q", joined)
	}
	if !strings.Contains(joined, "IMAGE fusion plays an important role") {
		t.Fatalf("正文首字母未拼回: %q", joined)
	}
	if !strings.Contains(joined, "visible-light images are captured") {
		t.Fatalf("断词与断行未正确合并: %q", joined)
	}
	if strings.Contains(joined, "\nNTRODUCTION") || strings.Contains(joined, "\nMAGE") {
		t.Fatalf("仍存在被拆开的词: %q", joined)
	}
	// 拼回后的 "I. INTRODUCTION" / "REFERENCES" 应被识别为标题
	if !strings.Contains(strings.Join(headings, "|"), "I. INTRODUCTION") {
		t.Fatalf("首字母下沉的标题未标记为标题: %v", headings)
	}
	if !strings.Contains(strings.Join(headings, "|"), "REFERENCES") {
		t.Fatalf("REFERENCES 未识别为标题: %v", headings)
	}
}
