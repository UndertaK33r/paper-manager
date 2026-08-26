package pdfmeta

import (
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
	f, r, err := pdf.Open(path)
	if err != nil {
		return Meta{}, err
	}
	defer f.Close()
	info := r.Trailer().Key("Info")
	m := Meta{}
	m.Title = strings.TrimSpace(info.Key("Title").Text())
	m.Author = strings.TrimSpace(info.Key("Author").Text())
	m.Keywords = strings.TrimSpace(info.Key("Keywords").Text())
	m.Subject = strings.TrimSpace(info.Key("Subject").Text())
	if m.Title == "" {
		m.Title = titleFromFilename(path)
	}
	return m, nil
}

func titleFromFilename(path string) string {
	base := filepath.Base(path)
	base = strings.TrimSuffix(base, filepath.Ext(base))
	base = strings.ReplaceAll(base, "_", " ")
	base = strings.ReplaceAll(base, "-", " ")
	base = strings.ReplaceAll(base, ".", " ")
	return strings.TrimSpace(base)
}

func IsValidPDF(path string) bool {
	f, r, err := pdf.Open(path)
	if err != nil {
		return false
	}
	defer f.Close()
	_ = r
	return true
}

func (m Meta) HasMeta() bool {
	return m.Title != "" || m.Author != "" || m.Keywords != ""
}
