package api

import (
	"fmt"
	"net/http"
	"regexp"
	"strings"

	"paper-manager/internal/models"
)

var bibEscapeRe = regexp.MustCompile(`[&%$#_{}~^\\]`)

func bibEscape(s string) string {
	return bibEscapeRe.ReplaceAllStringFunc(s, func(c string) string {
		switch c {
		case "~":
			return `{\~}`
		case "^":
			return `{\^{}}`
		case "\\":
			return `\textbackslash{}`
		default:
			return `\` + c
		}
	})
}

func bibAuthorName(name string) string {
	if strings.Contains(name, ",") {
		return strings.TrimSpace(name)
	}
	parts := strings.Fields(name)
	if len(parts) <= 1 {
		return strings.TrimSpace(name)
	}
	return parts[len(parts)-1] + ", " + strings.Join(parts[:len(parts)-1], " ")
}

func bibAuthors(authors string) string {
	out := []string{}
	for _, a := range strings.Split(authors, ",") {
		a = strings.TrimSpace(a)
		if a != "" {
			out = append(out, bibAuthorName(a))
		}
	}
	return strings.Join(out, " and ")
}

var bibKeyCleanRe = regexp.MustCompile(`[^A-Za-z0-9]`)

func bibKey(p models.Paper) string {
	key := ""
	first := ""
	if strings.TrimSpace(p.Authors) != "" {
		first = bibAuthorName(strings.Split(p.Authors, ",")[0])
		key = bibKeyCleanRe.ReplaceAllString(strings.Split(first, ",")[0], "")
	}
	if key == "" {
		key = fmt.Sprintf("paper%d", p.ID)
	}
	if p.Year > 0 {
		key += fmt.Sprintf("%d", p.Year)
	}
	return key
}

func toBibEntry(p models.Paper) string {
	typ := "misc"
	if p.Venue != "" {
		typ = "article"
	}
	var b strings.Builder
	fmt.Fprintf(&b, "@%s{%s,\n", typ, bibKey(p))
	fmt.Fprintf(&b, "  title = {%s},\n", bibEscape(p.Title))
	if strings.TrimSpace(p.Authors) != "" {
		fmt.Fprintf(&b, "  author = {%s},\n", bibEscape(bibAuthors(p.Authors)))
	}
	if p.Year > 0 {
		fmt.Fprintf(&b, "  year = {%d},\n", p.Year)
	}
	if p.Venue != "" {
		fmt.Fprintf(&b, "  journal = {%s},\n", bibEscape(p.Venue))
	}
	if p.DOI != "" {
		fmt.Fprintf(&b, "  doi = {%s},\n", bibEscape(p.DOI))
	}
	b.WriteString("}\n")
	return b.String()
}

func (s *Server) handleExportBib(w http.ResponseWriter, r *http.Request) {
	q := s.buildQuery(r)
	q.Page = 1
	q.PageSize = 1000
	res, err := s.store.ListPapers(q)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	var b strings.Builder
	for _, p := range res.Papers {
		b.WriteString(toBibEntry(p))
	}
	w.Header().Set("Content-Type", "application/x-bibtex; charset=utf-8")
	w.Header().Set("Content-Disposition", `attachment; filename="papers.bib"`)
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write([]byte(b.String()))
}
