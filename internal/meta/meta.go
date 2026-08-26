package meta

import (
	"context"
	"encoding/json"
	"encoding/xml"
	"net/http"
	"net/url"
	"regexp"
	"strconv"
	"strings"
	"time"
	"unicode"
)

type Info struct {
	Title    string
	Authors  []string
	Year     int
	Venue    string
	DOI      string
	Abstract string
	Source   string
}

var doiRe = regexp.MustCompile(`10\.\d{4,9}/[^\s]+`)

func SniffDOI(text string) string {
	m := doiRe.FindString(text)
	if m == "" {
		return ""
	}
	m = strings.TrimRight(m, ".,;)]}")
	return m
}

func SniffTitle(text string) string {
	for _, raw := range strings.Split(text, "\n") {
		line := strings.TrimSpace(raw)
		if len(line) < 5 || len(line) > 200 {
			continue
		}
		first := rune(line[0])
		if unicode.IsDigit(first) {
			continue
		}
		low := strings.ToLower(line)
		if strings.HasPrefix(low, "doi") || strings.HasPrefix(low, "copyright") || strings.HasPrefix(low, "abstract") || strings.HasPrefix(low, "http") || strings.HasPrefix(low, "www") || strings.HasPrefix(low, "arxiv:") || strings.HasPrefix(low, "@") {
			continue
		}
		return line
	}
	return ""
}

func EnrichByDOI(ctx context.Context, doi string) (*Info, error) {
	if doi == "" {
		return nil, nil
	}
	if info, err := openAlexDOI(ctx, doi); err == nil && info != nil && info.Title != "" {
		return info, nil
	}
	if info, err := crossrefByDOI(ctx, doi); err == nil && info != nil && info.Title != "" {
		return info, nil
	}
	return nil, nil
}

func SearchByTitle(ctx context.Context, title string) (*Info, error) {
	if title == "" {
		return nil, nil
	}
	if info, err := crossrefSearch(ctx, title); err == nil && info != nil {
		return info, nil
	}
	if info, err := arxivSearch(ctx, title); err == nil && info != nil {
		return info, nil
	}
	return nil, nil
}

func httpGetJSON(ctx context.Context, raw string, out any) error {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, raw, nil)
	if err != nil {
		return err
	}
	req.Header.Set("User-Agent", "paper-manager/1.0 (personal)")
	client := &http.Client{Timeout: 8 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil
	}
	return json.NewDecoder(resp.Body).Decode(out)
}

func openAlexDOI(ctx context.Context, doi string) (*Info, error) {
	var resp struct {
		Results []struct {
			DOI             string `json:"doi"`
			DisplayName     string `json:"display_name"`
			Year            int    `json:"publication_year"`
			PrimaryLocation struct {
				Source struct {
					DisplayName string `json:"display_name"`
				} `json:"source"`
			} `json:"primary_location"`
			Authorships []struct {
				Author struct {
					DisplayName string `json:"display_name"`
				} `json:"author"`
			} `json:"authorships"`
			AbstractInvertedIndex map[string][]int `json:"abstract_inverted_index"`
		} `json:"results"`
	}
	u := "https://api.openalex.org/works?filter=doi:" + url.QueryEscape(doi) + "&per-page=1&mailto=paper-manager@example.com"
	if err := httpGetJSON(ctx, u, &resp); err != nil {
		return nil, err
	}
	if len(resp.Results) == 0 || resp.Results[0].DisplayName == "" {
		return nil, nil
	}
	r := resp.Results[0]
	info := &Info{Title: r.DisplayName, Year: r.Year, DOI: r.DOI, Source: "openalex"}
	info.Venue = r.PrimaryLocation.Source.DisplayName
	for _, a := range r.Authorships {
		if a.Author.DisplayName != "" {
			info.Authors = append(info.Authors, a.Author.DisplayName)
		}
	}
	info.Abstract = rebuildAbstract(r.AbstractInvertedIndex)
	return info, nil
}

func rebuildAbstract(idx map[string][]int) string {
	type pair struct {
		w string
		p int
	}
	pairs := []pair{}
	for w, ps := range idx {
		for _, p := range ps {
			pairs = append(pairs, pair{w, p})
		}
	}
	max := 0
	for _, p := range pairs {
		if p.p > max {
			max = p.p
		}
	}
	words := make([]string, max+1)
	for _, p := range pairs {
		if p.p < len(words) {
			words[p.p] = p.w
		}
	}
	out := []string{}
	for _, w := range words {
		if w != "" {
			out = append(out, w)
		}
	}
	return strings.Join(out, " ")
}

func crossrefByDOI(ctx context.Context, doi string) (*Info, error) {
	var resp struct {
		Message struct {
			DOI    string   `json:"DOI"`
			Title  []string `json:"title"`
			Author []struct {
				Given  string `json:"given"`
				Family string `json:"family"`
			} `json:"author"`
			Issued struct {
				DateParts [][]int `json:"date-parts"`
			} `json:"issued"`
			ContainerTitle []string `json:"container-title"`
			Abstract       string   `json:"abstract"`
		} `json:"message"`
	}
	u := "https://api.crossref.org/works/" + url.PathEscape(doi)
	if err := httpGetJSON(ctx, u, &resp); err != nil {
		return nil, err
	}
	m := resp.Message
	if len(m.Title) == 0 {
		return nil, nil
	}
	info := &Info{Title: m.Title[0], DOI: m.DOI, Abstract: cleanAbstract(m.Abstract), Source: "crossref"}
	if len(m.Issued.DateParts) > 0 && len(m.Issued.DateParts[0]) > 0 {
		info.Year = m.Issued.DateParts[0][0]
	}
	if len(m.ContainerTitle) > 0 {
		info.Venue = m.ContainerTitle[0]
	}
	for _, a := range m.Author {
		name := strings.TrimSpace(a.Given + " " + a.Family)
		if name != "" {
			info.Authors = append(info.Authors, name)
		}
	}
	return info, nil
}

func crossrefSearch(ctx context.Context, title string) (*Info, error) {
	var resp struct {
		Message struct {
			Items []struct {
				Score  float64  `json:"score"`
				DOI    string   `json:"DOI"`
				Title  []string `json:"title"`
				Author []struct {
					Given  string `json:"given"`
					Family string `json:"family"`
				} `json:"author"`
				Issued struct {
					DateParts [][]int `json:"date-parts"`
				} `json:"issued"`
				ContainerTitle []string `json:"container-title"`
				Abstract       string   `json:"abstract"`
			} `json:"items"`
		} `json:"message"`
	}
	u := "https://api.crossref.org/works?query.bibliographic=" + url.QueryEscape(title) + "&rows=5"
	if err := httpGetJSON(ctx, u, &resp); err != nil {
		return nil, err
	}
	for _, item := range resp.Message.Items {
		if len(item.Title) == 0 || item.Score < 50 {
			continue
		}
		if titleOverlap(title, item.Title[0]) < 0.4 {
			continue
		}
		info := &Info{Title: item.Title[0], DOI: item.DOI, Abstract: cleanAbstract(item.Abstract), Source: "crossref"}
		if len(item.Issued.DateParts) > 0 && len(item.Issued.DateParts[0]) > 0 {
			info.Year = item.Issued.DateParts[0][0]
		}
		if len(item.ContainerTitle) > 0 {
			info.Venue = item.ContainerTitle[0]
		}
		for _, a := range item.Author {
			name := strings.TrimSpace(a.Given + " " + a.Family)
			if name != "" {
				info.Authors = append(info.Authors, name)
			}
		}
		return info, nil
	}
	return nil, nil
}

type arxivFeed struct {
	Entries []struct {
		ID        string `xml:"id"`
		Title     string `xml:"title"`
		Summary   string `xml:"summary"`
		Published string `xml:"published"`
		Authors   []struct {
			Name string `xml:"name"`
		} `xml:"author"`
	} `xml:"entry"`
}

func arxivSearch(ctx context.Context, title string) (*Info, error) {
	u := "http://export.arxiv.org/api/query?search_query=ti:%22" + url.QueryEscape(title) + "%22&max_results=5"
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, u, nil)
	if err != nil {
		return nil, err
	}
	client := &http.Client{Timeout: 8 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	var feed arxivFeed
	if err := xml.NewDecoder(resp.Body).Decode(&feed); err != nil {
		return nil, err
	}
	for _, e := range feed.Entries {
		if titleOverlap(title, e.Title) < 0.5 {
			continue
		}
		info := &Info{Title: strings.TrimSpace(e.Title), Abstract: strings.TrimSpace(e.Summary), Source: "arxiv"}
		if len(e.Published) >= 4 {
			if y, err := strconv.Atoi(e.Published[:4]); err == nil {
				info.Year = y
			}
		}
		for _, a := range e.Authors {
			if a.Name != "" {
				info.Authors = append(info.Authors, a.Name)
			}
		}
		return info, nil
	}
	return nil, nil
}

func titleOverlap(a, b string) float64 {
	normA := normalizeTitle(a)
	normB := normalizeTitle(b)
	if normA == "" || normB == "" {
		return 0
	}
	if strings.Contains(normA, normB) || strings.Contains(normB, normA) {
		return 1
	}
	tokA := tokenSet(normA)
	tokB := tokenSet(normB)
	if len(tokA) == 0 {
		return 0
	}
	inter := 0
	for w := range tokA {
		if tokB[w] {
			inter++
		}
	}
	return float64(inter) / float64(len(tokA))
}

func normalizeTitle(s string) string {
	return strings.ToLower(strings.TrimSpace(s))
}

func tokenSet(s string) map[string]bool {
	out := map[string]bool{}
	for _, w := range strings.FieldsFunc(s, func(r rune) bool { return !unicode.IsLetter(r) && !unicode.IsNumber(r) }) {
		if w != "" {
			out[w] = true
		}
	}
	return out
}

func cleanAbstract(s string) string {
	s = strings.TrimSpace(s)
	s = strings.TrimPrefix(s, "<jats:abstract>")
	s = strings.TrimSuffix(s, "</jats:abstract>")
	s = strings.ReplaceAll(s, "<jats:title>", "")
	s = strings.ReplaceAll(s, "</jats:title>", "")
	return strings.TrimSpace(s)
}
