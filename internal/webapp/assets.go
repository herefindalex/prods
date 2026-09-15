package webapp

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"runtime"
	"strings"

	"prods/internal/catalog"
	"prods/internal/identity"
)

const maxProductDocumentBytes int64 = 32 << 20

func (s *Server) adminUploadProductAsset(w http.ResponseWriter, r *http.Request) {
	current, ok := s.requireCapability(w, r, identity.CapabilityCatalogEdit, true)
	if !ok {
		return
	}
	reservation, err := s.admitResource(r.Context(), "product asset upload", s.config.AssetDir, uint64(maxProductDocumentBytes), 3)
	if err != nil {
		s.writeResourceError(w, err)
		return
	}
	defer reservation.Release()
	r.Body = http.MaxBytesReader(w, r.Body, maxProductDocumentBytes+(1<<20))
	reader, err := r.MultipartReader()
	if err != nil {
		http.Error(w, "multipart upload required", http.StatusBadRequest)
		return
	}

	var upload io.ReadCloser
	var filename string
	for {
		part, nextErr := reader.NextPart()
		if errors.Is(nextErr, io.EOF) {
			break
		}
		if nextErr != nil {
			http.Error(w, "invalid multipart upload", http.StatusBadRequest)
			return
		}
		if part.FormName() == "file" && part.FileName() != "" && upload == nil {
			upload = part
			filename = filepath.Base(part.FileName())
			break
		}
		_ = part.Close()
	}
	extension := strings.ToLower(filepath.Ext(filename))
	mimeType, canonicalExtension, limit := productAssetType(extension)
	if upload == nil || filename == "." || mimeType == "" {
		http.Error(w, "one PDF, JPEG, PNG, or WebP file is required", http.StatusUnprocessableEntity)
		return
	}
	defer upload.Close()

	assetToken, err := secureToken(24)
	if err != nil {
		s.internalError(w, err)
		return
	}
	assetID := "ast_" + assetToken
	stagingRoot := filepath.Join(s.config.AssetDir, ".staging")
	if err := os.MkdirAll(stagingRoot, 0o700); err != nil {
		s.internalError(w, err)
		return
	}
	temporaryPath := filepath.Join(stagingRoot, assetID+".upload")
	temporary, err := os.OpenFile(temporaryPath, os.O_CREATE|os.O_EXCL|os.O_RDWR, 0o600)
	if err != nil {
		s.internalError(w, err)
		return
	}
	keepTemporary := true
	defer func() {
		_ = temporary.Close()
		if keepTemporary {
			_ = os.Remove(temporaryPath)
		}
	}()

	hasher := sha256.New()
	size, err := io.Copy(io.MultiWriter(temporary, hasher), io.LimitReader(upload, limit+1))
	if err != nil {
		var maxBytesError *http.MaxBytesError
		if errors.As(err, &maxBytesError) {
			http.Error(w, "product asset exceeds the upload limit", http.StatusRequestEntityTooLarge)
			return
		}
		s.internalError(w, err)
		return
	}
	if size == 0 || size > limit {
		http.Error(w, "product asset is empty or exceeds the upload limit", http.StatusRequestEntityTooLarge)
		return
	}
	if _, err := temporary.Seek(0, io.SeekStart); err != nil {
		s.internalError(w, err)
		return
	}
	if mimeType == "application/pdf" {
		prefix := make([]byte, 512)
		prefixLength, err := temporary.Read(prefix)
		if err != nil && !errors.Is(err, io.EOF) {
			s.internalError(w, err)
			return
		}
		prefix = prefix[:prefixLength]
		if !bytes.HasPrefix(prefix, []byte("%PDF-")) || http.DetectContentType(prefix) != "application/pdf" {
			http.Error(w, "file content is not a PDF", http.StatusUnprocessableEntity)
			return
		}
	} else if err := validateAndSanitizeWebsiteImage(temporary, extension, mimeType); err != nil {
		writeJSON(w, http.StatusUnprocessableEntity, map[string]string{"error": err.Error()})
		return
	}
	if err := temporary.Sync(); err != nil {
		s.internalError(w, err)
		return
	}
	if err := temporary.Close(); err != nil {
		s.internalError(w, err)
		return
	}

	storagePath := filepath.Join("product", r.PathValue("id"), assetID, "content"+canonicalExtension)
	destination := filepath.Join(s.config.AssetDir, storagePath)
	if err := os.MkdirAll(filepath.Dir(destination), 0o700); err != nil {
		s.internalError(w, err)
		return
	}
	if err := os.Rename(temporaryPath, destination); err != nil {
		s.internalError(w, err)
		return
	}
	keepTemporary = false
	if err := syncDirectory(filepath.Dir(destination)); err != nil {
		s.internalError(w, err)
		return
	}

	asset, err := s.store.CreateAsset(r.Context(), current.UserID, catalog.Asset{
		ID:               assetID,
		OwnerType:        "product",
		OwnerID:          r.PathValue("id"),
		OriginalFilename: filename,
		StoragePath:      filepath.ToSlash(storagePath),
		MIMEType:         mimeType,
		SizeBytes:        size,
		Checksum:         hex.EncodeToString(hasher.Sum(nil)),
	})
	if err != nil {
		s.writeCatalogError(w, err)
		return
	}
	writeJSON(w, http.StatusCreated, asset)
}

func productAssetType(extension string) (mimeType, canonicalExtension string, limit int64) {
	if extension == ".pdf" {
		return "application/pdf", ".pdf", maxProductDocumentBytes
	}
	mimeType, canonicalExtension, limit = websiteImageType(extension)
	if mimeType == "image/svg+xml" {
		return "", "", 0
	}
	return mimeType, canonicalExtension, limit
}

func (s *Server) publicAsset(w http.ResponseWriter, r *http.Request) {
	if s.publisher == nil || !s.publisher.ServeAsset(w, r, r.PathValue("id")) {
		http.NotFound(w, r)
	}
}

func (s *Server) adminProductImages(w http.ResponseWriter, r *http.Request) {
	if _, ok := s.requireCapability(w, r, identity.CapabilityCatalogView, false); !ok {
		return
	}
	images, err := s.store.ProductImages(r.Context(), r.PathValue("id"))
	if err != nil {
		s.writeCatalogError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, images)
}

type productImageRequest struct {
	ExpectedRevision int64                `json:"expected_revision"`
	Image            catalog.ProductImage `json:"image"`
}

func (s *Server) adminAddProductImage(w http.ResponseWriter, r *http.Request) {
	current, ok := s.requireCapability(w, r, identity.CapabilityCatalogEdit, true)
	if !ok {
		return
	}
	var request productImageRequest
	if err := decodeJSON(r.Body, &request); err != nil {
		http.Error(w, "invalid JSON", http.StatusBadRequest)
		return
	}
	request.Image.ProductID = r.PathValue("id")
	image, err := s.store.AddProductImage(r.Context(), current.UserID, request.ExpectedRevision, request.Image)
	if err != nil {
		s.writeCatalogError(w, err)
		return
	}
	writeJSON(w, http.StatusCreated, image)
}

func (s *Server) adminUpdateProductImage(w http.ResponseWriter, r *http.Request) {
	current, ok := s.requireCapability(w, r, identity.CapabilityCatalogEdit, true)
	if !ok {
		return
	}
	var request productImageRequest
	if err := decodeJSON(r.Body, &request); err != nil {
		http.Error(w, "invalid JSON", http.StatusBadRequest)
		return
	}
	request.Image.ID = r.PathValue("image_id")
	request.Image.ProductID = r.PathValue("id")
	image, err := s.store.UpdateProductImage(r.Context(), current.UserID, request.ExpectedRevision, request.Image)
	if err != nil {
		s.writeCatalogError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, image)
}

func (s *Server) adminDeleteProductImage(w http.ResponseWriter, r *http.Request) {
	current, ok := s.requireCapability(w, r, identity.CapabilityCatalogEdit, true)
	if !ok {
		return
	}
	var request struct {
		ExpectedRevision int64 `json:"expected_revision"`
	}
	if err := decodeJSON(r.Body, &request); err != nil {
		http.Error(w, "invalid JSON", http.StatusBadRequest)
		return
	}
	if err := s.store.DeleteProductImage(r.Context(), current.UserID, r.PathValue("id"), r.PathValue("image_id"), request.ExpectedRevision); err != nil {
		s.writeCatalogError(w, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func syncDirectory(path string) error {
	if runtime.GOOS == "windows" {
		return nil
	}
	directory, err := os.Open(path)
	if err != nil {
		return err
	}
	defer directory.Close()
	if err := directory.Sync(); err != nil {
		return fmt.Errorf("sync directory: %w", err)
	}
	return nil
}
