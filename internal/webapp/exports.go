package webapp

import (
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"

	"prods/internal/exporting"
	"prods/internal/identity"
)

func (s *Server) adminExportProducts(w http.ResponseWriter, r *http.Request) {
	current, ok := s.requireCapability(w, r, identity.CapabilityCatalogExport, true)
	if !ok {
		return
	}
	s.generateExport(w, r, current.UserID, "product", "xlsx", "prods-products.xlsx",
		"application/vnd.openxmlformats-officedocument.spreadsheetml.sheet",
		func(path string) (int, error) {
			return exporting.ProductsXLSX(r.Context(), s.store, current.UserID, path)
		})
}

func (s *Server) adminExportRFQs(w http.ResponseWriter, r *http.Request) {
	current, ok := s.requireCapability(w, r, identity.CapabilityRFQExport, true)
	if !ok {
		return
	}
	s.generateExport(w, r, current.UserID, "rfq", "csv", "prods-rfqs.csv", "text/csv; charset=utf-8",
		func(path string) (int, error) { return exporting.RFQsCSV(r.Context(), s.store, current.UserID, path) })
}

func (s *Server) generateExport(w http.ResponseWriter, r *http.Request, actorID, kind, format, filename, contentType string,
	generate func(string) (int, error)) {
	reservation, err := s.admitResource(r.Context(), kind+" export", s.config.WorkDir, 64<<20, 16)
	if err != nil {
		s.writeResourceError(w, r, err)
		return
	}
	defer reservation.Release()
	if err := os.MkdirAll(s.config.WorkDir, 0o700); err != nil {
		s.internalAPIError(w, r, err)
		return
	}
	directory, err := os.MkdirTemp(s.config.WorkDir, "export-")
	if err != nil {
		s.internalAPIError(w, r, err)
		return
	}
	defer os.RemoveAll(directory)
	path := filepath.Join(directory, filename)
	rows, err := generate(path)
	if err != nil {
		s.writeCatalogError(w, r, err)
		return
	}
	if err := s.store.RecordExportAudit(r.Context(), actorID, kind, format, rows); err != nil {
		s.writeCatalogError(w, r, err)
		return
	}
	file, err := os.Open(path)
	if err != nil {
		s.internalAPIError(w, r, err)
		return
	}
	defer file.Close()
	info, err := file.Stat()
	if err != nil {
		s.internalAPIError(w, r, err)
		return
	}
	w.Header().Set("Content-Type", contentType)
	w.Header().Set("Content-Disposition", fmt.Sprintf(`attachment; filename="%s"`, filename))
	w.Header().Set("Cache-Control", "no-store")
	w.Header().Set("Content-Length", fmt.Sprintf("%d", info.Size()))
	w.WriteHeader(http.StatusOK)
	_, _ = io.Copy(w, file)
}
