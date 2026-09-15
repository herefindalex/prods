package webapp

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/xml"
	"errors"
	"image/jpeg"
	"image/png"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"

	"golang.org/x/image/webp"

	"prods/internal/catalog"
	"prods/internal/identity"
)

const (
	maxWebsiteImageBytes  int64 = 8 << 20
	maxWebsiteSVGBytes    int64 = 2 << 20
	maxWebsiteImageSide         = 8192
	maxWebsiteImagePixels       = 40_000_000
)

func (s *Server) adminUploadWebsiteAsset(w http.ResponseWriter, r *http.Request) {
	current, ok := s.requireCapability(w, r, identity.CapabilitySystemManage, true)
	if !ok {
		return
	}
	reservation, err := s.admitResource(r.Context(), "website asset upload", s.config.AssetDir, uint64(maxWebsiteImageBytes), 3)
	if err != nil {
		s.writeResourceError(w, err)
		return
	}
	defer reservation.Release()
	r.Body = http.MaxBytesReader(w, r.Body, maxWebsiteImageBytes+(1<<20))
	reader, err := r.MultipartReader()
	if err != nil {
		http.Error(w, "multipart upload required", http.StatusBadRequest)
		return
	}
	part, err := reader.NextPart()
	if err != nil || part.FormName() != "file" || part.FileName() == "" {
		http.Error(w, "one file field is required", http.StatusBadRequest)
		return
	}
	defer part.Close()
	filename := filepath.Base(part.FileName())
	extension := strings.ToLower(filepath.Ext(filename))
	mimeType, canonicalExtension, limit := websiteImageType(extension)
	if mimeType == "" {
		http.Error(w, "website image must be JPEG, PNG, WebP, or safe SVG", http.StatusUnprocessableEntity)
		return
	}

	assetID, err := secureToken(24)
	if err != nil {
		s.internalError(w, err)
		return
	}
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
	size, err := io.Copy(temporary, io.LimitReader(part, limit+1))
	if err != nil {
		var tooLarge *http.MaxBytesError
		if errors.As(err, &tooLarge) {
			http.Error(w, "website image exceeds upload limit", http.StatusRequestEntityTooLarge)
			return
		}
		s.internalError(w, err)
		return
	}
	if size == 0 || size > limit {
		http.Error(w, "website image is empty or exceeds upload limit", http.StatusRequestEntityTooLarge)
		return
	}
	if err := validateAndSanitizeWebsiteImage(temporary, extension, mimeType); err != nil {
		writeJSON(w, http.StatusUnprocessableEntity, map[string]string{"error": err.Error()})
		return
	}
	info, err := temporary.Stat()
	if err != nil {
		s.internalError(w, err)
		return
	}
	size = info.Size()
	if _, err := temporary.Seek(0, io.SeekStart); err != nil {
		s.internalError(w, err)
		return
	}
	hasher := sha256.New()
	if _, err := io.Copy(hasher, temporary); err != nil {
		s.internalError(w, err)
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
	storagePath := filepath.Join("website", assetID, "content"+canonicalExtension)
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
		ID: assetID, OwnerType: "website", OwnerID: "working", OriginalFilename: filename,
		StoragePath: filepath.ToSlash(storagePath), MIMEType: mimeType, SizeBytes: size,
		Checksum: hex.EncodeToString(hasher.Sum(nil)),
	})
	if err != nil {
		s.writeCatalogError(w, err)
		return
	}
	writeJSON(w, http.StatusCreated, asset)
}

func (s *Server) adminWebsiteAsset(w http.ResponseWriter, r *http.Request) {
	if _, ok := s.requireCapability(w, r, identity.CapabilitySystemManage, false); !ok {
		return
	}
	asset, err := s.store.WebsiteAsset(r.Context(), r.PathValue("id"))
	if err != nil {
		http.NotFound(w, r)
		return
	}
	root, err := filepath.Abs(s.config.AssetDir)
	if err != nil {
		s.internalError(w, err)
		return
	}
	resolvedRoot, err := filepath.EvalSymlinks(root)
	if err != nil {
		s.internalError(w, err)
		return
	}
	path, err := filepath.EvalSymlinks(filepath.Join(root, filepath.Clean(asset.StoragePath)))
	if err != nil {
		http.NotFound(w, r)
		return
	}
	relative, err := filepath.Rel(resolvedRoot, path)
	if err != nil || relative == ".." || strings.HasPrefix(relative, ".."+string(filepath.Separator)) {
		http.NotFound(w, r)
		return
	}
	file, err := os.Open(path)
	if err != nil {
		http.NotFound(w, r)
		return
	}
	defer file.Close()
	info, err := file.Stat()
	if err != nil || !info.Mode().IsRegular() || info.Size() != asset.SizeBytes {
		http.NotFound(w, r)
		return
	}
	hasher := sha256.New()
	if _, err := io.Copy(hasher, file); err != nil || hex.EncodeToString(hasher.Sum(nil)) != asset.Checksum {
		http.NotFound(w, r)
		return
	}
	if _, err := file.Seek(0, io.SeekStart); err != nil {
		s.internalError(w, err)
		return
	}
	w.Header().Set("Cache-Control", "private, no-store")
	w.Header().Set("Content-Type", asset.MIMEType)
	http.ServeContent(w, r, asset.OriginalFilename, asset.CreatedAt, file)
}

func websiteImageType(extension string) (mimeType, canonicalExtension string, limit int64) {
	switch extension {
	case ".jpg", ".jpeg":
		return "image/jpeg", ".jpg", maxWebsiteImageBytes
	case ".png":
		return "image/png", ".png", maxWebsiteImageBytes
	case ".webp":
		return "image/webp", ".webp", maxWebsiteImageBytes
	case ".svg":
		return "image/svg+xml", ".svg", maxWebsiteSVGBytes
	default:
		return "", "", 0
	}
}

func validateAndSanitizeWebsiteImage(file *os.File, extension, mimeType string) error {
	if _, err := file.Seek(0, io.SeekStart); err != nil {
		return err
	}
	if extension == ".svg" {
		body, err := io.ReadAll(io.LimitReader(file, maxWebsiteSVGBytes+1))
		if err != nil {
			return err
		}
		sanitized, err := sanitizeSVG(body)
		if err != nil {
			return err
		}
		if err := file.Truncate(0); err != nil {
			return err
		}
		if _, err := file.Seek(0, io.SeekStart); err != nil {
			return err
		}
		_, err = file.Write(sanitized)
		return err
	}
	prefix := make([]byte, 512)
	n, err := file.Read(prefix)
	if err != nil && !errors.Is(err, io.EOF) {
		return err
	}
	if http.DetectContentType(prefix[:n]) != mimeType {
		return errors.New("file content does not match its image extension")
	}
	if _, err := file.Seek(0, io.SeekStart); err != nil {
		return err
	}
	var width, height int
	switch mimeType {
	case "image/jpeg":
		configuration, err := jpeg.DecodeConfig(file)
		if err != nil {
			return errors.New("invalid JPEG image")
		}
		width, height = configuration.Width, configuration.Height
	case "image/png":
		configuration, err := png.DecodeConfig(file)
		if err != nil {
			return errors.New("invalid PNG image")
		}
		width, height = configuration.Width, configuration.Height
	case "image/webp":
		configuration, err := webp.DecodeConfig(file)
		if err != nil {
			return errors.New("invalid WebP image")
		}
		width, height = configuration.Width, configuration.Height
	}
	if width < 1 || height < 1 || width > maxWebsiteImageSide || height > maxWebsiteImageSide || int64(width)*int64(height) > maxWebsiteImagePixels {
		return errors.New("image dimensions exceed the safe decode bounds")
	}
	return nil
}

func sanitizeSVG(body []byte) ([]byte, error) {
	allowedElements := map[string]bool{
		"svg": true, "g": true, "path": true, "rect": true, "circle": true, "ellipse": true,
		"line": true, "polyline": true, "polygon": true, "title": true, "desc": true,
		"defs": true, "lineargradient": true, "radialgradient": true, "stop": true, "clippath": true,
	}
	allowedAttributes := map[string]bool{
		"xmlns": true, "viewbox": true, "width": true, "height": true, "x": true, "y": true,
		"x1": true, "x2": true, "y1": true, "y2": true, "cx": true, "cy": true, "r": true,
		"rx": true, "ry": true, "d": true, "points": true, "fill": true, "stroke": true,
		"stroke-width": true, "stroke-linecap": true, "stroke-linejoin": true, "fill-rule": true,
		"clip-rule": true, "opacity": true, "fill-opacity": true, "stroke-opacity": true,
		"transform": true, "id": true, "class": true, "offset": true, "stop-color": true, "stop-opacity": true,
	}
	decoder := xml.NewDecoder(bytes.NewReader(body))
	decoder.Strict = true
	var output bytes.Buffer
	encoder := xml.NewEncoder(&output)
	depth := 0
	sawRoot := false
	for {
		token, err := decoder.Token()
		if errors.Is(err, io.EOF) {
			break
		}
		if err != nil {
			return nil, errors.New("invalid SVG XML")
		}
		switch value := token.(type) {
		case xml.StartElement:
			name := strings.ToLower(value.Name.Local)
			if !allowedElements[name] || (!sawRoot && name != "svg") {
				return nil, errors.New("SVG contains an unsupported element")
			}
			if !sawRoot {
				sawRoot = true
			}
			for _, attribute := range value.Attr {
				attributeName := strings.ToLower(attribute.Name.Local)
				attributeValue := strings.ToLower(strings.TrimSpace(attribute.Value))
				if !allowedAttributes[attributeName] || strings.HasPrefix(attributeName, "on") || strings.Contains(attributeValue, "url(") || strings.Contains(attributeValue, "javascript:") || strings.Contains(attributeValue, "data:") || strings.ContainsAny(attribute.Value, "\x00\r\n") {
					return nil, errors.New("SVG contains an unsafe attribute")
				}
			}
			depth++
			if err := encoder.EncodeToken(value); err != nil {
				return nil, err
			}
		case xml.EndElement:
			depth--
			if depth < 0 {
				return nil, errors.New("invalid SVG nesting")
			}
			if err := encoder.EncodeToken(value); err != nil {
				return nil, err
			}
		case xml.CharData:
			if err := encoder.EncodeToken(value); err != nil {
				return nil, err
			}
		case xml.Comment:
			// Comments are not needed in managed logo assets.
		case xml.ProcInst:
			if strings.ToLower(value.Target) != "xml" {
				return nil, errors.New("SVG processing instructions are not allowed")
			}
		case xml.Directive:
			return nil, errors.New("SVG directives are not allowed")
		}
	}
	if !sawRoot || depth != 0 {
		return nil, errors.New("invalid SVG document")
	}
	if err := encoder.Flush(); err != nil {
		return nil, err
	}
	return output.Bytes(), nil
}
