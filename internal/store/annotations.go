package store

import (
	"database/sql"
	"encoding/json"

	"paper-manager/internal/models"
)

// 允许的高亮颜色（前端也只提供这几个）
var annotationColors = map[string]bool{
	"yellow": true, "green": true, "blue": true, "pink": true,
}

// NormalizeAnnotationColor 把未知颜色收敛为默认黄色。
func NormalizeAnnotationColor(c string) string {
	if annotationColors[c] {
		return c
	}
	return "yellow"
}

// ListAnnotations 返回某篇论文的标注，按在全文中的位置排序。
func (s *Store) ListAnnotations(paperID int64) ([]models.Annotation, error) {
	rows, err := s.db.Query(`SELECT id, paper_id, kind, page, rects, x, y, quote, color, note, created_at
		FROM annotations WHERE paper_id = ? ORDER BY page, id`, paperID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []models.Annotation{}
	for rows.Next() {
		var a models.Annotation
		var rects, created string
		if err := rows.Scan(&a.ID, &a.PaperID, &a.Kind, &a.Page, &rects, &a.X, &a.Y, &a.Quote, &a.Color, &a.Note, &created); err != nil {
			return nil, err
		}
		if rects != "" {
			_ = json.Unmarshal([]byte(rects), &a.Rects)
		}
		if a.Rects == nil {
			a.Rects = []models.AnnoRect{}
		}
		a.CreatedAt = parseTime(created)
		a.Color = NormalizeAnnotationColor(a.Color)
		if a.Kind == "" {
			a.Kind = models.AnnoKindHighlight
		}
		out = append(out, a)
	}
	return out, rows.Err()
}

// CreateAnnotation 新建标注；论文不存在时由外键约束拦下。
func (s *Store) CreateAnnotation(a *models.Annotation) (int64, error) {
	a.Color = NormalizeAnnotationColor(a.Color)
	if a.Page < 1 {
		a.Page = 1
	}
	raw, err := json.Marshal(a.Rects)
	if err != nil {
		return 0, err
	}
	if a.Kind != models.AnnoKindNote {
		a.Kind = models.AnnoKindHighlight
	}
	res, err := s.db.Exec(`INSERT INTO annotations (paper_id, kind, page, rects, x, y, quote, color, note)
		VALUES (?,?,?,?,?,?,?,?,?)`,
		a.PaperID, a.Kind, a.Page, string(raw), a.X, a.Y, a.Quote, a.Color, a.Note)
	if err != nil {
		return 0, err
	}
	id, err := res.LastInsertId()
	if err != nil {
		return 0, err
	}
	a.ID = id
	return id, nil
}

// UpdateAnnotation 只更新显式提供的字段（颜色 / 备注）。
func (s *Store) UpdateAnnotation(id int64, color, note *string, x, y *float64) error {
	sets := []string{}
	args := []any{}
	if color != nil {
		sets = append(sets, "color = ?")
		args = append(args, NormalizeAnnotationColor(*color))
	}
	if note != nil {
		sets = append(sets, "note = ?")
		args = append(args, *note)
	}
	if x != nil && y != nil { // 拖动文字批注框
		sets = append(sets, "x = ?", "y = ?")
		args = append(args, *x, *y)
	}
	if len(sets) == 0 {
		return nil
	}
	args = append(args, id)
	res, err := s.db.Exec("UPDATE annotations SET "+joinComma(sets)+" WHERE id = ?", args...)
	if err != nil {
		return err
	}
	if n, err := res.RowsAffected(); err == nil && n == 0 {
		return ErrConflict
	}
	return nil
}

// DeleteAnnotation 删除一条标注。
func (s *Store) DeleteAnnotation(id int64) error {
	res, err := s.db.Exec("DELETE FROM annotations WHERE id = ?", id)
	if err != nil {
		return err
	}
	if n, err := res.RowsAffected(); err == nil && n == 0 {
		return ErrConflict
	}
	return nil
}

// CountAnnotations 统计某篇论文的标注数量。
func (s *Store) CountAnnotations(paperID int64) (int, error) {
	var n int
	err := s.db.QueryRow("SELECT COUNT(*) FROM annotations WHERE paper_id = ?", paperID).Scan(&n)
	if err == sql.ErrNoRows {
		return 0, nil
	}
	return n, err
}

func joinComma(parts []string) string {
	out := ""
	for i, p := range parts {
		if i > 0 {
			out += ", "
		}
		out += p
	}
	return out
}
