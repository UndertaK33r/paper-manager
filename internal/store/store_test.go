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

// 搜索覆盖 title/authors/venue/doi/keywords/summary/fulltext 七列，中英文子串都能命中
func TestSearchMatchesAllColumns(t *testing.T) {
	st := newTestStore(t)
	en := models.Paper{Title: "Image Fusion with Knowledge Distillation", Authors: "Ran Zhang", Keywords: "image fusion"}
	if _, err := st.CreatePaper(&en); err != nil {
		t.Fatal(err)
	}
	zh := models.Paper{Title: "红外与可见光图像融合综述", Authors: "张三"}
	if _, err := st.CreatePaper(&zh); err != nil {
		t.Fatal(err)
	}

	find := func(q string) []models.Paper {
		res, err := st.ListPapers(models.PaperQuery{Page: 1, PageSize: 10, Search: q})
		if err != nil {
			t.Fatal(err)
		}
		return res.Papers
	}

	// 英文子串（≥3 字符 → FTS）
	if got := find("fusion"); len(got) != 1 || got[0].ID != en.ID {
		t.Fatalf("搜索 fusion 应只命中英文那篇，实际 %+v", got)
	}
	// 中文子串
	if got := find("图像融合"); len(got) != 1 || got[0].Title != zh.Title {
		t.Fatalf("搜索「图像融合」结果不对: %+v", got)
	}
	// 短查询（2 字符）
	if got := find("融合"); len(got) != 1 {
		t.Fatalf("搜索「融合」应命中 1 篇，实际 %d", len(got))
	}
	// 全文列也参与检索
	ft := models.Paper{Title: "Unrelated Title", FullText: "we propose a novel distillation framework"}
	if _, err := st.CreatePaper(&ft); err != nil {
		t.Fatal(err)
	}
	if got := find("novel distillation"); len(got) != 1 || got[0].ID != ft.ID {
		t.Fatalf("全文搜索未命中: %+v", got)
	}

	// 更新标题后按新标题可搜、旧标题搜不到
	updated, _ := st.GetPaper(en.ID)
	updated.Title = "Renamed Paper About Transformers"
	if _, err := st.SavePaper(&updated, nil, nil, false); err != nil {
		t.Fatal(err)
	}
	if got := find("transformers"); len(got) != 1 || got[0].ID != en.ID {
		t.Fatalf("更新后新标题搜不到: %+v", got)
	}
	if got := find("fusion with knowledge"); len(got) != 0 {
		t.Fatalf("更新后旧标题仍能搜到: %+v", got)
	}

	// 特殊字符（LIKE 通配符）不会导致 SQL 错误
	if _, err := st.ListPapers(models.PaperQuery{Page: 1, PageSize: 5, Search: `a"b(c) -x%`}); err != nil {
		t.Fatalf("特殊字符查询报错: %v", err)
	}

	// 软删除后不再出现在搜索结果里
	if _, err := st.DeletePaper(zh.ID); err != nil {
		t.Fatal(err)
	}
	if got := find("图像融合"); len(got) != 0 {
		t.Fatalf("软删除的论文仍被搜到: %+v", got)
	}
}

// 回归：带笔记的插入 + 搜索 + 改状态后数据库仍完整
// （曾因笔记触发器嵌套写入 FTS 索引导致 CreatePaper 报 database disk image is malformed）
func TestNotesWritePathsKeepIntegrity(t *testing.T) {
	st := newTestStore(t)
	p := models.Paper{Title: "Fusion Notes Paper", Authors: "A", Notes: "重要笔记"}
	id, err := st.CreatePaper(&p)
	if err != nil {
		t.Fatalf("带笔记创建失败: %v", err)
	}
	if got := searchTitles(t, st, "fusion"); len(got) != 1 {
		t.Fatalf("创建后搜索应命中 1 篇，实际 %v", got)
	}
	if err := st.UpdateNotes(id, "改过的笔记内容", nil); err != nil {
		t.Fatalf("更新笔记失败: %v", err)
	}
	if got := searchTitles(t, st, "fusion"); len(got) != 1 {
		t.Fatalf("更新笔记后搜索应命中 1 篇，实际 %v", got)
	}
	if err := st.SetPaperStatus(id, "reading"); err != nil {
		t.Fatalf("改状态失败: %v", err)
	}
	var ic string
	if err := st.db.QueryRow("PRAGMA integrity_check").Scan(&ic); err != nil || ic != "ok" {
		t.Fatalf("完整性检查: %q %v", ic, err)
	}
}

func searchTitles(t *testing.T, st *Store, q string) []string {
	t.Helper()
	res, err := st.ListPapers(models.PaperQuery{Page: 1, PageSize: 10, Search: q})
	if err != nil {
		t.Fatal(err)
	}
	out := []string{}
	for _, p := range res.Papers {
		out = append(out, p.Title)
	}
	return out
}

// 标注：CRUD、按页排序、颜色收敛、随论文彻底删除而级联清理
func TestAnnotationsCRUDAndCascade(t *testing.T) {
	st := newTestStore(t)
	p := seedPaper(t, st)

	rect := func(page int, x, y, w, h float64) []models.AnnoRect {
		return []models.AnnoRect{{Page: page, X: x, Y: y, W: w, H: h}}
	}
	a1 := models.Annotation{PaperID: p.ID, Page: 5, Rects: rect(5, .1, .2, .3, .02), Quote: "第五页的句子"}
	a2 := models.Annotation{PaperID: p.ID, Page: 1, Rects: rect(1, .2, .1, .4, .02), Quote: "第一页的句子", Color: "green", Note: "备注"}
	if _, err := st.CreateAnnotation(&a2); err != nil {
		t.Fatal(err)
	}
	if _, err := st.CreateAnnotation(&a1); err != nil {
		t.Fatal(err)
	}

	list, err := st.ListAnnotations(p.ID)
	if err != nil {
		t.Fatal(err)
	}
	if len(list) != 2 || list[0].Page != 1 || list[1].Page != 5 {
		t.Fatalf("应按页码排序: %+v", list)
	}
	if list[0].Color != "green" || list[0].Note != "备注" {
		t.Fatalf("字段丢失: %+v", list[0])
	}
	if len(list[0].Rects) != 1 || list[0].Rects[0].W != 0.4 {
		t.Fatalf("矩形未持久化: %+v", list[0].Rects)
	}
	if list[0].CreatedAt.IsZero() {
		t.Fatal("创建时间未解析")
	}
	if n, _ := st.CountAnnotations(p.ID); n != 2 {
		t.Fatalf("计数错误: %d", n)
	}
	// 新建后应带回创建时间（由数据库默认值生成，需要回读）
	if a2.CreatedAt.IsZero() {
		t.Fatal("CreateAnnotation 未回读 created_at")
	}

	// 未知颜色收敛为 yellow
	bad := models.Annotation{PaperID: p.ID, Page: 2, Rects: rect(2, .1, .1, .1, .01), Quote: "x", Color: "rainbow"}
	if _, err := st.CreateAnnotation(&bad); err != nil {
		t.Fatal(err)
	}
	got, _ := st.ListAnnotations(p.ID)
	for _, a := range got {
		if a.ID == bad.ID && a.Color != "yellow" {
			t.Fatalf("颜色未收敛: %q", a.Color)
		}
	}

	// 更新备注与颜色
	newNote, newColor := "改过的备注", "blue"
	if err := st.UpdateAnnotation(a2.ID, &newColor, &newNote, nil, nil); err != nil {
		t.Fatal(err)
	}
	got, _ = st.ListAnnotations(p.ID)
	for _, a := range got {
		if a.ID == a2.ID && (a.Note != newNote || a.Color != "blue") {
			t.Fatalf("更新未生效: %+v", a)
		}
	}
	if err := st.UpdateAnnotation(99999, nil, &newNote, nil, nil); err != ErrConflict {
		t.Fatalf("更新不存在的标注应返回 ErrConflict, got %v", err)
	}

	// 删除
	if err := st.DeleteAnnotation(a2.ID); err != nil {
		t.Fatal(err)
	}
	if err := st.DeleteAnnotation(a2.ID); err != ErrConflict {
		t.Fatalf("重复删除应返回 ErrConflict, got %v", err)
	}
	if n, _ := st.CountAnnotations(p.ID); n != 2 {
		t.Fatalf("删除后计数错误: %d", n)
	}

	// 论文彻底删除后标注级联清理
	if _, err := st.DeletePaper(p.ID); err != nil {
		t.Fatal(err)
	}
	if _, err := st.PurgePaper(p.ID); err != nil {
		t.Fatal(err)
	}
	if n, _ := st.CountAnnotations(p.ID); n != 0 {
		t.Fatalf("论文删除后标注未清理: %d", n)
	}
}

// 软删除（进回收站）时标注保留，恢复后仍在
func TestAnnotationsSurviveSoftDelete(t *testing.T) {
	st := newTestStore(t)
	p := seedPaper(t, st)
	if _, err := st.CreateAnnotation(&models.Annotation{
		PaperID: p.ID, Page: 1,
		Rects: []models.AnnoRect{{Page: 1, X: .1, Y: .1, W: .2, H: .02}},
		Quote: "abc",
	}); err != nil {
		t.Fatal(err)
	}
	if _, err := st.DeletePaper(p.ID); err != nil {
		t.Fatal(err)
	}
	if n, _ := st.CountAnnotations(p.ID); n != 1 {
		t.Fatal("软删除不应清理标注")
	}
	if err := st.RestorePaper(p.ID); err != nil {
		t.Fatal(err)
	}
	if list, _ := st.ListAnnotations(p.ID); len(list) != 1 {
		t.Fatalf("恢复后标注丢失: %+v", list)
	}
}

// 文字批注：kind 区分、位置可改（拖动）、与高亮共存排序
func TestNoteAnnotationsStore(t *testing.T) {
	st := newTestStore(t)
	p := seedPaper(t, st)

	note := models.Annotation{PaperID: p.ID, Kind: models.AnnoKindNote, Page: 2, X: 0.3, Y: 0.7, Color: "blue"}
	if _, err := st.CreateAnnotation(&note); err != nil {
		t.Fatal(err)
	}
	hl := models.Annotation{PaperID: p.ID, Kind: models.AnnoKindHighlight, Page: 1,
		Rects: []models.AnnoRect{{Page: 1, X: .1, Y: .1, W: .2, H: .02}}, Quote: "高亮"}
	if _, err := st.CreateAnnotation(&hl); err != nil {
		t.Fatal(err)
	}
	list, err := st.ListAnnotations(p.ID)
	if err != nil {
		t.Fatal(err)
	}
	if len(list) != 2 || list[0].Page != 1 || list[1].Page != 2 {
		t.Fatalf("排序不对: %+v", list)
	}
	if list[1].Kind != models.AnnoKindNote || list[1].X != 0.3 || list[1].Y != 0.7 {
		t.Fatalf("批注字段丢失: %+v", list[1])
	}
	if list[0].Kind != models.AnnoKindHighlight || len(list[0].Rects) != 1 {
		t.Fatalf("高亮字段丢失: %+v", list[0])
	}

	// 拖动批注框 + 改文字
	nx, ny := 0.6, 0.25
	text := "改过的批注"
	if err := st.UpdateAnnotation(note.ID, nil, &text, &nx, &ny); err != nil {
		t.Fatal(err)
	}
	list, _ = st.ListAnnotations(p.ID)
	if list[1].X != 0.6 || list[1].Y != 0.25 || list[1].Note != text {
		t.Fatalf("拖动/改字未生效: %+v", list[1])
	}

	// kind 为空时按高亮处理（兼容旧数据）
	raw := models.Annotation{PaperID: p.ID, Page: 3, Rects: []models.AnnoRect{{Page: 3, X: .1, Y: .1, W: .1, H: .01}}}
	if _, err := st.CreateAnnotation(&raw); err != nil {
		t.Fatal(err)
	}
	list, _ = st.ListAnnotations(p.ID)
	for _, a := range list {
		if a.ID == raw.ID && a.Kind != models.AnnoKindHighlight {
			t.Fatalf("空 kind 应归一为 highlight: %q", a.Kind)
		}
	}
}

// 列表筛选：按分类 ID、标签名、合集名过滤（此前 API 把名字当 ID 解析，条件被静默丢弃）
func TestListPapersFiltering(t *testing.T) {
	st := newTestStore(t)
	mk := func(title string) int64 {
		p := models.Paper{Title: title}
		id, err := st.CreatePaper(&p)
		if err != nil {
			t.Fatal(err)
		}
		return id
	}
	a, b := mk("论文A"), mk("论文B")

	cat, err := st.EnsureCategory("cv")
	if err != nil {
		t.Fatal(err)
	}
	if err := st.UpdateFields(a, map[string]any{"category_id": cat.ID}, nil, nil, nil); err != nil {
		t.Fatal(err)
	}
	tagFusion, _ := st.EnsureTag("fusion")
	tagDistill, _ := st.EnsureTag("蒸馏")
	colRead, _ := st.EnsureCollection("要精读的", "")
	if err := st.AddPaperTag(a, tagFusion.ID); err != nil {
		t.Fatal(err)
	}
	if err := st.AddPaperTag(b, tagFusion.ID); err != nil {
		t.Fatal(err)
	}
	if err := st.AddPaperTag(b, tagDistill.ID); err != nil {
		t.Fatal(err)
	}
	if err := st.AddPaperCollection(a, colRead.ID); err != nil {
		t.Fatal(err)
	}

	cases := []struct {
		name string
		q    models.PaperQuery
		want int64
	}{
		{"无条件", models.PaperQuery{}, 2},
		{"按分类", models.PaperQuery{CategoryID: &cat.ID}, 1},
		{"按标签名", models.PaperQuery{TagNames: []string{"fusion"}}, 2},
		{"按标签名（单个）", models.PaperQuery{TagNames: []string{"蒸馏"}}, 1},
		{"按合集名", models.PaperQuery{CollectionNames: []string{"要精读的"}}, 1},
		{"不存在的标签", models.PaperQuery{TagNames: []string{"nope"}}, 0},
		{"标签+合集", models.PaperQuery{TagNames: []string{"fusion"}, CollectionNames: []string{"要精读的"}}, 1},
	}
	for _, c := range cases {
		res, err := st.ListPapers(c.q)
		if err != nil {
			t.Fatalf("%s: %v", c.name, err)
		}
		if res.Total != c.want {
			t.Errorf("%s: 命中 %d 篇，期望 %d", c.name, res.Total, c.want)
		}
	}
}
