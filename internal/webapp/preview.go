package webapp

import (
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"

	"prods/internal/catalog"
	"prods/internal/identity"
	"prods/internal/publishing"
)

const (
	productPreviewTTL      = 30 * time.Minute
	maxProductPreviewBytes = 5 << 20
)

type productPreviewEnvelope struct {
	ExpiresAt time.Time `json:"expires_at"`
	HTML      []byte    `json:"html"`
}

func (s *Server) adminCreateProductPreview(w http.ResponseWriter, r *http.Request) {
	if _, ok := s.requireCapability(w, r, identity.CapabilityCatalogEdit, true); !ok {
		return
	}
	reservation, err := s.admitResource(r.Context(), "product preview", s.config.WorkDir, maxProductPreviewBytes, 2)
	if err != nil {
		s.writeResourceError(w, r, err)
		return
	}
	defer reservation.Release()
	var request struct {
		ExpectedRevision int64           `json:"expected_revision"`
		Product          catalog.Product `json:"product"`
	}
	if err := decodeJSON(r.Body, &request); err != nil {
		s.writeAPIError(w, r, http.StatusBadRequest, apiCodeInvalidJSON)
		return
	}
	if request.Product.ID != "" {
		current, err := s.store.Product(r.Context(), request.Product.ID)
		if err != nil {
			s.writeCatalogError(w, r, err)
			return
		}
		if current.Revision != request.ExpectedRevision {
			s.writeCatalogError(w, r, catalog.ErrRevisionConflict)
			return
		}
	} else {
		request.Product.ID = "preview"
	}
	request.Product.Status = catalog.Published
	request.Product.RecordState = catalog.RecordCurrent
	if request.Product.Revision < 1 {
		request.Product.Revision = 1
	}
	if err := request.Product.Prepare(); err != nil {
		s.writeCatalogError(w, r, err)
		return
	}
	defaultLocale, err := s.store.DefaultLocale(r.Context())
	if err != nil {
		s.internalAPIError(w, r, err)
		return
	}
	view := publishing.PublicView{
		ID: request.Product.ID, Revision: request.Product.Revision, PartNumber: request.Product.PartNumber,
		Name: request.Product.Name, Manufacturer: request.Product.Manufacturer, Brand: request.Product.Brand,
		Description: request.Product.Description, Features: request.Product.Features, Specification: request.Product.Specification,
		Language: defaultLocale,
	}
	if request.Product.DocumentURL != "" {
		view.Documents = []publishing.Document{{Label: "Datasheet", URL: request.Product.DocumentURL}}
	}
	body, err := publishing.HTML(view)
	if err != nil {
		s.internalAPIError(w, r, err)
		return
	}
	token, err := secureToken(32)
	if err != nil {
		s.internalAPIError(w, r, err)
		return
	}
	expiresAt := time.Now().UTC().Add(productPreviewTTL)
	envelope, err := json.Marshal(productPreviewEnvelope{ExpiresAt: expiresAt, HTML: body})
	if err != nil {
		s.internalAPIError(w, r, err)
		return
	}
	root := filepath.Join(s.config.WorkDir, "preview")
	if err := os.MkdirAll(root, 0o700); err != nil {
		s.internalAPIError(w, r, err)
		return
	}
	path := filepath.Join(root, token+".json")
	file, err := os.OpenFile(path, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0o600)
	if err != nil {
		s.internalAPIError(w, r, err)
		return
	}
	if _, err := file.Write(envelope); err != nil {
		file.Close()
		s.internalAPIError(w, r, err)
		return
	}
	if err := file.Sync(); err != nil {
		file.Close()
		s.internalAPIError(w, r, err)
		return
	}
	if err := file.Close(); err != nil {
		s.internalAPIError(w, r, err)
		return
	}
	writeJSON(w, http.StatusCreated, map[string]any{"url": "/admin/previews/" + token, "expires_at": expiresAt})
}

func (s *Server) adminProductPreview(w http.ResponseWriter, r *http.Request) {
	if _, ok := s.requireCapability(w, r, identity.CapabilityCatalogView, false); !ok {
		return
	}
	token := r.PathValue("token")
	if token == "" || strings.ContainsAny(token, `/\.`) {
		http.NotFound(w, r)
		return
	}
	path := filepath.Join(s.config.WorkDir, "preview", token+".json")
	file, err := os.Open(path)
	if errors.Is(err, os.ErrNotExist) {
		http.NotFound(w, r)
		return
	}
	if err != nil {
		s.internalAPIError(w, r, err)
		return
	}
	defer file.Close()
	var envelope productPreviewEnvelope
	if err := json.NewDecoder(io.LimitReader(file, maxProductPreviewBytes)).Decode(&envelope); err != nil || len(envelope.HTML) == 0 {
		http.NotFound(w, r)
		return
	}
	if !time.Now().UTC().Before(envelope.ExpiresAt) {
		_ = os.Remove(path)
		http.NotFound(w, r)
		return
	}
	w.Header().Set("Cache-Control", "private, no-store")
	w.Header().Set("X-Robots-Tag", "noindex, nofollow, noarchive")
	w.Header().Set("Content-Security-Policy", "default-src 'self'; img-src 'self' data:; style-src 'self' 'unsafe-inline'; script-src 'self'")
	writeBody(w, "text/html; charset=utf-8", envelope.HTML)
}

func (s *Server) savePrivatePreviewHTML(body []byte) (string, time.Time, error) {
	token, err := secureToken(32)
	if err != nil {
		return "", time.Time{}, err
	}
	expiresAt := time.Now().UTC().Add(productPreviewTTL)
	envelope, err := json.Marshal(productPreviewEnvelope{ExpiresAt: expiresAt, HTML: body})
	if err != nil {
		return "", time.Time{}, err
	}
	root := filepath.Join(s.config.WorkDir, "preview")
	if err := os.MkdirAll(root, 0o700); err != nil {
		return "", time.Time{}, err
	}
	path := filepath.Join(root, token+".json")
	file, err := os.OpenFile(path, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0o600)
	if err != nil {
		return "", time.Time{}, err
	}
	if _, err := file.Write(envelope); err != nil {
		_ = file.Close()
		_ = os.Remove(path)
		return "", time.Time{}, err
	}
	if err := file.Sync(); err != nil {
		_ = file.Close()
		_ = os.Remove(path)
		return "", time.Time{}, err
	}
	if err := file.Close(); err != nil {
		_ = os.Remove(path)
		return "", time.Time{}, err
	}
	return token, expiresAt, nil
}

func (s *Server) servePrivatePreviewHTML(w http.ResponseWriter, r *http.Request, token string) {
	if token == "" || strings.ContainsAny(token, `/\.`) {
		http.NotFound(w, r)
		return
	}
	path := filepath.Join(s.config.WorkDir, "preview", token+".json")
	file, err := os.Open(path)
	if errors.Is(err, os.ErrNotExist) {
		http.NotFound(w, r)
		return
	}
	if err != nil {
		s.internalAPIError(w, r, err)
		return
	}
	defer file.Close()
	var envelope productPreviewEnvelope
	if err := json.NewDecoder(io.LimitReader(file, maxProductPreviewBytes)).Decode(&envelope); err != nil || len(envelope.HTML) == 0 {
		http.NotFound(w, r)
		return
	}
	if !time.Now().UTC().Before(envelope.ExpiresAt) {
		_ = os.Remove(path)
		http.NotFound(w, r)
		return
	}
	w.Header().Set("Cache-Control", "private, no-store")
	w.Header().Set("X-Robots-Tag", "noindex, nofollow, noarchive")
	w.Header().Set("Content-Security-Policy", "default-src 'self'; img-src 'self' data:; style-src 'self' 'unsafe-inline'; script-src 'self'")
	writeBody(w, "text/html; charset=utf-8", envelope.HTML)
}
