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

// 列表页关联批量加载：每篇论文只应拿到自己的标签/合集（防止批量映射串行）
func TestListPapersRelationsArePerPaper(t *testing.T) {
	st := newTestStore(t)
	mk := func(title string, tags, cols []string) int64 {
		p := models.Paper{Title: title}
		id, err := st.CreatePaper(&p)
		if err != nil {
			t.Fatal(err)
		}
		var tagIDs, colIDs []int64
		for _, name := range tags {
			tag, err := st.EnsureTag(name)
			if err != nil {
				t.Fatal(err)
			}
			tagIDs = append(tagIDs, tag.ID)
		}
		for _, name := range cols {
			col, err := st.EnsureCollection(name, "")
			if err != nil {
				t.Fatal(err)
			}
			colIDs = append(colIDs, col.ID)
		}
		got, err := st.GetPaper(id)
		if err != nil {
			t.Fatal(err)
		}
		if _, err := st.SavePaper(&got, tagIDs, colIDs, false); err != nil {
			t.Fatal(err)
		}
		return id
	}
	a := mk("A 论文", []string{"标签一", "标签二"}, []string{"合集一"})
	b := mk("B 论文", []string{"标签三"}, nil)
	c := mk("C 论文", nil, nil)

	res, err := st.ListPapers(models.PaperQuery{Page: 1, PageSize: 10})
	if err != nil {
		t.Fatal(err)
	}
	if len(res.Papers) != 3 {
		t.Fatalf("want 3 papers, got %d", len(res.Papers))
	}
	byID := map[int64]models.Paper{}
	for _, p := range res.Papers {
		byID[p.ID] = p
	}
	if len(byID[a].Tags) != 2 || len(byID[a].Collections) != 1 {
		t.Fatalf("A 关联错误: %+v %+v", byID[a].Tags, byID[a].Collections)
	}
	if len(byID[b].Tags) != 1 || byID[b].Tags[0].Name != "标签三" || len(byID[b].Collections) != 0 {
		t.Fatalf("B 关联错误: %+v %+v", byID[b].Tags, byID[b].Collections)
	}
	if len(byID[c].Tags) != 0 || len(byID[c].Collections) != 0 {
		t.Fatalf("C 关联错误: %+v %+v", byID[c].Tags, byID[c].Collections)
	}
	// 无关联时也要是非 nil 切片，保证 JSON 输出 [] 而不是 null
	if byID[c].Tags == nil || byID[c].Collections == nil {
		t.Fatal("空关联应为空切片而非 nil")
	}
}

// 软删除：默认删除只进回收站，可恢复；彻底删除才真正移除并返回 PDF 路径
func TestSoftDeleteRestoreAndPurge(t *testing.T) {
	st := newTestStore(t)
	p := seedPaper(t, st)
	pdfName := "keepme.pdf"
	got, err := st.GetPaper(p.ID)
	if err != nil {
		t.Fatal(err)
	}
	got.PDFPath = pdfName
	got.PDFSize = 10
	if _, err := st.SavePaper(&got, nil, nil, false); err != nil {
		t.Fatal(err)
	}

	// 软删除：列表/详情/统计/笔记导出/去重都看不到
	if _, err := st.DeletePaper(p.ID); err != nil {
		t.Fatal(err)
	}
	list, err := st.ListPapers(models.PaperQuery{Page: 1, PageSize: 10})
	if err != nil {
		t.Fatal(err)
	}
	if len(list.Papers) != 0 {
		t.Fatalf("软删除后列表仍有 %d 条", len(list.Papers))
	}
	if _, err := st.GetPaper(p.ID); err == nil {
		t.Fatal("软删除后详情仍可读")
	}
	if stats := st.Stats(); stats["total"].(int64) != 0 {
		t.Fatalf("软删除后统计仍计入: %v", stats["total"])
	}
	if rows, _ := st.AllNotes(); len(rows) != 0 {
		t.Fatalf("软删除后笔记导出仍包含该论文: %d", len(rows))
	}
	if dup, _ := st.FindDuplicate("Test Paper", "A. Author", ""); dup != nil {
		t.Fatal("软删除后仍被判定为重复")
	}
	// 重复删除应返回冲突
	if _, err := st.DeletePaper(p.ID); err != ErrConflict {
		t.Fatalf("重复删除 want ErrConflict, got %v", err)
	}

	// 回收站里能看到，PDF 路径仍在
	trash, err := st.ListTrash()
	if err != nil {
		t.Fatal(err)
	}
	if len(trash) != 1 || trash[0].ID != p.ID || trash[0].DeletedAt == "" {
		t.Fatalf("回收站内容不对: %+v", trash)
	}
	if trash[0].PDFPath != pdfName {
		t.Fatalf("回收站应保留 PDF 路径: %q", trash[0].PDFPath)
	}

	// 恢复
	if err := st.RestorePaper(p.ID); err != nil {
		t.Fatal(err)
	}
	if again, err := st.GetPaper(p.ID); err != nil || again.Title != "Test Paper" {
		t.Fatalf("恢复失败: %+v %v", again, err)
	}

	// 彻底删除：返回 PDF 文件名，行被移除
	if _, err := st.DeletePaper(p.ID); err != nil {
		t.Fatal(err)
	}
	name, err := st.PurgePaper(p.ID)
	if err != nil {
		t.Fatal(err)
	}
	if name != pdfName {
		t.Fatalf("purge 应返回 PDF 文件名 %q, got %q", pdfName, name)
	}
	if _, err := st.GetPaper(p.ID); err == nil {
		t.Fatal("彻底删除后仍能读到")
	}
	if name, err := st.PurgePaper(p.ID); err != ErrConflict {
		t.Fatalf("重复 purge want ErrConflict, got %v %v", err, name)
	}
}

// 清空回收站返回所有待删 PDF，且不碰未删除的论文
func TestEmptyTrashKeepsLivePapers(t *testing.T) {
	st := newTestStore(t)
	live := seedPaper(t, st)
	trashed := seedPaper(t, st)
	got, _ := st.GetPaper(trashed.ID)
	got.PDFPath = "gone.pdf"
	if _, err := st.SavePaper(&got, nil, nil, false); err != nil {
		t.Fatal(err)
	}
	if _, err := st.DeletePaper(trashed.ID); err != nil {
		t.Fatal(err)
	}
	paths, err := st.EmptyTrash()
	if err != nil {
		t.Fatal(err)
	}
	if len(paths) != 1 || paths[0] != "gone.pdf" {
		t.Fatalf("清空回收站返回 %v", paths)
	}
	if trash, _ := st.ListTrash(); len(trash) != 0 {
		t.Fatalf("回收站应已清空: %+v", trash)
	}
	if _, err := st.GetPaper(live.ID); err != nil {
		t.Fatalf("未删除的论文被误删: %v", err)
	}
}
