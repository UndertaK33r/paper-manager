package store

import (
	"database/sql"
	"strings"

	"paper-manager/internal/models"
)

const paperCols = `p.id, p.title, p.authors, p.year, p.venue, p.doi, p.keywords, p.link,
 p.summary, p.notes, p.category_id, c.name, p.read, p.status, p.starred,
 CASE WHEN p.pdf_path = '' THEN 0 ELSE 1 END, p.pdf_size, p.pdf_path, p.created_at, p.updated_at, p.notes_updated_at`

type rowScanner interface{ Scan(dest ...any) error }

func scanPaper(scanner rowScanner) (models.Paper, error) {
	var p models.Paper
	var read, starred, hasPDF int
	var created, updated string
	var categoryID sql.NullInt64
	var categoryName sql.NullString
	err := scanner.Scan(&p.ID, &p.Title, &p.Authors, &p.Year, &p.Venue, &p.DOI, &p.Keywords, &p.Link,
		&p.Summary, &p.Notes, &categoryID, &categoryName, &read, &p.Status, &starred, &hasPDF, &p.PDFSize, &p.PDFPath, &created, &updated, &p.NotesUpdatedAt)
	if err != nil {
		return p, err
	}
	if categoryID.Valid {
		id := categoryID.Int64
		p.CategoryID = &id
	}
	p.Read = read != 0 || p.Status == "read"
	p.Starred = starred != 0
	p.HasPDF = hasPDF != 0
	p.CategoryName = categoryName.String
	p.CreatedAt = parseTime(created)
	p.UpdatedAt = parseTime(updated)
	p.Tags = []models.Tag{}
	p.Collections = []models.Collection{}
	return p, nil
}

func (s *Store) loadRelations(paperID int64) ([]models.Tag, []models.Collection, error) {
	tags, err := s.PaperTags(paperID)
	if err != nil {
		return nil, nil, err
	}
	cols, err := s.PaperCollections(paperID)
	if err != nil {
		return nil, nil, err
	}
	return tags, cols, nil
}

func buildWhere(q models.PaperQuery) (string, []any) {
	conds := []string{}
	args := []any{}
	if q.Search != "" {
		like := "%" + strings.ToLower(q.Search) + "%"
		conds = append(conds, `(lower(p.title) LIKE ? OR lower(p.authors) LIKE ? OR lower(p.venue) LIKE ? OR lower(p.doi) LIKE ? OR lower(p.keywords) LIKE ? OR lower(p.fulltext) LIKE ?)`)
		for i := 0; i < 6; i++ {
			args = append(args, like)
		}
	}
	if q.CategoryID != nil {
		conds = append(conds, "p.category_id = ?")
		args = append(args, *q.CategoryID)
	}
	if len(q.TagIDs) > 0 {
		ph := placeholders(len(q.TagIDs))
		conds = append(conds, "EXISTS (SELECT 1 FROM paper_tags pt WHERE pt.paper_id = p.id AND pt.tag_id IN ("+ph+"))")
		for _, id := range q.TagIDs {
			args = append(args, id)
		}
	}
	if len(q.CollectionIDs) > 0 {
		ph := placeholders(len(q.CollectionIDs))
		conds = append(conds, "EXISTS (SELECT 1 FROM paper_collections pc WHERE pc.paper_id = p.id AND pc.collection_id IN ("+ph+"))")
		for _, id := range q.CollectionIDs {
			args = append(args, id)
		}
	}
	if q.YearFrom != nil {
		conds = append(conds, "p.year >= ?")
		args = append(args, *q.YearFrom)
	}
	if q.YearTo != nil {
		conds = append(conds, "p.year <= ?")
		args = append(args, *q.YearTo)
	}
	if q.Read != nil {
		if *q.Read {
			conds = append(conds, "p.status = 'read'")
		} else {
			conds = append(conds, "p.status != 'read'")
		}
	}
	if q.Status != "" && q.Status != "all" {
		conds = append(conds, "p.status = ?")
		args = append(args, q.Status)
	}
	if q.Starred != nil {
		conds = append(conds, "p.starred = ?")
		if *q.Starred {
			args = append(args, 1)
		} else {
			args = append(args, 0)
		}
	}
	if len(conds) == 0 {
		return "", args
	}
	return " WHERE " + strings.Join(conds, " AND "), args
}

func (s *Store) ListPapers(q models.PaperQuery) (models.ListResult, error) {
	where, args := buildWhere(q)
	var total int64
	if err := s.db.QueryRow("SELECT COUNT(*) FROM papers p"+where, args...).Scan(&total); err != nil {
		return models.ListResult{}, err
	}
	page := q.Page
	if page < 1 {
		page = 1
	}
	size := q.PageSize
	if size < 1 {
		size = 20
	}
	if size > 100 {
		size = 100
	}
	order := sortClause(q.Sort, q.Order)
	offset := (page - 1) * size
	sqlStr := "SELECT " + paperCols + " FROM papers p LEFT JOIN categories c ON c.id = p.category_id" + where + order + " LIMIT ? OFFSET ?"
	rows, err := s.db.Query(sqlStr, append(args, size, offset)...)
	if err != nil {
		return models.ListResult{}, err
	}
	defer rows.Close()
	papers := make([]models.Paper, 0)
	for rows.Next() {
		p, err := scanPaper(rows)
		if err != nil {
			return models.ListResult{}, err
		}
		tags, cols, err := s.loadRelations(p.ID)
		if err != nil {
			return models.ListResult{}, err
		}
		p.Tags = tags
		p.Collections = cols
		if q.Search != "" {
			p.Snippet = buildSnippet(s.PaperFullText(p.ID), q.Search, 160)
		}
		papers = append(papers, p)
	}
	return models.ListResult{Total: total, Page: page, PageSize: size, Papers: papers}, rows.Err()
}

func (s *Store) GetPaper(id int64) (models.Paper, error) {
	row := s.db.QueryRow("SELECT "+paperCols+" FROM papers p LEFT JOIN categories c ON c.id = p.category_id WHERE p.id = ?", id)
	p, err := scanPaper(row)
	if err != nil {
		return p, err
	}
	p.Tags, p.Collections, err = s.loadRelations(id)
	return p, err
}

func (s *Store) PaperTags(paperID int64) ([]models.Tag, error) {
	rows, err := s.db.Query(`SELECT t.id, t.name, (SELECT COUNT(*) FROM paper_tags WHERE tag_id = t.id)
		FROM tags t JOIN paper_tags pt ON pt.tag_id = t.id WHERE pt.paper_id = ? ORDER BY t.name`, paperID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []models.Tag{}
	for rows.Next() {
		var t models.Tag
		if err := rows.Scan(&t.ID, &t.Name, &t.PaperCount); err != nil {
			return nil, err
		}
		out = append(out, t)
	}
	return out, rows.Err()
}

func (s *Store) PaperCollections(paperID int64) ([]models.Collection, error) {
	rows, err := s.db.Query(`SELECT c.id, c.name, c.description, (SELECT COUNT(*) FROM paper_collections WHERE collection_id = c.id)
		FROM collections c JOIN paper_collections pc ON pc.collection_id = c.id WHERE pc.paper_id = ? ORDER BY c.name`, paperID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []models.Collection{}
	for rows.Next() {
		var c models.Collection
		if err := rows.Scan(&c.ID, &c.Name, &c.Description, &c.PaperCount); err != nil {
			return nil, err
		}
		out = append(out, c)
	}
	return out, rows.Err()
}

func (s *Store) FindDuplicate(title, authors, doi string) (*models.Paper, error) {
	if strings.TrimSpace(doi) != "" {
		var p models.Paper
		var read, starred, hasPDF int
		var created, updated string
		var categoryID sql.NullInt64
		var categoryName sql.NullString
		err := s.db.QueryRow("SELECT "+paperCols+" FROM papers p LEFT JOIN categories c ON c.id = p.category_id WHERE lower(p.doi) = lower(?)", strings.TrimSpace(doi)).
			Scan(&p.ID, &p.Title, &p.Authors, &p.Year, &p.Venue, &p.DOI, &p.Keywords, &p.Link,
				&p.Summary, &p.Notes, &categoryID, &categoryName, &read, &p.Status, &starred, &hasPDF, &p.PDFSize, &p.PDFPath, &created, &updated, &p.NotesUpdatedAt)
		if err == nil {
			if categoryID.Valid {
				id := categoryID.Int64
				p.CategoryID = &id
			}
			p.CategoryName = categoryName.String
			p.Read = read != 0 || p.Status == "read"
			p.Starred = starred != 0
			p.HasPDF = hasPDF != 0
			p.CreatedAt = parseTime(created)
			p.UpdatedAt = parseTime(updated)
			p.Tags = []models.Tag{}
			p.Collections = []models.Collection{}
			return &p, nil
		}
		if err != sql.ErrNoRows {
			return nil, err
		}
	}
	normTitle := normalizeTitle(title)
	firstAuth := firstAuthor(authors)
	if normTitle == "" || firstAuth == "" {
		return nil, nil
	}
	rows, err := s.db.Query("SELECT p.id, p.title, p.authors FROM papers p")
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	for rows.Next() {
		var id int64
		var t, a string
		if err := rows.Scan(&id, &t, &a); err != nil {
			return nil, err
		}
		if normalizeTitle(t) == normTitle && firstAuthor(a) == firstAuth {
			p, err := s.GetPaper(id)
			if err != nil {
				return nil, err
			}
			return &p, nil
		}
	}
	return nil, rows.Err()
}

func (s *Store) PaperFullText(id int64) string {
	var v string
	err := s.db.QueryRow("SELECT fulltext FROM papers WHERE id = ?", id).Scan(&v)
	if err != nil {
		return ""
	}
	return v
}

// SearchRelaxed 任意 token 命中即返回（OR 检索，供 AI 问答召回使用）。
func (s *Store) SearchRelaxed(query string, limit int) ([]models.Paper, error) {
	tokens := queryTokens(query)
	if len(tokens) == 0 {
		return nil, nil
	}
	if limit <= 0 || limit > 10 {
		limit = 3
	}
	conds := []string{}
	args := []any{}
	for _, t := range tokens {
		like := "%" + t + "%"
		conds = append(conds, "(lower(p.title) LIKE ? OR lower(p.authors) LIKE ? OR lower(p.keywords) LIKE ? OR lower(p.summary) LIKE ? OR lower(p.fulltext) LIKE ?)")
		for i := 0; i < 5; i++ {
			args = append(args, like)
		}
	}
	where := " WHERE " + strings.Join(conds, " OR ")
	sqlStr := "SELECT " + paperCols + " FROM papers p LEFT JOIN categories c ON c.id = p.category_id" + where + " ORDER BY p.year DESC, p.id DESC LIMIT ?"
	args = append(args, limit)
	rows, err := s.db.Query(sqlStr, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []models.Paper{}
	for rows.Next() {
		p, err := scanPaper(rows)
		if err != nil {
			return nil, err
		}
		p.Tags = []models.Tag{}
		p.Collections = []models.Collection{}
		out = append(out, p)
	}
	return out, rows.Err()
}

// GetTranslation 读取论文的 AI 中文译文（独立于 paperCols，避免进列表查询）。
func (s *Store) GetTranslation(id int64) (string, error) {
	var v string
	err := s.db.QueryRow("SELECT translation FROM papers WHERE id = ?", id).Scan(&v)
	if err != nil {
		return "", err
	}
	return v, nil
}

// UpdateTranslation 保存译文（只更新该列，不影响其他字段）。
func (s *Store) UpdateTranslation(id int64, text string) error {
	_, err := s.db.Exec("UPDATE papers SET translation = ? WHERE id = ?", text, id)
	return err
}

// NoteRow 是一条非空笔记的导出行。
type NoteRow struct {
	ID      int64
	Title   string
	Authors string
	Notes   string
	Updated string
}

// AllNotes 返回所有非空笔记（按修改时间倒序），用于笔记导出。
func (s *Store) AllNotes() ([]NoteRow, error) {
	rows, err := s.db.Query(`SELECT id, title, authors, notes, updated_at
		FROM papers WHERE TRIM(notes) != '' ORDER BY updated_at DESC`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []NoteRow{}
	for rows.Next() {
		var r NoteRow
		if err := rows.Scan(&r.ID, &r.Title, &r.Authors, &r.Notes, &r.Updated); err != nil {
			return nil, err
		}
		out = append(out, r)
	}
	return out, rows.Err()
}
