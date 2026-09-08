package store

import (
	"database/sql"

	"paper-manager/internal/models"
)

type paperExecer interface {
	Exec(string, ...any) (sql.Result, error)
}

func insertPaper(db paperExecer, p *models.Paper) (int64, error) {
	if p.Status == "" {
		p.Status = "unread"
	}
	now := nowStr()
	res, err := db.Exec(`INSERT INTO papers
(title, authors, year, venue, doi, keywords, link, summary, notes, category_id, read, status, starred, pdf_path, pdf_size, fulltext, created_at, updated_at)
VALUES (?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?)`,
		p.Title, p.Authors, p.Year, p.Venue, p.DOI, p.Keywords, p.Link, p.Summary, p.Notes,
		p.CategoryID, boolInt(p.Read), p.Status, boolInt(p.Starred), p.PDFPath, p.PDFSize, p.FullText, now, now)
	if err != nil {
		return 0, err
	}
	return res.LastInsertId()
}

func updatePaper(db paperExecer, p *models.Paper) error {
	if p.Status == "" {
		p.Status = "unread"
	}
	res, err := db.Exec(`UPDATE papers SET
 title=?, authors=?, year=?, venue=?, doi=?, keywords=?, link=?, summary=?, notes=?,
 category_id=?, read=?, status=?, starred=?, pdf_path=?, pdf_size=?, fulltext=?, updated_at=?
 WHERE id=? AND (notes_updated_at=? OR notes=?)`,
		p.Title, p.Authors, p.Year, p.Venue, p.DOI, p.Keywords, p.Link, p.Summary, p.Notes,
		p.CategoryID, boolInt(p.Read), p.Status, boolInt(p.Starred), p.PDFPath, p.PDFSize, p.FullText, nowStr(), p.ID, p.NotesUpdatedAt, p.Notes)
	if err != nil {
		return err
	}
	n, err := res.RowsAffected()
	if err != nil {
		return err
	}
	if n == 0 {
		return ErrConflict
	}
	return nil
}

// SavePaper commits the record and both sets of relations as one unit.
func (s *Store) SavePaper(p *models.Paper, tags, collections []int64, create bool) (int64, error) {
	tx, err := s.db.Begin()
	if err != nil {
		return 0, err
	}
	defer tx.Rollback()
	id := p.ID
	if create {
		id, err = insertPaper(tx, p)
	} else {
		err = updatePaper(tx, p)
	}
	if err != nil {
		return 0, err
	}
	if err := replaceRelations(tx, "paper_tags", "tag_id", id, tags); err != nil {
		return 0, err
	}
	if err := replaceRelations(tx, "paper_collections", "collection_id", id, collections); err != nil {
		return 0, err
	}
	if err := tx.Commit(); err != nil {
		return 0, err
	}
	return id, nil
}

// Metadata extraction must not write back notes/status captured before a slow AI call.
func (s *Store) UpdateMetadata(p *models.Paper, includeText bool) error {
	fields := map[string]any{"title": p.Title, "authors": p.Authors, "year": p.Year, "venue": p.Venue, "doi": p.DOI, "keywords": p.Keywords, "link": p.Link, "summary": p.Summary, "category_id": p.CategoryID}
	if includeText {
		fields["fulltext"] = p.FullText
	}
	return s.UpdateFields(p.ID, fields, nil, nil, nil)
}
