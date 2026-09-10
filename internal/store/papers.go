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

// scanPaperWithDeleted 与 scanPaper 相同，额外读取 p.deleted_at（回收站列表用）。
func scanPaperWithDeleted(scanner rowScanner) (models.Paper, error) {
	var p models.Paper
	var read, starred, hasPDF int
	var created, updated string
	var categoryID sql.NullInt64
	var categoryName sql.NullString
	err := scanner.Scan(&p.ID, &p.Title, &p.Authors, &p.Year, &p.Venue, &p.DOI, &p.Keywords, &p.Link,
		&p.Summary, &p.Notes, &categoryID, &categoryName, &read, &p.Status, &starred, &hasPDF, &p.PDFSize, &p.PDFPath, &created, &updated, &p.NotesUpdatedAt, &p.DeletedAt)
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

// searchCond 生成一条子串搜索条件（title/authors/venue/doi/keywords/summary/fulltext）。
// 曾尝试用 FTS5(trigram) 替代，实测在 2 万篇规模下常见词反而更慢且索引体积翻倍，故保留 LIKE，见 ROADMAP。
func searchCond(raw string) (cond string, args []any, ok bool) {
	q := strings.TrimSpace(raw)
	if q == "" {
		return "", nil, false
	}
	like := "%" + strings.ToLower(q) + "%"
	return `(lower(p.title) LIKE ? OR lower(p.authors) LIKE ? OR lower(p.venue) LIKE ? OR lower(p.doi) LIKE ? OR lower(p.keywords) LIKE ? OR lower(p.summary) LIKE ? OR lower(p.fulltext) LIKE ?)`,
		[]any{like, like, like, like, like, like, like}, true
}

func buildWhere(q models.PaperQuery) (string, []any) {
	// 软删除的论文不出现在任何常规列表/统计里
	conds := []string{"p.deleted_at = ''"}
	args := []any{}
	if q.Search != "" {
		if cond, sargs, ok := searchCond(q.Search); ok {
			conds = append(conds, cond)
			args = append(args, sargs...)
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
	if len(q.TagNames) > 0 {
		ph := placeholders(len(q.TagNames))
		conds = append(conds, "EXISTS (SELECT 1 FROM paper_tags pt JOIN tags t ON t.id = pt.tag_id WHERE pt.paper_id = p.id AND t.name IN ("+ph+"))")
		for _, n := range q.TagNames {
			args = append(args, n)
		}
	}
	if len(q.CollectionNames) > 0 {
		ph := placeholders(len(q.CollectionNames))
		conds = append(conds, "EXISTS (SELECT 1 FROM paper_collections pc JOIN collections co ON co.id = pc.collection_id WHERE pc.paper_id = p.id AND co.name IN ("+ph+"))")
		for _, n := range q.CollectionNames {
			args = append(args, n)
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
	papers := make([]models.Paper, 0, size)
	ids := make([]int64, 0, size)
	for rows.Next() {
		p, err := scanPaper(rows)
		if err != nil {
			return models.ListResult{}, err
		}
		papers = append(papers, p)
		ids = append(ids, p.ID)
	}
	if err := rows.Err(); err != nil {
		return models.ListResult{}, err
	}
	// 关联与搜索片段批量取：此前每行各查 2~3 次（20 条 = 40+ 次往返）
	tagsByID, colsByID, err := s.loadRelationsBatch(ids)
	if err != nil {
		return models.ListResult{}, err
	}
	var texts map[int64]string
	if q.Search != "" {
		if texts, err = s.paperFullTexts(ids); err != nil {
			return models.ListResult{}, err
		}
	}
	for i := range papers {
		if t, ok := tagsByID[papers[i].ID]; ok {
			papers[i].Tags = t
		}
		if c, ok := colsByID[papers[i].ID]; ok {
			papers[i].Collections = c
		}
		if q.Search != "" {
			papers[i].Snippet = buildSnippet(texts[papers[i].ID], q.Search, 160)
		}
	}
	return models.ListResult{Total: total, Page: page, PageSize: size, Papers: papers}, nil
}

// loadRelationsBatch 一次取回多篇论文的标签与合集，避免列表页 N+1 查询。
func (s *Store) loadRelationsBatch(ids []int64) (map[int64][]models.Tag, map[int64][]models.Collection, error) {
	tags := make(map[int64][]models.Tag, len(ids))
	cols := make(map[int64][]models.Collection, len(ids))
	if len(ids) == 0 {
		return tags, cols, nil
	}
	args := make([]any, len(ids))
	for i, id := range ids {
		args[i] = id
	}
	ph := placeholders(len(ids))

	trows, err := s.db.Query(`SELECT pt.paper_id, t.id, t.name, (SELECT COUNT(*) FROM paper_tags pt2 JOIN papers p2 ON p2.id = pt2.paper_id WHERE pt2.tag_id = t.id AND p2.deleted_at = '')
		FROM tags t JOIN paper_tags pt ON pt.tag_id = t.id WHERE pt.paper_id IN (`+ph+`) ORDER BY t.name`, args...)
	if err != nil {
		return nil, nil, err
	}
	for trows.Next() {
		var pid int64
		var t models.Tag
		if err := trows.Scan(&pid, &t.ID, &t.Name, &t.PaperCount); err != nil {
			trows.Close()
			return nil, nil, err
		}
		tags[pid] = append(tags[pid], t)
	}
	err = trows.Err()
	trows.Close()
	if err != nil {
		return nil, nil, err
	}

	crows, err := s.db.Query(`SELECT pc.paper_id, c.id, c.name, c.description, (SELECT COUNT(*) FROM paper_collections pc2 JOIN papers p2 ON p2.id = pc2.paper_id WHERE pc2.collection_id = c.id AND p2.deleted_at = '')
		FROM collections c JOIN paper_collections pc ON pc.collection_id = c.id WHERE pc.paper_id IN (`+ph+`) ORDER BY c.name`, args...)
	if err != nil {
		return nil, nil, err
	}
	for crows.Next() {
		var pid int64
		var c models.Collection
		if err := crows.Scan(&pid, &c.ID, &c.Name, &c.Description, &c.PaperCount); err != nil {
			crows.Close()
			return nil, nil, err
		}
		cols[pid] = append(cols[pid], c)
	}
	err = crows.Err()
	crows.Close()
	return tags, cols, err
}

// paperFullTexts 批量取全文，供搜索片段使用（避免逐行查询）。
func (s *Store) paperFullTexts(ids []int64) (map[int64]string, error) {
	out := make(map[int64]string, len(ids))
	if len(ids) == 0 {
		return out, nil
	}
	args := make([]any, len(ids))
	for i, id := range ids {
		args[i] = id
	}
	rows, err := s.db.Query("SELECT id, fulltext FROM papers WHERE id IN ("+placeholders(len(ids))+")", args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	for rows.Next() {
		var id int64
		var text string
		if err := rows.Scan(&id, &text); err != nil {
			return nil, err
		}
		out[id] = text
	}
	return out, rows.Err()
}

func (s *Store) GetPaper(id int64) (models.Paper, error) {
	row := s.db.QueryRow("SELECT "+paperCols+" FROM papers p LEFT JOIN categories c ON c.id = p.category_id WHERE p.id = ? AND p.deleted_at = ''", id)
	p, err := scanPaper(row)
	if err != nil {
		return p, err
	}
	p.Tags, p.Collections, err = s.loadRelations(id)
	return p, err
}

func (s *Store) PaperTags(paperID int64) ([]models.Tag, error) {
	rows, err := s.db.Query(`SELECT t.id, t.name, (SELECT COUNT(*) FROM paper_tags pt2 JOIN papers p2 ON p2.id = pt2.paper_id WHERE pt2.tag_id = t.id AND p2.deleted_at = '')
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
	rows, err := s.db.Query(`SELECT c.id, c.name, c.description, (SELECT COUNT(*) FROM paper_collections pc2 JOIN papers p2 ON p2.id = pc2.paper_id WHERE pc2.collection_id = c.id AND p2.deleted_at = '')
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
		err := s.db.QueryRow("SELECT "+paperCols+" FROM papers p LEFT JOIN categories c ON c.id = p.category_id WHERE lower(p.doi) = lower(?) AND p.deleted_at = ''", strings.TrimSpace(doi)).
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
	rows, err := s.db.Query("SELECT p.id, p.title, p.authors FROM papers p WHERE p.deleted_at = ''")
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
		if cond, sargs, ok := searchCond(t); ok {
			conds = append(conds, cond)
			args = append(args, sargs...)
		}
	}
	where := " WHERE p.deleted_at = '' AND (" + strings.Join(conds, " OR ") + ")"
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
		FROM papers WHERE TRIM(notes) != '' AND deleted_at = '' ORDER BY updated_at DESC`)
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

// ---------- 软删除与回收站 ----------
// 删除默认只做软删除（保留 PDF 与关联），避免误删不可逆；彻底删除必须显式调用 Purge。

// DeletePaper 移入回收站（软删除）。返回空字符串以兼容旧调用方。
func (s *Store) DeletePaper(id int64) (string, error) {
	res, err := s.db.Exec("UPDATE papers SET deleted_at = ?, updated_at = ? WHERE id = ? AND deleted_at = ''", nowStr(), nowStr(), id)
	if err != nil {
		return "", err
	}
	if n, err := res.RowsAffected(); err == nil && n == 0 {
		return "", ErrConflict
	}
	return "", nil
}

// RestorePaper 从回收站恢复。
func (s *Store) RestorePaper(id int64) error {
	res, err := s.db.Exec("UPDATE papers SET deleted_at = '', updated_at = ? WHERE id = ? AND deleted_at != ''", nowStr(), id)
	if err != nil {
		return err
	}
	if n, err := res.RowsAffected(); err == nil && n == 0 {
		return ErrConflict
	}
	return nil
}

// PurgePaper 彻底删除：返回需要删除的 PDF 文件名（由调用方删文件）。
func (s *Store) PurgePaper(id int64) (string, error) {
	var pdfPath string
	if err := s.db.QueryRow("SELECT pdf_path FROM papers WHERE id = ? AND deleted_at != ''", id).Scan(&pdfPath); err != nil {
		if err == sql.ErrNoRows {
			return "", ErrConflict
		}
		return "", err
	}
	if _, err := s.db.Exec("DELETE FROM papers WHERE id = ?", id); err != nil {
		return "", err
	}
	return pdfPath, nil
}

// ListTrash 列出回收站中的论文（按删除时间倒序）。
func (s *Store) ListTrash() ([]models.Paper, error) {
	rows, err := s.db.Query("SELECT " + paperCols + ", p.deleted_at FROM papers p LEFT JOIN categories c ON c.id = p.category_id WHERE p.deleted_at != '' ORDER BY p.deleted_at DESC")
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []models.Paper{}
	for rows.Next() {
		p, err := scanPaperWithDeleted(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, p)
	}
	return out, rows.Err()
}

// EmptyTrash 清空回收站，返回全部待删除的 PDF 文件名。
func (s *Store) EmptyTrash() ([]string, error) {
	rows, err := s.db.Query("SELECT pdf_path FROM papers WHERE deleted_at != '' AND pdf_path != ''")
	if err != nil {
		return nil, err
	}
	paths := []string{}
	for rows.Next() {
		var p string
		if err := rows.Scan(&p); err != nil {
			rows.Close()
			return nil, err
		}
		paths = append(paths, p)
	}
	rows.Close()
	if err := rows.Err(); err != nil {
		return nil, err
	}
	if _, err := s.db.Exec("DELETE FROM papers WHERE deleted_at != ''"); err != nil {
		return nil, err
	}
	return paths, nil
}
