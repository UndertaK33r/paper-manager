package store

import (
	"database/sql"
	"errors"
	"fmt"
	"sort"
	"strings"
)

var ErrConflict = errors.New("笔记已被其他窗口修改，请刷新后合并，未覆盖已有笔记")

// migrateNotes preserves the best available timestamp for legacy notes once.
// Triggers cover every write path, including older clients using PUT.
func (s *Store) migrateNotes() error {
	rows, err := s.db.Query("PRAGMA table_info(papers)")
	if err != nil {
		return err
	}
	found := false
	for rows.Next() {
		var id, notnull, pk int
		var name, typ string
		var def any
		if err := rows.Scan(&id, &name, &typ, &notnull, &def, &pk); err != nil {
			rows.Close()
			return err
		}
		found = found || name == "notes_updated_at"
	}
	err = rows.Err()
	rows.Close()
	if err != nil {
		return err
	}
	tx, err := s.db.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()
	if !found {
		if _, err = tx.Exec("ALTER TABLE papers ADD COLUMN notes_updated_at TEXT NOT NULL DEFAULT ''"); err != nil {
			return err
		}
		if _, err = tx.Exec("UPDATE papers SET notes_updated_at = updated_at WHERE notes != ''"); err != nil {
			return err
		}
	}
	_, err = tx.Exec(`CREATE TRIGGER IF NOT EXISTS papers_notes_insert AFTER INSERT ON papers
WHEN NEW.notes != '' AND NEW.notes_updated_at = '' BEGIN
UPDATE papers SET notes_updated_at = strftime('%Y-%m-%dT%H:%M:%fZ','now') WHERE id = NEW.id;
END;
CREATE TRIGGER IF NOT EXISTS papers_notes_update AFTER UPDATE OF notes ON papers
WHEN NEW.notes IS NOT OLD.notes BEGIN
UPDATE papers SET notes_updated_at = strftime('%Y-%m-%dT%H:%M:%fZ','now') WHERE id = NEW.id;
END;`)
	if err != nil {
		return err
	}
	return tx.Commit()
}

// UpdateFields updates only explicitly supplied columns. Relations and fields are
// committed together so a failed foreign key never clears the existing set.
// expectedNotes is optional for compatibility with older clients.
func (s *Store) UpdateFields(id int64, fields map[string]any, tags, collections *[]int64, expectedNotes *string) error {
	allowed := map[string]bool{"title": true, "authors": true, "year": true, "venue": true, "doi": true, "keywords": true, "link": true, "summary": true, "notes": true, "status": true, "read": true, "starred": true, "category_id": true, "fulltext": true}
	keys := make([]string, 0, len(fields))
	for k := range fields {
		if !allowed[k] {
			return fmt.Errorf("unsupported paper field %q", k)
		}
		keys = append(keys, k)
	}
	sort.Strings(keys)
	tx, err := s.db.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()
	if len(keys) > 0 {
		sets := []string{}
		args := []any{}
		for _, k := range keys {
			sets = append(sets, k+" = ?")
			args = append(args, fields[k])
		}
		sets = append(sets, "updated_at = ?")
		args = append(args, nowStr(), id)
		query := "UPDATE papers SET " + strings.Join(sets, ", ") + " WHERE id = ?"
		if expectedNotes != nil {
			query += " AND notes = ?"
			args = append(args, *expectedNotes)
		}
		res, err := tx.Exec(query, args...)
		if err != nil {
			return err
		}
		n, err := res.RowsAffected()
		if err != nil {
			return err
		}
		if n == 0 {
			var exists int
			if err := tx.QueryRow("SELECT 1 FROM papers WHERE id = ?", id).Scan(&exists); err != nil && !errors.Is(err, sql.ErrNoRows) {
				return err
			}
			return ErrConflict
		}
	} else {
		var exists int
		if err := tx.QueryRow("SELECT 1 FROM papers WHERE id = ?", id).Scan(&exists); err != nil {
			if errors.Is(err, sql.ErrNoRows) {
				return ErrConflict
			}
			return err
		}
	}
	if tags != nil {
		if err := replaceRelations(tx, "paper_tags", "tag_id", id, *tags); err != nil {
			return err
		}
	}
	if collections != nil {
		if err := replaceRelations(tx, "paper_collections", "collection_id", id, *collections); err != nil {
			return err
		}
	}
	return tx.Commit()
}

func replaceRelations(tx *sql.Tx, table, column string, id int64, ids []int64) error {
	if _, err := tx.Exec("DELETE FROM "+table+" WHERE paper_id = ?", id); err != nil {
		return err
	}
	for _, related := range ids {
		if _, err := tx.Exec("INSERT OR IGNORE INTO "+table+" (paper_id, "+column+") VALUES (?,?)", id, related); err != nil {
			return err
		}
	}
	return nil
}

func (s *Store) UpdateNotes(id int64, notes string, expected *string) error {
	return s.UpdateFields(id, map[string]any{"notes": notes}, nil, nil, expected)
}
func (s *Store) UpdateSummary(id int64, summary string) error {
	return s.UpdateFields(id, map[string]any{"summary": summary}, nil, nil, nil)
}
