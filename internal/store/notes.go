package store

import (
	"database/sql"
	"errors"
	"fmt"
	"sort"
	"strings"
)

var ErrConflict = errors.New("笔记已被其他窗口修改，请刷新后合并，未覆盖已有笔记")

// migrateNotes 为历史笔记补一次时间戳，并清理旧版遗留的触发器。
// 时间戳现在由 Go 层在写入时维护（INSERT/UPDATE 都覆盖，包括 PUT 全量提交）。
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
	// 时间戳改由 Go 层维护（见 paper_write.go / UpdateFields）：
	// 触发器会在 FTS 外部内容表上产生嵌套写入，而 SQLite 按"创建顺序的逆序"触发，
	// 导致同一 rowid 被插入两次、索引损坏（database disk image is malformed）。
	// 这里清理旧版本遗留的触发器。
	_, err = tx.Exec(`DROP TRIGGER IF EXISTS papers_notes_insert;
DROP TRIGGER IF EXISTS papers_notes_update;`)
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
		// 笔记内容变化时才更新 notes_updated_at（旧版靠触发器，见文件顶部说明）
		if notes, ok := fields["notes"]; ok {
			sets = append(sets, "notes_updated_at = CASE WHEN notes IS NOT ? THEN ? ELSE notes_updated_at END")
			args = append(args, notes, nowMillis())
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
