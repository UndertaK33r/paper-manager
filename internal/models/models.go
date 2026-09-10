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

// Annotation 是 PDF 上的一处划段标注。
// 位置用「页面 + 归一化矩形」表示（0..1 相对页面宽高），与栏数、缩放、列宽无关：
// 双栏论文里选区本身就是视觉范围，不依赖文本抽取顺序。
type Annotation struct {
	ID        int64      `json:"id"`
	PaperID   int64      `json:"paperId"`
	Page      int        `json:"page"`  // 起始页（1 起）
	Rects     []AnnoRect `json:"rects"` // 覆盖的矩形（可能跨页/跨行）
	Quote     string     `json:"quote"`
	Color     string     `json:"color"`
	Note      string     `json:"note,omitempty"`
	CreatedAt time.Time  `json:"createdAt"`
}

// AnnoRect 是标注在某一页上的一个矩形（归一化坐标，相对页面左上角）
type AnnoRect struct {
	Page int     `json:"p"`
	X    float64 `json:"x"`
	Y    float64 `json:"y"`
	W    float64 `json:"w"`
	H    float64 `json:"h"`
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
