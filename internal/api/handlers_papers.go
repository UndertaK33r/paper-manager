package api

import (
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"paper-manager/internal/models"
	pdfmeta "paper-manager/internal/pdf"
	"paper-manager/internal/store"
)

func (s *Server) handleListPapers(w http.ResponseWriter, r *http.Request) {
	q := s.buildQuery(r)
	res, err := s.store.ListPapers(q)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, res)
}

func (s *Server) handleGetPaper(w http.ResponseWriter, r *http.Request) {
	id, ok := s.idParam(r, "id")
	if !ok {
		writeError(w, http.StatusBadRequest, "invalid paper id")
		return
	}
	p, err := s.store.GetPaper(id)
	if err != nil {
		writeError(w, http.StatusNotFound, "paper not found")
		return
	}
	writeJSON(w, http.StatusOK, p)
}

func formPaperInput(r *http.Request) models.PaperInput {
	in := models.PaperInput{}
	in.Title = r.FormValue("title")
	in.Authors = r.FormValue("authors")
	in.Venue = r.FormValue("venue")
	in.DOI = r.FormValue("doi")
	in.Keywords = r.FormValue("keywords")
	in.Link = r.FormValue("link")
	in.Summary = r.FormValue("summary")
	in.Notes = r.FormValue("notes")
	in.Year, _ = strconv.Atoi(strings.TrimSpace(r.FormValue("year")))
	in.CategoryID = parseIntPtr(r.FormValue("categoryId"))
	in.Read = r.FormValue("read") == "1" || strings.EqualFold(r.FormValue("read"), "true")
	in.Status = r.FormValue("status")
	in.Starred = r.FormValue("starred") == "1" || strings.EqualFold(r.FormValue("starred"), "true")
	in.Force = r.FormValue("force") == "1" || strings.EqualFold(r.FormValue("force"), "true")
	in.UseAI = r.FormValue("useAI") == "1" || strings.EqualFold(r.FormValue("useAI"), "true")
	in.TagNames = csvSplit(r.FormValue("tags"))
	in.CollectionNames = csvSplit(r.FormValue("collections"))
	return in
}

func (s *Server) decodeInput(w http.ResponseWriter, r *http.Request) (models.PaperInput, bool, error) {
	if strings.HasPrefix(r.Header.Get("Content-Type"), "multipart/form-data") {
		size := s.maxUploadMB * 1024 * 1024
		r.Body = http.MaxBytesReader(w, r.Body, size)
		if err := r.ParseMultipartForm(size); err != nil {
			return models.PaperInput{}, true, err
		}
		return formPaperInput(r), true, nil
	}
	var in models.PaperInput
	if err := readJSON(r, &in); err != nil {
		return in, false, err
	}
	return in, false, nil
}

func (s *Server) savePdf(r *http.Request) (string, int64, string, error) {
	file, header, err := r.FormFile("pdf")
	if err != nil {
		file, header, err = r.FormFile("file")
	}
	if err != nil {
		return "", 0, "", nil
	}
	defer file.Close()
	if !strings.HasSuffix(strings.ToLower(header.Filename), ".pdf") {
		return "", 0, "", fmt.Errorf("only PDF files are supported")
	}
	name := randomName() + ".pdf"
	dst := filepath.Join(s.uploadDir, name)
	out, err := os.Create(dst)
	if err != nil {
		return "", 0, "", err
	}
	n, err := io.Copy(out, file)
	closeErr := out.Close()
	if err != nil {
		os.Remove(dst)
		return "", 0, "", err
	}
	if closeErr != nil {
		os.Remove(dst)
		return "", 0, "", closeErr
	}
	if n == 0 {
		os.Remove(dst)
		return "", 0, "", fmt.Errorf("empty PDF file")
	}
	head := make([]byte, 5)
	_, herr := file.Seek(0, io.SeekStart)
	if herr == nil {
		_, herr = io.ReadFull(file, head)
	}
	if herr != nil || string(head) != "%PDF-" {
		os.Remove(dst)
		return "", 0, "", fmt.Errorf("not a valid PDF file")
	}
	orig := filepath.Base(header.Filename)
	orig = strings.TrimSuffix(orig, filepath.Ext(orig))
	return name, n, strings.TrimSpace(orig), nil
}

func randomName() string {
	b := make([]byte, 16)
	if _, err := rand.Read(b); err != nil {
		return fmt.Sprintf("%d-%d", os.Getpid(), time.Now().UnixNano())
	}
	return hex.EncodeToString(b)
}

func (s *Server) handleCreatePaper(w http.ResponseWriter, r *http.Request) {
	in, isMultipart, err := s.decodeInput(w, r)
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	in.Title = strings.TrimSpace(in.Title)
	pdfPath, pdfSize := "", int64(0)
	fullText := ""
	if isMultipart {
		name, size, origName, perr := s.savePdf(r)
		if perr != nil {
			writeError(w, http.StatusBadRequest, perr.Error())
			return
		}
		if name != "" {
			pdfPath, pdfSize = name, size
			meta, merr := pdfmeta.ExtractWithFallback(filepath.Join(s.uploadDir, name), origName)
			if merr != nil {
				os.Remove(filepath.Join(s.uploadDir, name))
				writeError(w, http.StatusBadRequest, "无法解析 PDF 元数据: "+merr.Error())
				return
			}
			if in.Title == "" {
				in.Title = meta.Title
			}
			if in.Authors == "" {
				in.Authors = meta.Author
			}
			if in.Keywords == "" {
				in.Keywords = meta.Keywords
			}
			fullText, _ = pdfmeta.ExtractText(filepath.Join(s.uploadDir, name), 200000)
		}
	}
	if in.Title == "" {
		if pdfPath != "" {
			os.Remove(filepath.Join(s.uploadDir, pdfPath))
		}
		writeError(w, http.StatusBadRequest, "title is required")
		return
	}
	_ = s.maybeEnrich(&in, pdfPath)
	dup, derr := s.store.FindDuplicate(in.Title, in.Authors, in.DOI)
	if derr != nil {
		if pdfPath != "" {
			os.Remove(filepath.Join(s.uploadDir, pdfPath))
		}
		writeError(w, http.StatusInternalServerError, derr.Error())
		return
	}
	if dup != nil && !in.Force {
		if pdfPath != "" {
			os.Remove(filepath.Join(s.uploadDir, pdfPath))
		}
		writeJSON(w, http.StatusConflict, models.DuplicateCheck{Duplicate: true, Reason: "duplicate paper", PaperID: dup.ID, Title: dup.Title})
		return
	}
	if err := s.maybeAIExtract(&in, pdfPath); err != nil {
		if pdfPath != "" {
			os.Remove(filepath.Join(s.uploadDir, pdfPath))
		}
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	p := s.paperFromInput(in, nil)
	p.PDFPath = pdfPath
	p.PDFSize = pdfSize
	p.FullText = fullText
	id, err := s.store.SavePaper(&p, in.Tags, in.Collections, true)
	if err != nil {
		if pdfPath != "" {
			os.Remove(filepath.Join(s.uploadDir, pdfPath))
		}
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	paper, err := s.store.GetPaper(id)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusCreated, paper)
}

func (s *Server) applyRelations(id int64, in models.PaperInput) error {
	var tagIDs []int64
	if len(in.TagNames) > 0 {
		tags, err := s.ensureTags(in.TagNames)
		if err != nil {
			return err
		}
		tagIDs = s.tagIDs(tags)
	} else {
		tagIDs = in.Tags
	}
	if err := s.store.SetPaperTags(id, tagIDs); err != nil {
		return err
	}
	var colIDs []int64
	if len(in.CollectionNames) > 0 {
		cols, err := s.ensureCollections(in.CollectionNames)
		if err != nil {
			return err
		}
		colIDs = s.collectionIDs(cols)
	} else {
		colIDs = in.Collections
	}
	return s.store.SetPaperCollections(id, colIDs)
}

func (s *Server) handleUpdatePaper(w http.ResponseWriter, r *http.Request) {
	id, ok := s.idParam(r, "id")
	if !ok {
		writeError(w, http.StatusBadRequest, "invalid paper id")
		return
	}
	current, err := s.store.GetPaper(id)
	if err != nil {
		writeError(w, http.StatusNotFound, "paper not found")
		return
	}
	in, isMultipart, err := s.decodeInput(w, r)
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	if strings.TrimSpace(in.Title) == "" {
		writeError(w, http.StatusBadRequest, "title is required")
		return
	}
	newPDF := false
	staged := "" // 新 PDF 暂存名：数据库提交成功前不删旧文件
	originalPDF := current.PDFPath
	if isMultipart {
		name, size, origName, perr := s.savePdf(r)
		if perr != nil {
			writeError(w, http.StatusBadRequest, perr.Error())
			return
		}
		if name != "" {
			staged = name
			newPDF = true
			meta, merr := pdfmeta.ExtractWithFallback(filepath.Join(s.uploadDir, name), origName)
			if merr != nil {
				os.Remove(filepath.Join(s.uploadDir, name))
				writeError(w, http.StatusBadRequest, "无法解析 PDF 元数据: "+merr.Error())
				return
			}
			current.PDFPath = name
			current.PDFSize = size
			if in.Title == "" {
				in.Title = meta.Title
			}
			if in.Authors == "" {
				in.Authors = meta.Author
			}
			if in.Keywords == "" {
				in.Keywords = meta.Keywords
			}
			current.FullText, _ = pdfmeta.ExtractText(filepath.Join(s.uploadDir, name), 200000)
		}
	}
	if current.FullText == "" {
		current.FullText = s.store.PaperFullText(id)
	}
	if newPDF {
		_ = s.maybeEnrich(&in, current.PDFPath)
		if err := s.maybeAIExtract(&in, current.PDFPath); err != nil {
			os.Remove(filepath.Join(s.uploadDir, staged))
			writeError(w, http.StatusInternalServerError, err.Error())
			return
		}
	}
	p := s.paperFromInput(in, &current)
	// oldPDF 取自覆盖前的原始路径：current.PDFPath 已在上方指向暂存文件
	oldPDF := ""
	if staged != "" && originalPDF != "" && originalPDF != staged {
		oldPDF = originalPDF
	}
	if _, err := s.store.SavePaper(&p, in.Tags, in.Collections, false); err != nil {
		os.Remove(filepath.Join(s.uploadDir, staged))
		if errors.Is(err, store.ErrConflict) {
			writeError(w, http.StatusConflict, err.Error())
		} else {
			writeError(w, http.StatusInternalServerError, err.Error())
		}
		return
	}
	// 数据库提交成功后才替换旧文件
	if oldPDF != "" {
		os.Remove(filepath.Join(s.uploadDir, oldPDF))
	}
	paper, err := s.store.GetPaper(id)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, paper)
}

func (s *Server) handleDeletePaper(w http.ResponseWriter, r *http.Request) {
	id, ok := s.idParam(r, "id")
	if !ok {
		writeError(w, http.StatusBadRequest, "invalid paper id")
		return
	}
	pdfPath, err := s.store.DeletePaper(id)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	if pdfPath != "" {
		os.Remove(filepath.Join(s.uploadDir, pdfPath))
	}
	writeJSON(w, http.StatusOK, map[string]bool{"ok": true})
}

func (s *Server) handleGetPDF(w http.ResponseWriter, r *http.Request) {
	id, ok := s.idParam(r, "id")
	if !ok {
		writeError(w, http.StatusBadRequest, "invalid paper id")
		return
	}
	p, err := s.store.GetPaper(id)
	if err != nil || !p.HasPDF || p.PDFPath == "" {
		writeError(w, http.StatusNotFound, "pdf not found")
		return
	}
	full := filepath.Join(s.uploadDir, p.PDFPath)
	if _, err := os.Stat(full); err != nil {
		writeError(w, http.StatusNotFound, "pdf file missing on disk")
		return
	}
	w.Header().Set("Content-Type", "application/pdf")
	disp := "inline"
	if r.URL.Query().Get("download") == "1" {
		disp = "attachment"
	}
	w.Header().Set("Content-Disposition", disp+"; filename=\"paper-"+strconv.FormatInt(id, 10)+".pdf\"")
	http.ServeFile(w, r, full)
}

func (s *Server) handleToggleRead(w http.ResponseWriter, r *http.Request) {
	s.toggleFlag(w, r, true)
}

func (s *Server) handleToggleStar(w http.ResponseWriter, r *http.Request) {
	s.toggleFlag(w, r, false)
}

func (s *Server) toggleFlag(w http.ResponseWriter, r *http.Request, read bool) {
	id, ok := s.idParam(r, "id")
	if !ok {
		writeError(w, http.StatusBadRequest, "invalid paper id")
		return
	}
	p, err := s.store.GetPaper(id)
	if err != nil {
		writeError(w, http.StatusNotFound, "paper not found")
		return
	}
	if read {
		next := "read"
		switch p.Status {
		case "unread":
			next = "reading"
		case "reading":
			next = "read"
		default:
			next = "unread"
		}
		err = s.store.SetPaperStatus(id, next)
	} else {
		err = s.store.SetPaperStarred(id, !p.Starred)
	}
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	paper, _ := s.store.GetPaper(id)
	writeJSON(w, http.StatusOK, paper)
}

func (s *Server) handleAddPaperTag(w http.ResponseWriter, r *http.Request) {
	id, ok := s.idParam(r, "id")
	if !ok {
		writeError(w, http.StatusBadRequest, "invalid paper id")
		return
	}
	var body struct {
		TagID int64
		Name  string
	}
	if err := readJSON(r, &body); err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	if body.TagID == 0 && strings.TrimSpace(body.Name) == "" {
		writeError(w, http.StatusBadRequest, "tagId or name required")
		return
	}
	var tagID int64
	if body.TagID != 0 {
		tagID = body.TagID
	} else {
		t, err := s.store.EnsureTag(body.Name)
		if err != nil {
			writeError(w, http.StatusInternalServerError, err.Error())
			return
		}
		tagID = t.ID
	}
	if err := s.store.AddPaperTag(id, tagID); err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	paper, _ := s.store.GetPaper(id)
	writeJSON(w, http.StatusOK, paper)
}

func (s *Server) handleRemovePaperTag(w http.ResponseWriter, r *http.Request) {
	id, ok := s.idParam(r, "id")
	if !ok {
		writeError(w, http.StatusBadRequest, "invalid paper id")
		return
	}
	tagID, ok := s.idParam(r, "tagID")
	if !ok {
		writeError(w, http.StatusBadRequest, "invalid tag id")
		return
	}
	if err := s.store.RemovePaperTag(id, tagID); err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	paper, _ := s.store.GetPaper(id)
	writeJSON(w, http.StatusOK, paper)
}

func (s *Server) handleAddPaperCollection(w http.ResponseWriter, r *http.Request) {
	id, ok := s.idParam(r, "id")
	if !ok {
		writeError(w, http.StatusBadRequest, "invalid paper id")
		return
	}
	var body struct {
		CollectionID int64
		Name         string
	}
	if err := readJSON(r, &body); err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	var colID int64
	if body.CollectionID != 0 {
		colID = body.CollectionID
	} else if strings.TrimSpace(body.Name) != "" {
		c, err := s.store.EnsureCollection(body.Name, "")
		if err != nil {
			writeError(w, http.StatusInternalServerError, err.Error())
			return
		}
		colID = c.ID
	} else {
		writeError(w, http.StatusBadRequest, "collectionId or name required")
		return
	}
	if err := s.store.AddPaperCollection(id, colID); err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	paper, _ := s.store.GetPaper(id)
	writeJSON(w, http.StatusOK, paper)
}

func (s *Server) handleRemovePaperCollection(w http.ResponseWriter, r *http.Request) {
	id, ok := s.idParam(r, "id")
	if !ok {
		writeError(w, http.StatusBadRequest, "invalid paper id")
		return
	}
	colID, ok := s.idParam(r, "collectionID")
	if !ok {
		writeError(w, http.StatusBadRequest, "invalid collection id")
		return
	}
	if err := s.store.RemovePaperCollection(id, colID); err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	paper, _ := s.store.GetPaper(id)
	writeJSON(w, http.StatusOK, paper)
}

func (s *Server) decodeJSONOnly(w http.ResponseWriter, r *http.Request, v any) bool {
	if err := json.NewDecoder(r.Body).Decode(v); err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return false
	}
	return true
}
