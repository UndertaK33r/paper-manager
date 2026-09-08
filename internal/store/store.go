package store

import (
	"database/sql"
	"strconv"
	"strings"
	"time"

	"paper-manager/internal/models"

	_ "modernc.org/sqlite"
)

type Store struct {
	db *sql.DB
}

func Open(path string) (*Store, error) {
	dsn := path + "?_pragma=foreign_keys(1)&_pragma=busy_timeout(5000)&_pragma=journal_mode(WAL)"
	db, err := sql.Open("sqlite", dsn)
	if err != nil {
		return nil, err
	}
	db.SetMaxOpenConns(4)
	s := &Store{db: db}
	if err := s.init(); err != nil {
		db.Close()
		return nil, err
	}
	return s, nil
}

func (s *Store) Close() error { return s.db.Close() }

func (s *Store) HealthCheck() error { return s.db.Ping() }

func (s *Store) init() error {
	schema := `CREATE TABLE IF NOT EXISTS categories (
  id INTEGER PRIMARY KEY AUTOINCREMENT,
  name TEXT NOT NULL UNIQUE,
  created_at TEXT NOT NULL DEFAULT (strftime('%Y-%m-%dT%H:%M:%SZ','now'))
);
CREATE TABLE IF NOT EXISTS tags (
  id INTEGER PRIMARY KEY AUTOINCREMENT,
  name TEXT NOT NULL UNIQUE,
  created_at TEXT NOT NULL DEFAULT (strftime('%Y-%m-%dT%H:%M:%SZ','now'))
);
CREATE TABLE IF NOT EXISTS collections (
  id INTEGER PRIMARY KEY AUTOINCREMENT,
  name TEXT NOT NULL UNIQUE,
  description TEXT NOT NULL DEFAULT '',
  created_at TEXT NOT NULL DEFAULT (strftime('%Y-%m-%dT%H:%M:%SZ','now'))
);
CREATE TABLE IF NOT EXISTS papers (
  id INTEGER PRIMARY KEY AUTOINCREMENT,
  title TEXT NOT NULL,
  authors TEXT NOT NULL DEFAULT '',
  year INTEGER NOT NULL DEFAULT 0,
  venue TEXT NOT NULL DEFAULT '',
  doi TEXT NOT NULL DEFAULT '',
  keywords TEXT NOT NULL DEFAULT '',
  link TEXT NOT NULL DEFAULT '',
  summary TEXT NOT NULL DEFAULT '',
  notes TEXT NOT NULL DEFAULT '',
  category_id INTEGER REFERENCES categories(id) ON DELETE SET NULL,
  read INTEGER NOT NULL DEFAULT 0,
  status TEXT NOT NULL DEFAULT 'unread',
  starred INTEGER NOT NULL DEFAULT 0,
  pdf_path TEXT NOT NULL DEFAULT '',
  pdf_size INTEGER NOT NULL DEFAULT 0,
  created_at TEXT NOT NULL DEFAULT (strftime('%Y-%m-%dT%H:%M:%SZ','now')),
  updated_at TEXT NOT NULL DEFAULT (strftime('%Y-%m-%dT%H:%M:%SZ','now'))
);
CREATE TABLE IF NOT EXISTS paper_tags (
  paper_id INTEGER NOT NULL REFERENCES papers(id) ON DELETE CASCADE,
  tag_id INTEGER NOT NULL REFERENCES tags(id) ON DELETE CASCADE,
  PRIMARY KEY (paper_id, tag_id)
);
CREATE TABLE IF NOT EXISTS paper_collections (
  paper_id INTEGER NOT NULL REFERENCES papers(id) ON DELETE CASCADE,
  collection_id INTEGER NOT NULL REFERENCES collections(id) ON DELETE CASCADE,
  PRIMARY KEY (paper_id, collection_id)
);
CREATE INDEX IF NOT EXISTS idx_papers_title ON papers(title);
CREATE INDEX IF NOT EXISTS idx_papers_year ON papers(year);
CREATE INDEX IF NOT EXISTS idx_papers_category ON papers(category_id);
CREATE TABLE IF NOT EXISTS settings (
  key TEXT PRIMARY KEY,
  value TEXT NOT NULL DEFAULT '',
  updated_at TEXT NOT NULL DEFAULT (strftime('%Y-%m-%dT%H:%M:%SZ','now'))
);
CREATE INDEX IF NOT EXISTS idx_papers_doi ON papers(doi);
`
	_, err := s.db.Exec(schema)
	if err != nil {
		return err
	}
	_, _ = s.db.Exec("ALTER TABLE papers ADD COLUMN fulltext TEXT NOT NULL DEFAULT ''")
	_, _ = s.db.Exec("ALTER TABLE papers ADD COLUMN status TEXT NOT NULL DEFAULT 'unread'")
	_, _ = s.db.Exec("ALTER TABLE papers ADD COLUMN translation TEXT NOT NULL DEFAULT ''")
	_, _ = s.db.Exec("UPDATE papers SET status = 'read' WHERE read = 1 AND status = 'unread'")
	return nil
}

func nowStr() string { return time.Now().UTC().Format(time.RFC3339) }

func parseTime(v string) time.Time {
	t, err := time.Parse(time.RFC3339, v)
	if err != nil {
		if t2, e2 := time.Parse("2006-01-02 15:04:05", v); e2 == nil {
			return t2
		}
		return time.Time{}
	}
	return t
}

func boolInt(b bool) int {
	if b {
		return 1
	}
	return 0
}

func placeholders(n int) string {
	parts := make([]string, n)
	for i := range parts {
		parts[i] = "?"
	}
	return strings.Join(parts, ",")
}

func sortClause(sort, order string) string {
	col := "p.created_at"
	switch sort {
	case "title":
		col = "p.title COLLATE NOCASE"
	case "year":
		col = "p.year"
	case "authors":
		col = "p.authors COLLATE NOCASE"
	case "created":
		col = "p.created_at"
	case "updated":
		col = "p.updated_at"
	}
	if order != "asc" {
		order = "desc"
	}
	return " ORDER BY " + col + " " + strings.ToUpper(order)
}

func parseIntPtr(s string) *int64 {
	if s == "" {
		return nil
	}
	v, err := strconv.ParseInt(s, 10, 64)
	if err != nil {
		return nil
	}
	return &v
}

func parseIDList(s string) []int64 {
	if s == "" {
		return nil
	}
	parts := strings.Split(s, ",")
	out := []int64{}
	for _, p := range parts {
		p = strings.TrimSpace(p)
		if p == "" {
			continue
		}
		v, err := strconv.ParseInt(p, 10, 64)
		if err == nil {
			out = append(out, v)
		}
	}
	return out
}

func normalizeTitle(t string) string {
	t = strings.ToLower(t)
	var b strings.Builder
	for _, r := range t {
		if (r >= 'a' && r <= 'z') || (r >= '0' && r <= '9') || (r >= 0x4e00 && r <= 0x9fff) {
			b.WriteRune(r)
		} else if r == ' ' {
			if b.Len() > 0 && b.String()[b.Len()-1] != ' ' {
				b.WriteByte(' ')
			}
		}
	}
	return strings.TrimSpace(b.String())
}

func firstAuthor(a string) string {
	a = strings.TrimSpace(a)
	if a == "" {
		return ""
	}
	parts := strings.Split(a, ",")
	if len(parts) > 0 {
		return strings.TrimSpace(parts[0])
	}
	return normalizeTitle(a)
}

func (s *Store) CreatePaper(p *models.Paper) (int64, error) {
	if p.Status == "" {
		p.Status = "unread"
	}
	now := nowStr()
	res, err := s.db.Exec(`INSERT INTO papers
		(title, authors, year, venue, doi, keywords, link, summary, notes, category_id, read, status, starred, pdf_path, pdf_size, fulltext, created_at, updated_at)
		VALUES (?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?)`,
		p.Title, p.Authors, p.Year, p.Venue, p.DOI, p.Keywords, p.Link, p.Summary, p.Notes,
		p.CategoryID, boolInt(p.Read), p.Status, boolInt(p.Starred), p.PDFPath, p.PDFSize, p.FullText, now, now)
	if err != nil {
		return 0, err
	}
	return res.LastInsertId()
}

func (s *Store) UpdatePaper(p *models.Paper) error {
	if p.Status == "" {
		p.Status = "unread"
	}
	_, err := s.db.Exec(`UPDATE papers SET
		title=?, authors=?, year=?, venue=?, doi=?, keywords=?, link=?, summary=?, notes=?,
		category_id=?, read=?, status=?, starred=?, pdf_path=?, pdf_size=?, fulltext=?, updated_at=?
		WHERE id=?`,
		p.Title, p.Authors, p.Year, p.Venue, p.DOI, p.Keywords, p.Link, p.Summary, p.Notes,
		p.CategoryID, boolInt(p.Read), p.Status, boolInt(p.Starred), p.PDFPath, p.PDFSize, p.FullText, nowStr(), p.ID)
	return err
}

func (s *Store) DeletePaper(id int64) (string, error) {
	var pdfPath string
	err := s.db.QueryRow("SELECT pdf_path FROM papers WHERE id = ?", id).Scan(&pdfPath)
	if err == sql.ErrNoRows {
		return "", nil
	}
	if err != nil {
		return "", err
	}
	_, err = s.db.Exec("DELETE FROM papers WHERE id = ?", id)
	return pdfPath, err
}

func (s *Store) SetPaperCategory(id int64, categoryID *int64) error {
	_, err := s.db.Exec("UPDATE papers SET category_id = ?, updated_at = ? WHERE id = ?", categoryID, nowStr(), id)
	return err
}

func (s *Store) SetPaperRead(id int64, read bool) error {
	status := "unread"
	if read {
		status = "read"
	}
	_, err := s.db.Exec("UPDATE papers SET read = ?, status = ?, updated_at = ? WHERE id = ?", boolInt(read), status, nowStr(), id)
	return err
}

func (s *Store) SetPaperStatus(id int64, status string) error {
	if status != "unread" && status != "reading" && status != "read" {
		status = "unread"
	}
	_, err := s.db.Exec("UPDATE papers SET status = ?, read = ?, updated_at = ? WHERE id = ?", status, boolInt(status == "read"), nowStr(), id)
	return err
}

func (s *Store) SetPaperStarred(id int64, starred bool) error {
	_, err := s.db.Exec("UPDATE papers SET starred = ?, updated_at = ? WHERE id = ?", boolInt(starred), nowStr(), id)
	return err
}

func (s *Store) SetPaperTags(paperID int64, tagIDs []int64) error {
	if _, err := s.db.Exec("DELETE FROM paper_tags WHERE paper_id = ?", paperID); err != nil {
		return err
	}
	for _, tid := range tagIDs {
		if _, err := s.db.Exec("INSERT OR IGNORE INTO paper_tags (paper_id, tag_id) VALUES (?,?)", paperID, tid); err != nil {
			return err
		}
	}
	return nil
}

func (s *Store) AddPaperTag(paperID, tagID int64) error {
	_, err := s.db.Exec("INSERT OR IGNORE INTO paper_tags (paper_id, tag_id) VALUES (?,?)", paperID, tagID)
	return err
}

func (s *Store) RemovePaperTag(paperID, tagID int64) error {
	_, err := s.db.Exec("DELETE FROM paper_tags WHERE paper_id = ? AND tag_id = ?", paperID, tagID)
	return err
}

func (s *Store) SetPaperCollections(paperID int64, collectionIDs []int64) error {
	if _, err := s.db.Exec("DELETE FROM paper_collections WHERE paper_id = ?", paperID); err != nil {
		return err
	}
	for _, cid := range collectionIDs {
		if _, err := s.db.Exec("INSERT OR IGNORE INTO paper_collections (paper_id, collection_id) VALUES (?,?)", paperID, cid); err != nil {
			return err
		}
	}
	return nil
}

func (s *Store) AddPaperCollection(paperID, collectionID int64) error {
	_, err := s.db.Exec("INSERT OR IGNORE INTO paper_collections (paper_id, collection_id) VALUES (?,?)", paperID, collectionID)
	return err
}

func (s *Store) RemovePaperCollection(paperID, collectionID int64) error {
	_, err := s.db.Exec("DELETE FROM paper_collections WHERE paper_id = ? AND collection_id = ?", paperID, collectionID)
	return err
}

func (s *Store) PaperPDFPath(id int64) (string, int64, error) {
	var path string
	var size int64
	err := s.db.QueryRow("SELECT pdf_path, pdf_size FROM papers WHERE id = ?", id).Scan(&path, &size)
	return path, size, err
}
