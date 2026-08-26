package store

import (
	"strings"

	"paper-manager/internal/models"
)

func (s *Store) ListCategories() ([]models.Category, error) {
	rows, err := s.db.Query(`SELECT c.id, c.name, (SELECT COUNT(*) FROM papers WHERE category_id = c.id)
		FROM categories c ORDER BY c.name COLLATE NOCASE`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []models.Category{}
	for rows.Next() {
		var c models.Category
		if err := rows.Scan(&c.ID, &c.Name, &c.PaperCount); err != nil {
			return nil, err
		}
		out = append(out, c)
	}
	return out, rows.Err()
}

func (s *Store) EnsureCategory(name string) (models.Category, error) {
	name = strings.TrimSpace(name)
	_, err := s.db.Exec("INSERT OR IGNORE INTO categories (name) VALUES (?)", name)
	if err != nil {
		return models.Category{}, err
	}
	var c models.Category
	err = s.db.QueryRow("SELECT id, name, (SELECT COUNT(*) FROM papers WHERE category_id = id) FROM categories WHERE name = ?", name).Scan(&c.ID, &c.Name, &c.PaperCount)
	return c, err
}

func (s *Store) DeleteCategory(id int64) error {
	_, err := s.db.Exec("DELETE FROM categories WHERE id = ?", id)
	return err
}

func (s *Store) ListTags() ([]models.Tag, error) {
	rows, err := s.db.Query(`SELECT t.id, t.name, (SELECT COUNT(*) FROM paper_tags WHERE tag_id = t.id)
		FROM tags t ORDER BY t.name COLLATE NOCASE`)
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

func (s *Store) EnsureTag(name string) (models.Tag, error) {
	name = strings.TrimSpace(name)
	_, err := s.db.Exec("INSERT OR IGNORE INTO tags (name) VALUES (?)", name)
	if err != nil {
		return models.Tag{}, err
	}
	var t models.Tag
	err = s.db.QueryRow("SELECT id, name, (SELECT COUNT(*) FROM paper_tags WHERE tag_id = id) FROM tags WHERE name = ?", name).Scan(&t.ID, &t.Name, &t.PaperCount)
	return t, err
}

func (s *Store) DeleteTag(id int64) error {
	_, err := s.db.Exec("DELETE FROM tags WHERE id = ?", id)
	return err
}

func (s *Store) ListCollections() ([]models.Collection, error) {
	rows, err := s.db.Query(`SELECT c.id, c.name, c.description, (SELECT COUNT(*) FROM paper_collections WHERE collection_id = c.id)
		FROM collections c ORDER BY c.name COLLATE NOCASE`)
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

func (s *Store) EnsureCollection(name, description string) (models.Collection, error) {
	name = strings.TrimSpace(name)
	_, err := s.db.Exec("INSERT OR IGNORE INTO collections (name, description) VALUES (?,?)", name, description)
	if err != nil {
		return models.Collection{}, err
	}
	var c models.Collection
	err = s.db.QueryRow(`SELECT id, name, description, (SELECT COUNT(*) FROM paper_collections WHERE collection_id = id)
		FROM collections WHERE name = ?`, name).Scan(&c.ID, &c.Name, &c.Description, &c.PaperCount)
	return c, err
}

func (s *Store) DeleteCollection(id int64) error {
	_, err := s.db.Exec("DELETE FROM collections WHERE id = ?", id)
	return err
}

func (s *Store) Stats() map[string]any {
	out := map[string]any{}
	var total, readN, starred, categories, tags, collections int64
	s.db.QueryRow("SELECT COUNT(*) FROM papers").Scan(&total)
	s.db.QueryRow("SELECT COUNT(*) FROM papers WHERE read = 1").Scan(&readN)
	s.db.QueryRow("SELECT COUNT(*) FROM papers WHERE starred = 1").Scan(&starred)
	s.db.QueryRow("SELECT COUNT(*) FROM categories").Scan(&categories)
	s.db.QueryRow("SELECT COUNT(*) FROM tags").Scan(&tags)
	s.db.QueryRow("SELECT COUNT(*) FROM collections").Scan(&collections)
	out["total"] = total
	out["read"] = readN
	out["unread"] = total - readN
	out["starred"] = starred
	out["categories"] = categories
	out["tags"] = tags
	out["collections"] = collections
	return out
}
