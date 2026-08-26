package api

import (
	"encoding/json"

	"fmt"
	"net/http"
	"net/url"
	"paper-manager/internal/models"
	"strings"
	"sync"
	"time"
	"unicode"
)

type GraphNode struct {
	ID        string `json:"id"`
	Title     string `json:"title"`
	Year      int    `json:"year"`
	Venue     string `json:"venue"`
	DOI       string `json:"doi"`
	InLibrary bool   `json:"in_library"`
}

type GraphEdge struct {
	Source string `json:"source"`
	Target string `json:"target"`
}

type GraphResponse struct {
	Nodes   []GraphNode `json:"nodes"`
	Edges   []GraphEdge `json:"edges"`
	Warning string      `json:"warning,omitempty"`
}

type oaRefs struct {
	OpenAlexID string   `json:"openalex_id"`
	References []string `json:"references"`
}

type refsCache struct {
	FetchedAt time.Time         `json:"fetched_at"`
	Works     map[string]oaRefs `json:"works"`
}

const (
	maxRefsPerPaper = 15
	maxExtNodes     = 150
)

func (s *Server) loadGraphCache() refsCache {
	var c refsCache
	if raw := s.store.GetSetting("graph_cache"); raw != "" {
		_ = json.Unmarshal([]byte(raw), &c)
	}
	if c.Works == nil {
		c.Works = map[string]oaRefs{}
	}
	return c
}

func (s *Server) saveGraphCache(c refsCache) {
	if data, err := json.Marshal(c); err == nil {
		_ = s.store.SetSetting("graph_cache", string(data))
	}
}

func (s *Server) handleGraph(w http.ResponseWriter, r *http.Request) {
	force := r.URL.Query().Get("refresh") == "1"
	resp := s.buildGraph(force)
	writeJSON(w, http.StatusOK, resp)
}

func (s *Server) buildGraph(force bool) GraphResponse {
	resp := GraphResponse{}
	cache := s.loadGraphCache()
	var cacheMu sync.Mutex
	if cache.FetchedAt.IsZero() {
		cache.FetchedAt = time.Now()
	}
	list, err := s.store.ListPapers(models.PaperQuery{Page: 1, PageSize: 500})
	if err != nil {
		resp.Warning = err.Error()
		return resp
	}
	type libRef struct {
		p    models.Paper
		refs oaRefs
	}
	var refs []libRef
	var mu sync.Mutex
	var wg sync.WaitGroup
	sem := make(chan struct{}, 6)
	netErr := false
	for _, p := range list.Papers {
		if p.DOI == "" && p.Title == "" {
			continue
		}
		p := p
		wg.Add(1)
		sem <- struct{}{}
		go func() {
			defer wg.Done()
			defer func() { <-sem }()
			r, ok := getRefsCached(p.DOI, p.Title, &cache, &cacheMu, force)
			if !ok {
				mu.Lock()
				netErr = true
				mu.Unlock()
				return
			}
			mu.Lock()
			refs = append(refs, libRef{p: p, refs: r})
			mu.Unlock()
		}()
	}
	wg.Wait()
	s.saveGraphCache(cache)
	if netErr {
		resp.Warning = "部分论文的引用数据获取失败（网络或未收录），已显示已获取部分"
	}
	libID := map[int64]string{}
	for _, r := range refs {
		id := fmt.Sprintf("lib-%d", r.p.ID)
		libID[r.p.ID] = id
		resp.Nodes = append(resp.Nodes, GraphNode{ID: id, Title: r.p.Title, Year: r.p.Year, Venue: r.p.Venue, DOI: r.p.DOI, InLibrary: true})
	}
	extByOA := map[string]string{}
	extOrder := []string{}
	edgeSeen := map[string]bool{}
	for _, r := range refs {
		for n := 0; n < len(r.refs.References) && n < maxRefsPerPaper; n++ {
			refID := r.refs.References[n]
			if refID == r.refs.OpenAlexID {
				continue
			}
			nodeID, ok := extByOA[refID]
			if !ok {
				if len(extByOA) >= maxExtNodes {
					continue
				}
				nodeID = "ext-" + refID
				extByOA[refID] = nodeID
				extOrder = append(extOrder, refID)
			}
			e := fmt.Sprintf("%s->%s", libID[r.p.ID], nodeID)
			if !edgeSeen[e] {
				edgeSeen[e] = true
				resp.Edges = append(resp.Edges, GraphEdge{Source: libID[r.p.ID], Target: nodeID})
			}
		}
	}
	titles := fetchExternalTitles(extOrder)
	for _, oaID := range extOrder {
		resp.Nodes = append(resp.Nodes, GraphNode{ID: extByOA[oaID], Title: titles[oaID], InLibrary: false})
	}
	return resp
}

func getRefsCached(doi, title string, cache *refsCache, mu *sync.Mutex, force bool) (oaRefs, bool) {
	mu.Lock()
	if r, ok := cache.Works[doi]; ok && !force && time.Since(cache.FetchedAt) < 7*24*time.Hour {
		mu.Unlock()
		return r, true
	}
	mu.Unlock()
	r, err := fetchOpenAlexRefs(doi, title)
	if err != nil {
		mu.Lock()
		if r, ok := cache.Works[doi]; ok {
			mu.Unlock()
			return r, true
		}
		mu.Unlock()
		return oaRefs{}, false
	}
	mu.Lock()
	if doi != "" {
		cache.Works[doi] = r
	}
	mu.Unlock()
	return r, true
}

func fetchOpenAlexRefs(doi, title string) (oaRefs, error) {
	if doi != "" {
		if r, err := fetchOpenAlexByDOI(doi); err == nil {
			return r, nil
		}
	}
	if title != "" {
		return fetchOpenAlexByTitle(title)
	}
	return oaRefs{}, fmt.Errorf("no doi or title")
}

func oaGetJSON(raw string, out any) error {
	client := &http.Client{Timeout: 8 * time.Second}
	req, err := http.NewRequest(http.MethodGet, raw, nil)
	if err != nil {
		return err
	}
	req.Header.Set("User-Agent", "paper-manager/1.0 (personal)")
	resp, err := client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("status %d", resp.StatusCode)
	}
	return json.NewDecoder(resp.Body).Decode(out)
}

type oaWork struct {
	ID              string   `json:"id"`
	Title           string   `json:"title"`
	ReferencedWorks []string `json:"referenced_works"`
}

func fetchOpenAlexByDOI(doi string) (oaRefs, error) {
	u := "https://api.openalex.org/works?per-page=1&filter=doi:" + url.QueryEscape(doi)
	var resp struct {
		Results []oaWork `json:"results"`
	}
	if err := oaGetJSON(u, &resp); err != nil {
		return oaRefs{}, err
	}
	if len(resp.Results) == 0 {
		return oaRefs{}, fmt.Errorf("no openalex record")
	}
	w := resp.Results[0]
	return oaRefs{OpenAlexID: w.ID, References: w.ReferencedWorks}, nil
}

func fetchOpenAlexByTitle(title string) (oaRefs, error) {
	u := "https://api.openalex.org/works?per-page=5&filter=title.search:" + url.QueryEscape(title)
	var resp struct {
		Results []oaWork `json:"results"`
	}
	if err := oaGetJSON(u, &resp); err != nil {
		return oaRefs{}, err
	}
	for _, w := range resp.Results {
		if w.Title != "" && titleOverlapLocal(title, w.Title) >= 0.6 {
			return oaRefs{OpenAlexID: w.ID, References: w.ReferencedWorks}, nil
		}
	}
	return oaRefs{}, fmt.Errorf("no title match")
}

func fetchExternalTitles(ids []string) map[string]string {
	out := map[string]string{}
	for i := 0; i < len(ids); i += 50 {
		end := i + 50
		if end > len(ids) {
			end = len(ids)
		}
		batch := ids[i:end]
		u := "https://api.openalex.org/works?per-page=50&filter=openalex_id:" + url.QueryEscape(strings.Join(batch, "|"))
		var resp struct {
			Results []struct {
				ID    string `json:"id"`
				Title string `json:"title"`
			} `json:"results"`
		}
		if oaGetJSON(u, &resp) != nil {
			continue
		}
		for _, r := range resp.Results {
			if r.Title != "" {
				out[r.ID] = r.Title
			}
		}
	}
	return out
}

func titleOverlapLocal(a, b string) float64 {
	na := strings.ToLower(strings.TrimSpace(a))
	nb := strings.ToLower(strings.TrimSpace(b))
	if na == "" || nb == "" {
		return 0
	}
	if strings.Contains(na, nb) || strings.Contains(nb, na) {
		return 1
	}
	ta := tokenSetLocal(na)
	tb := tokenSetLocal(nb)
	if len(ta) == 0 {
		return 0
	}
	inter := 0
	for w := range ta {
		if tb[w] {
			inter++
		}
	}
	return float64(inter) / float64(len(ta))
}

func tokenSetLocal(s string) map[string]bool {
	out := map[string]bool{}
	for _, w := range strings.FieldsFunc(s, func(r rune) bool { return !unicode.IsLetter(r) && !unicode.IsNumber(r) }) {
		if w != "" {
			out[w] = true
		}
	}
	return out
}
