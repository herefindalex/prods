package webapp

import (
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/csv"
	"encoding/hex"
	"encoding/json"
	"errors"
	"io"
	"mime/multipart"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"strings"

	"prods/internal/catalog"
	"prods/internal/identity"
	"prods/internal/importing"
)

const (
	maxImportUploadBytes = 32 << 20
	maxTemplateBytes     = 64 << 10
)

func (s *Server) adminCreateImportJob(w http.ResponseWriter, r *http.Request) {
	current, ok := s.requireCapability(w, r, identity.CapabilityCatalogImport, true)
	if !ok {
		return
	}
	if s.config.EnablePOCAdmin || current.UserID == "" {
		http.Error(w, "formal import is unavailable in PoC fixture mode", http.StatusForbidden)
		return
	}
	reservation, err := s.admitResource(r.Context(), "import upload and preview", s.config.WorkDir, uint64(maxImportUploadBytes)*4, 8)
	if err != nil {
		s.writeResourceError(w, err)
		return
	}
	defer func() {
		if reservation != nil {
			reservation.Release()
		}
	}()
	err = os.MkdirAll(s.config.WorkDir, 0o700)
	if err != nil {
		s.internalError(w, err)
		return
	}
	temporaryDir, err := os.MkdirTemp(s.config.WorkDir, "import-upload-")
	if err != nil {
		s.internalError(w, err)
		return
	}
	keepTemporary := false
	defer func() {
		if !keepTemporary {
			_ = os.RemoveAll(temporaryDir)
		}
	}()

	r.Body = http.MaxBytesReader(w, r.Body, maxImportUploadBytes+maxTemplateBytes+(1<<20))
	reader, err := r.MultipartReader()
	if err != nil {
		http.Error(w, "multipart form required", http.StatusBadRequest)
		return
	}
	uploadPath := filepath.Join(temporaryDir, "upload.xlsx")
	filename, checksum, snapshot, err := readImportMultipart(reader, uploadPath)
	if err != nil {
		http.Error(w, err.Error(), http.StatusUnprocessableEntity)
		return
	}
	job, err := s.store.CreateImportJob(r.Context(), current.UserID, filename, checksum, snapshot)
	if err != nil {
		s.writeImportError(w, err)
		return
	}
	jobsRoot := filepath.Join(s.config.WorkDir, "jobs")
	if err := os.MkdirAll(jobsRoot, 0o700); err != nil {
		_ = s.store.FailImportJob(r.Context(), job.ID, "upload", "could not prepare the private job directory")
		s.internalError(w, err)
		return
	}
	jobDir := filepath.Join(jobsRoot, job.ID)
	if err := os.Rename(temporaryDir, jobDir); err != nil {
		_ = s.store.FailImportJob(r.Context(), job.ID, "upload", "could not activate the private upload")
		s.internalError(w, err)
		return
	}
	keepTemporary = true
	ctx, cancel := context.WithCancel(context.Background())
	s.importMu.Lock()
	if s.closing {
		s.importMu.Unlock()
		cancel()
		_ = s.store.FailImportJob(context.Background(), job.ID, "shutdown", "server is shutting down")
		http.Error(w, "server is shutting down", http.StatusServiceUnavailable)
		return
	}
	s.importCancels[job.ID] = cancel
	s.importWG.Add(1)
	s.importMu.Unlock()
	workerReservation := reservation
	reservation = nil
	go func() {
		defer s.importWG.Done()
		defer workerReservation.Release()
		s.processImportJob(ctx, job, filepath.Join(jobDir, "upload.xlsx"), jobDir)
	}()
	writeJSON(w, http.StatusAccepted, job)
}

func readImportMultipart(reader *multipart.Reader, uploadPath string) (string, string, importing.TemplateSnapshot, error) {
	var filename string
	var checksum string
	var snapshot importing.TemplateSnapshot
	var templateSeen, fileSeen bool
	for {
		part, err := reader.NextPart()
		if errors.Is(err, io.EOF) {
			break
		}
		if err != nil {
			return "", "", snapshot, err
		}
		switch part.FormName() {
		case "template":
			encoded, err := io.ReadAll(io.LimitReader(part, maxTemplateBytes+1))
			if err != nil || len(encoded) > maxTemplateBytes || json.Unmarshal(encoded, &snapshot) != nil {
				part.Close()
				return "", "", snapshot, errors.New("invalid import template snapshot")
			}
			templateSeen = true
		case "file":
			filename = filepath.Base(part.FileName())
			if filename == "." || filename == "" || !strings.EqualFold(filepath.Ext(filename), ".xlsx") {
				part.Close()
				return "", "", snapshot, errors.New("an .xlsx file is required")
			}
			file, err := os.OpenFile(uploadPath, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0o600)
			if err != nil {
				part.Close()
				return "", "", snapshot, err
			}
			hash := sha256.New()
			written, copyErr := io.Copy(io.MultiWriter(file, hash), io.LimitReader(part, maxImportUploadBytes+1))
			syncErr := file.Sync()
			closeErr := file.Close()
			if copyErr != nil || syncErr != nil || closeErr != nil || written > maxImportUploadBytes {
				part.Close()
				return "", "", snapshot, errors.New("upload could not be saved within the configured limit")
			}
			fileSeen = true
			part.Close()
			checksum = hex.EncodeToString(hash.Sum(nil))
		default:
			_, _ = io.Copy(io.Discard, io.LimitReader(part, maxTemplateBytes+1))
		}
		part.Close()
	}
	if !fileSeen || !templateSeen || snapshot.HeaderRow < 1 || len(snapshot.Mappings) == 0 {
		return "", "", snapshot, errors.New("file and complete template snapshot are required")
	}
	return filename, checksum, snapshot, nil
}

func (s *Server) processImportJob(ctx context.Context, job importing.Job, uploadPath, jobDir string) {
	defer func() {
		s.importMu.Lock()
		delete(s.importCancels, job.ID)
		s.importMu.Unlock()
		_ = os.Remove(uploadPath)
	}()
	if err := s.store.MarkImportJobParsing(ctx, job.ID); err != nil {
		return
	}
	file, err := os.Open(uploadPath)
	if err != nil {
		_ = s.store.FailImportJob(context.Background(), job.ID, "parse", "the private upload could not be opened")
		return
	}
	workbook, readErr := importing.ReadXLSXContext(ctx, file, job.TemplateSnapshot.Sheet, job.TemplateSnapshot.HeaderRow,
		importing.Limits{MaxBytes: maxImportUploadBytes, MaxRows: 25_000, MaxColumns: 256})
	_ = file.Close()
	if ctx.Err() != nil {
		return
	}
	if readErr != nil {
		_ = s.store.FailImportJob(context.Background(), job.ID, "parse", readErr.Error())
		return
	}
	preview, err := s.store.BuildImportPreview(ctx, job.ActorID, workbook, job.TemplateSnapshot.Mappings, job.TemplateSnapshot.IdentityMode)
	if err != nil {
		_ = s.store.FailImportJob(context.Background(), job.ID, "validate", err.Error())
		return
	}
	preview.OperationID = job.ID
	previewPath := filepath.Join(jobDir, "preview.json")
	reportPath := filepath.Join(jobDir, "errors.csv")
	if err := writeImportPreview(previewPath, preview); err != nil {
		_ = s.store.FailImportJob(context.Background(), job.ID, "preview", "the preview could not be saved")
		return
	}
	if err := writeImportReport(reportPath, preview.Issues); err != nil {
		_ = s.store.FailImportJob(context.Background(), job.ID, "report", "the error report could not be saved")
		return
	}
	if err := s.store.SaveImportJobPreview(ctx, job.ID, previewPath, reportPath, preview); err != nil && ctx.Err() == nil {
		_ = s.store.FailImportJob(context.Background(), job.ID, "preview", err.Error())
	}
}

func writeImportPreview(path string, preview importing.Preview) error {
	temporary := path + ".tmp"
	file, err := os.OpenFile(temporary, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0o600)
	if err != nil {
		return err
	}
	encoder := json.NewEncoder(file)
	err = encoder.Encode(preview)
	if err == nil {
		err = file.Sync()
	}
	if closeErr := file.Close(); err == nil {
		err = closeErr
	}
	if err != nil {
		_ = os.Remove(temporary)
		return err
	}
	return os.Rename(temporary, path)
}

func writeImportReport(path string, issues []importing.RowIssue) error {
	file, err := os.OpenFile(path, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0o600)
	if err != nil {
		return err
	}
	writer := csv.NewWriter(file)
	_ = writer.Write([]string{"Sheet", "Row", "Column", "Value", "Code", "Message"})
	for _, issue := range issues {
		_ = writer.Write([]string{issue.Sheet, strconv.Itoa(issue.Row), issue.Column, issue.Value, issue.Code, issue.Message})
	}
	writer.Flush()
	if err := writer.Error(); err != nil {
		file.Close()
		return err
	}
	if err := file.Sync(); err != nil {
		file.Close()
		return err
	}
	return file.Close()
}

func (s *Server) adminImportJob(w http.ResponseWriter, r *http.Request) {
	if _, ok := s.requireCapability(w, r, identity.CapabilityCatalogImport, false); !ok {
		return
	}
	job, err := s.store.ImportJob(r.Context(), r.PathValue("id"))
	if err != nil {
		s.writeImportError(w, err)
		return
	}
	response := struct {
		Job     importing.Job      `json:"job"`
		Preview *importing.Preview `json:"preview,omitempty"`
	}{Job: job}
	if job.PreviewPath != "" {
		var preview importing.Preview
		if err := s.readPrivateJobJSON(job.PreviewPath, &preview); err == nil {
			response.Preview = &preview
		}
	}
	writeJSON(w, http.StatusOK, response)
}

func (s *Server) adminImportReport(w http.ResponseWriter, r *http.Request) {
	if _, ok := s.requireCapability(w, r, identity.CapabilityCatalogImport, false); !ok {
		return
	}
	job, err := s.store.ImportJob(r.Context(), r.PathValue("id"))
	if err != nil {
		s.writeImportError(w, err)
		return
	}
	path, err := s.safeJobPath(job.ReportPath)
	if err != nil {
		http.NotFound(w, r)
		return
	}
	w.Header().Set("Content-Disposition", `attachment; filename="import-errors.csv"`)
	http.ServeFile(w, r, path)
}

func (s *Server) adminCommitImport(w http.ResponseWriter, r *http.Request) {
	current, ok := s.requireCapability(w, r, identity.CapabilityCatalogImport, true)
	if !ok {
		return
	}
	job, err := s.store.ImportJob(r.Context(), r.PathValue("id"))
	if err != nil {
		s.writeImportError(w, err)
		return
	}
	var preview importing.Preview
	if err := s.readPrivateJobJSON(job.PreviewPath, &preview); err != nil {
		http.Error(w, "import preview is unavailable", http.StatusConflict)
		return
	}
	if err := s.store.BeginImportCommit(r.Context(), current.UserID, job.ID); err != nil {
		s.writeImportError(w, err)
		return
	}
	receipt, err := s.store.CommitImport(r.Context(), current.UserID, preview)
	if err != nil {
		_ = s.store.FailImportJob(context.Background(), job.ID, "final_commit", err.Error())
		s.writeImportError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, receipt)
}

func (s *Server) adminCancelImport(w http.ResponseWriter, r *http.Request) {
	current, ok := s.requireCapability(w, r, identity.CapabilityCatalogImport, true)
	if !ok {
		return
	}
	if err := s.store.RequestImportCancel(r.Context(), current.UserID, r.PathValue("id")); err != nil {
		s.writeImportError(w, err)
		return
	}
	s.importMu.Lock()
	cancel := s.importCancels[r.PathValue("id")]
	s.importMu.Unlock()
	if cancel != nil {
		cancel()
	}
	w.WriteHeader(http.StatusNoContent)
}

func (s *Server) adminSaveImportTemplate(w http.ResponseWriter, r *http.Request) {
	current, ok := s.requireCapability(w, r, identity.CapabilityCatalogImport, true)
	if !ok {
		return
	}
	var request struct {
		ExpectedVersion int64              `json:"expected_version"`
		Template        importing.Template `json:"template"`
	}
	if err := decodeJSON(r.Body, &request); err != nil {
		http.Error(w, "invalid JSON", http.StatusBadRequest)
		return
	}
	template, err := s.store.SaveImportTemplate(r.Context(), current.UserID, request.ExpectedVersion, request.Template)
	if err != nil {
		s.writeImportError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, template)
}

func (s *Server) adminImportTemplate(w http.ResponseWriter, r *http.Request) {
	if _, ok := s.requireCapability(w, r, identity.CapabilityCatalogImport, false); !ok {
		return
	}
	version, _ := strconv.ParseInt(r.URL.Query().Get("version"), 10, 64)
	template, err := s.store.ImportTemplate(r.Context(), r.PathValue("id"), version)
	if err != nil {
		s.writeImportError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, template)
}

func (s *Server) readPrivateJobJSON(path string, target any) error {
	path, err := s.safeJobPath(path)
	if err != nil {
		return err
	}
	file, err := os.Open(path)
	if err != nil {
		return err
	}
	defer file.Close()
	return json.NewDecoder(io.LimitReader(file, maxImportUploadBytes)).Decode(target)
}

func (s *Server) safeJobPath(path string) (string, error) {
	if path == "" {
		return "", errors.New("empty job path")
	}
	root, err := filepath.Abs(filepath.Join(s.config.WorkDir, "jobs"))
	if err != nil {
		return "", err
	}
	resolved, err := filepath.Abs(path)
	if err != nil {
		return "", err
	}
	relative, err := filepath.Rel(root, resolved)
	if err != nil || relative == "." || relative == ".." || strings.HasPrefix(relative, ".."+string(filepath.Separator)) {
		return "", errors.New("job path escapes private work root")
	}
	return resolved, nil
}

func (s *Server) writeImportError(w http.ResponseWriter, err error) {
	switch {
	case errors.Is(err, os.ErrNotExist), errors.Is(err, sql.ErrNoRows):
		http.Error(w, "import not found", http.StatusNotFound)
	case errors.Is(err, catalog.ErrRevisionConflict), errors.Is(err, importing.ErrImportConflict),
		errors.Is(err, importing.ErrImportReceiptConflict), errors.Is(err, importing.ErrInvalidImport):
		writeJSON(w, http.StatusConflict, map[string]string{"error": err.Error()})
	case errors.Is(err, catalog.ErrInvalidProduct), errors.Is(err, catalog.ErrInvalidDictionary), errors.Is(err, importing.ErrInvalidMapping):
		writeJSON(w, http.StatusUnprocessableEntity, map[string]string{"error": err.Error()})
	default:
		s.internalError(w, err)
	}
}
