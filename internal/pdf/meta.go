package pdfmeta

import (
	"io"
	"path/filepath"
	"strings"

	"github.com/ledongthuc/pdf"
)

type Meta struct {
	Title    string
	Author   string
	Keywords string
	Subject  string
}

func ExtractWithFallback(path, fallbackTitle string) (Meta, error) {
	m := Meta{}
	// PDF Info 字典（尽力而为：解析失败不影响后续流程）
	if f, r, err := pdf.Open(path); err == nil {
		info := r.Trailer().Key("Info")
		m.Title = plausibleInfoTitle(info.Key("Title").Text())
		m.Author = strings.TrimSpace(info.Key("Author").Text())
		m.Keywords = strings.TrimSpace(info.Key("Keywords").Text())
		m.Subject = strings.TrimSpace(info.Key("Subject").Text())
		f.Close()
	}
	// 主路径：字体感知的按页结构化提取（行带字号，标题靠字号聚类）
	doc, err := ExtractStructured(path)
	if err == nil && strings.TrimSpace(doc.Text) != "" {
		if m.Title == "" {
			m.Title = SniffTitleFromLines(doc.Lines)
		}
		if m.Title == "" {
			m.Title = SniffTitle(doc.Text)
		}
	}
	if m.Title == "" {
		// 兜底：旧版流扫描（兼容 xref 损坏等库解析不了的 PDF）
		text, big, _ := ExtractPDFText(path)
		if m.Title == "" {
			m.Title = SniffTitle(big)
		}
		if m.Title == "" {
			m.Title = SniffTitle(text)
		}
	}
	if m.Title == "" {
		m.Title = cleanTitle(fallbackTitle)
	}
	if m.Title == "" {
		m.Title = titleFromFilename(path)
	}
	return m, nil
}

// infoTitleStoplist 里是常见的无意义 Info Title。
var infoTitleStoplist = []string{
	"appendix", "untitled", "unknown", "microsoft word", "powerpoint",
	"layout", "print", "slide", ".doc", ".pdf", ".tex", "untitled document",
}

// plausibleInfoTitle 校验 PDF Info 字典里的 Title 是否可信。
// 不少生成器会把文件名、"Appendix" 之类塞进 Info Title。
func plausibleInfoTitle(s string) string {
	s = strings.TrimSpace(s)
	if s == "" {
		return ""
	}
	low := strings.ToLower(s)
	for _, bad := range infoTitleStoplist {
		if strings.Contains(low, bad) {
			return ""
		}
	}
	// 无空格且过短的（单词）标题基本不可信
	if !strings.Contains(s, " ") && len([]rune(s)) < 12 {
		return ""
	}
	if !plausibleText(s, 4, 300, 0.4) {
		return ""
	}
	return s
}

func titleFromFilename(path string) string {
	return cleanTitle(filepath.Base(path))
}

func cleanTitle(s string) string {
	s = filepath.Base(s)
	s = strings.TrimSuffix(s, filepath.Ext(s))
	s = strings.ReplaceAll(s, "_", " ")
	s = strings.ReplaceAll(s, "-", " ")
	s = strings.ReplaceAll(s, ".", " ")
	return strings.TrimSpace(s)
}

func ExtractText(path string, maxChars int) (string, error) {
	if maxChars <= 0 {
		maxChars = 20000
	}
	text, big, err := ExtractPDFText(path)
	if err == nil && strings.TrimSpace(text) == "" && strings.TrimSpace(big) != "" {
		text = big
	}
	if err != nil || strings.TrimSpace(text) == "" {
		// 回退 ledongthuc（少数规范 PDF）
		f, r, oerr := pdf.Open(path)
		if oerr == nil {
			defer f.Close()
			if rd, rerr := r.GetPlainText(); rerr == nil {
				if data, derr := io.ReadAll(io.LimitReader(rd, int64(maxChars))); derr == nil {
					return string(data), nil
				}
			}
		}
		if err != nil {
			return "", err
		}
		return "", nil
	}
	if len(text) > maxChars {
		text = text[:maxChars]
	}
	return text, nil
}
