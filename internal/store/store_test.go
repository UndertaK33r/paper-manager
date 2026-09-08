package store

import (
	"path/filepath"
	"strings"
	"testing"

	"paper-manager/internal/models"
)

func newTestStore(t *testing.T) *Store {
	t.Helper()
	st, err := Open(filepath.Join(t.TempDir(), "test.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { st.Close() })
	return st
}

func seedPaper(t *testing.T, st *Store) *models.Paper {
	t.Helper()
	p := models.Paper{Title: "Test Paper", Authors: "A. Author", Notes: "v1"}
	id, err := st.CreatePaper(&p)
	if err != nil {
		t.Fatal(err)
	}
	p.ID = id
	got, err := st.GetPaper(id)
	if err != nil {
		t.Fatal(err)
	}
	return &got
}

func TestNotesTimestampOnlyChangesWithNotes(t *testing.T) {
	st := newTestStore(t)
	p := seedPaper(t, st)
	if p.NotesUpdatedAt == "" {
		t.Fatal("seed note should set notes_updated_at")
	}
	before := p.NotesUpdatedAt

	if err := st.SetPaperStarred(p.ID, true); err != nil {
		t.Fatal(err)
	}
	if err := st.SetPaperStatus(p.ID, "reading"); err != nil {
		t.Fatal(err)
	}
	got, err := st.GetPaper(p.ID)
	if err != nil {
		t.Fatal(err)
	}
	if got.NotesUpdatedAt != before {
		t.Fatalf("notes date drifted: %s -> %s", before, got.NotesUpdatedAt)
	}

	if err := st.UpdateNotes(p.ID, "v1", nil); err != nil {
		t.Fatal(err)
	}
	got, _ = st.GetPaper(p.ID)
	if got.NotesUpdatedAt != before {
		t.Fatal("unchanged note updated date")
	}

	if err := st.UpdateNotes(p.ID, "v2", nil); err != nil {
		t.Fatal(err)
	}
	got, _ = st.GetPaper(p.ID)
	if got.NotesUpdatedAt == before {
		t.Fatal("note edit did not update date")
	}
}

func TestNotesGuardedUpdateDetectsConflict(t *testing.T) {
	st := newTestStore(t)
	p := seedPaper(t, st)
	expected := "v1"
	if err := st.UpdateNotes(p.ID, "v2", &expected); err != nil {
		t.Fatal(err)
	}
	if err := st.UpdateNotes(p.ID, "stale", &expected); err != ErrConflict {
		t.Fatalf("want ErrConflict, got %v", err)
	}
	got, _ := st.GetPaper(p.ID)
	if got.Notes != "v2" {
		t.Fatalf("stale write overwrote note: %q", got.Notes)
	}
}

func TestSavePaperAtomicRelations(t *testing.T) {
	st := newTestStore(t)
	p := seedPaper(t, st)
	cat, err := st.EnsureCategory("机器学习")
	if err != nil {
		t.Fatal(err)
	}
	tag, err := st.EnsureTag("深度学习")
	if err != nil {
		t.Fatal(err)
	}
	col, err := st.EnsureCollection("必读", "")
	if err != nil {
		t.Fatal(err)
	}
	p.CategoryID = &cat.ID
	p.Title = "Updated"
	if _, err := st.SavePaper(p, []int64{tag.ID}, []int64{col.ID}, false); err != nil {
		t.Fatal(err)
	}
	got, _ := st.GetPaper(p.ID)
	if got.Title != "Updated" || len(got.Tags) != 1 || got.Tags[0].Name != "深度学习" || len(got.Collections) != 1 {
		t.Fatalf("unexpected state: %+v", got)
	}
	if _, err := st.SavePaper(&got, []int64{99999}, []int64{col.ID}, false); err == nil {
		t.Fatal("expected FK error")
	}
	got, _ = st.GetPaper(p.ID)
	if len(got.Tags) != 1 || got.Tags[0].Name != "深度学习" || len(got.Collections) != 1 {
		t.Fatalf("relations clobbered on failure: %+v %+v", got.Tags, got.Collections)
	}
}

func TestUpdateFieldsRejectsUnknownAndConflicts(t *testing.T) {
	st := newTestStore(t)
	p := seedPaper(t, st)
	if err := st.UpdateFields(p.ID, map[string]any{"hacker": "x"}, nil, nil, nil); err == nil {
		t.Fatal("unknown field accepted")
	}
	if err := st.UpdateFields(p.ID, map[string]any{"notes": "x"}, nil, nil, nil); err != nil {
		t.Fatal(err)
	}
	if err := st.UpdateFields(p.ID+999, map[string]any{"notes": "x"}, nil, nil, nil); err != ErrConflict {
		t.Fatalf("missing row should be conflict, got %v", err)
	}
}

func TestAllNotesUsesNoteDateAndSkipsEmpty(t *testing.T) {
	st := newTestStore(t)
	seedPaper(t, st)
	empty := models.Paper{Title: "No Notes"}
	if _, err := st.CreatePaper(&empty); err != nil {
		t.Fatal(err)
	}
	if err := st.UpdateNotes(empty.ID, "   ", nil); err != nil {
		t.Fatal(err)
	}
	rows, err := st.AllNotes()
	if err != nil {
		t.Fatal(err)
	}
	if len(rows) != 1 {
		t.Fatalf("want 1 note row, got %d", len(rows))
	}
	if !strings.Contains(rows[0].Notes, "v1") {
		t.Fatalf("unexpected note %q", rows[0].Notes)
	}
	if rows[0].Updated == "" {
		t.Fatal("missing note date")
	}
}

func TestUpdateMetadataKeepsNotesAndStatus(t *testing.T) {
	st := newTestStore(t)
	p := seedPaper(t, st)
	if err := st.SetPaperStatus(p.ID, "reading"); err != nil {
		t.Fatal(err)
	}
	before, _ := st.GetPaper(p.ID)
	mp := models.Paper{ID: p.ID, Title: "New Title", Notes: "hacked", Status: "read", FullText: "ft"}
	if err := st.UpdateMetadata(&mp, true); err != nil {
		t.Fatal(err)
	}
	got, _ := st.GetPaper(p.ID)
	if got.Title != "New Title" {
		t.Fatal("title not updated")
	}
	if got.Notes != "v1" {
		t.Fatalf("notes clobbered: %q", got.Notes)
	}
	if got.Status != "reading" {
		t.Fatalf("status clobbered: %q", got.Status)
	}
	if ft := st.PaperFullText(p.ID); ft != "ft" {
		t.Fatalf("fulltext not updated: %q", ft)
	}
	if got.NotesUpdatedAt != before.NotesUpdatedAt {
		t.Fatal("notes date changed by metadata update")
	}
}
