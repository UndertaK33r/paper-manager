package models

import "time"

type Category struct {
	ID         int64  `json:"id"`
	Name       string `json:"name"`
	PaperCount int64  `json:"paperCount"`
}

type Tag struct {
	ID         int64  `json:"id"`
	Name       string `json:"name"`
	PaperCount int64  `json:"paperCount"`
}

type Collection struct {
	ID          int64  `json:"id"`
	Name        string `json:"name"`
	Description string `json:"description"`
	PaperCount  int64  `json:"paperCount"`
}

type Paper struct {
	ID             int64        `json:"id"`
	Title          string       `json:"title"`
	Authors        string       `json:"authors"`
	Year           int          `json:"year"`
	Venue          string       `json:"venue"`
	DOI            string       `json:"doi"`
	Keywords       string       `json:"keywords"`
	Link           string       `json:"link"`
	Summary        string       `json:"summary"`
	Notes          string       `json:"notes"`
	NotesUpdatedAt string       `json:"notesUpdatedAt"`
	DeletedAt      string       `json:"deletedAt,omitempty"`
	CategoryID     *int64       `json:"categoryId"`
	CategoryName   string       `json:"categoryName"`
	Read           bool         `json:"read"`
	Status         string       `json:"status"`
	Snippet        string       `json:"snippet,omitempty"`
	Starred        bool         `json:"starred"`
	HasPDF         bool         `json:"hasPdf"`
	PDFSize        int64        `json:"pdfSize"`
	PDFPath        string       `json:"pdfPath,omitempty"`
	FullText       string       `json:"fulltext,omitempty"`
	CreatedAt      time.Time    `json:"createdAt"`
	UpdatedAt      time.Time    `json:"updatedAt"`
	Tags           []Tag        `json:"tags"`
	Collections    []Collection `json:"collections"`
}

// Annotation 是全文阅读模式里的一处高亮标注。
// Start/End 是以 UTF-16 码元计的字符偏移（与浏览器 JS 字符串下标一致）。
type Annotation struct {
	ID        int64     `json:"id"`
	PaperID   int64     `json:"paperId"`
	Start     int       `json:"start"`
	End       int       `json:"end"`
	Quote     string    `json:"quote"`
	Color     string    `json:"color"`
	Note      string    `json:"note,omitempty"`
	CreatedAt time.Time `json:"createdAt"`
}

type PaperInput struct {
	Title           string   `json:"title"`
	Authors         string   `json:"authors"`
	Year            int      `json:"year"`
	Venue           string   `json:"venue"`
	DOI             string   `json:"doi"`
	Keywords        string   `json:"keywords"`
	Link            string   `json:"link"`
	Summary         string   `json:"summary"`
	Notes           string   `json:"notes"`
	CategoryID      *int64   `json:"categoryId"`
	Read            bool     `json:"read"`
	Status          string   `json:"status"`
	Force           bool     `json:"force"`
	UseAI           bool     `json:"useAI"`
	Starred         bool     `json:"starred"`
	Tags            []int64  `json:"tags"`
	Collections     []int64  `json:"collections"`
	TagNames        []string `json:"tagNames"`
	CollectionNames []string `json:"collectionNames"`
}

type PaperQuery struct {
	Search        string
	CategoryID    *int64
	TagIDs        []int64
	CollectionIDs []int64
	YearFrom      *int
	YearTo        *int
	Read          *bool
	Status        string
	Starred       *bool
	Sort          string
	Order         string
	Page          int
	PageSize      int
}

type DuplicateCheck struct {
	Duplicate bool   `json:"duplicate"`
	Reason    string `json:"reason"`
	PaperID   int64  `json:"paperId,omitempty"`
	Title     string `json:"title,omitempty"`
}

type ListResult struct {
	Total    int64   `json:"total"`
	Page     int     `json:"page"`
	PageSize int     `json:"pageSize"`
	Papers   []Paper `json:"papers"`
}
