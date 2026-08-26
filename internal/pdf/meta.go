package pdfmeta

import (
	"io"
	"os"
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

func Extract(path string) (Meta, error) {
	return ExtractWithFallback(path, "")
}

func ExtractWithFallback(path, fallbackTitle string) (Meta, error) {
	m := Meta{}
	// PDF Info 字典（尽力而为：解析失败不影响后续流程）
	if f, r, err := pdf.Open(path); err == nil {
		info := r.Trailer().Key("Info")
		m.Title = strings.TrimSpace(info.Key("Title").Text())
		m.Author = strings.TrimSpace(info.Key("Author").Text())
		m.Keywords = strings.TrimSpace(info.Key("Keywords").Text())
		m.Subject = strings.TrimSpace(info.Key("Subject").Text())
		f.Close()
	}
	// 老项目同款流扫描提取（不依赖 xref，兼容性更好）
	text, big, _ := ExtractPDFText(path)
	if m.Title == "" {
		m.Title = SniffTitle(text)
	}
	if m.Title == "" {
		m.Title = SniffTitle(big)
	}
	if m.Title == "" {
		m.Title = cleanTitle(fallbackTitle)
	}
	if m.Title == "" {
		m.Title = titleFromFilename(path)
	}
	return m, nil
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

func IsValidPDF(path string) bool {
	data, err := os.ReadFile(path)
	if err != nil || len(data) < 5 {
		return false
	}
	return string(data[:5]) == "%PDF-"
}

func (m Meta) HasMeta() bool {
	return m.Title != "" || m.Author != "" || m.Keywords != ""
}
