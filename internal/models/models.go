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
	ID           int64        `json:"id"`
	Title        string       `json:"title"`
	Authors      string       `json:"authors"`
	Year         int          `json:"year"`
	Venue        string       `json:"venue"`
	DOI          string       `json:"doi"`
	Keywords     string       `json:"keywords"`
	Link         string       `json:"link"`
	Summary      string       `json:"summary"`
	Notes        string       `json:"notes"`
	CategoryID   *int64       `json:"categoryId"`
	CategoryName string       `json:"categoryName"`
	Read         bool         `json:"read"`
	Starred      bool         `json:"starred"`
	HasPDF       bool         `json:"hasPdf"`
	PDFSize      int64        `json:"pdfSize"`
	PDFPath      string       `json:"pdfPath,omitempty"`
	FullText     string       `json:"fulltext,omitempty"`
	CreatedAt    time.Time    `json:"createdAt"`
	UpdatedAt    time.Time    `json:"updatedAt"`
	Tags         []Tag        `json:"tags"`
	Collections  []Collection `json:"collections"`
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
