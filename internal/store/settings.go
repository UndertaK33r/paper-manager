package store

import "strings"

func (s *Store) GetSetting(key string) string {
	var v string
	err := s.db.QueryRow("SELECT value FROM settings WHERE key = ?", key).Scan(&v)
	if err != nil {
		return ""
	}
	return v
}

func (s *Store) SetSetting(key, value string) error {
	key = strings.TrimSpace(key)
	_, err := s.db.Exec(`INSERT INTO settings (key, value, updated_at)
		VALUES (?, ?, strftime('%Y-%m-%dT%H:%M:%SZ','now'))
		ON CONFLICT(key) DO UPDATE SET value = excluded.value, updated_at = excluded.updated_at`, key, value)
	return err
}

func (s *Store) DeleteSetting(key string) error {
	_, err := s.db.Exec("DELETE FROM settings WHERE key = ?", key)
	return err
}
