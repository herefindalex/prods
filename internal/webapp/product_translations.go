package webapp

import (
	"database/sql"
	"errors"
	"net/http"

	"prods/internal/catalog"
	"prods/internal/identity"
	"prods/internal/localization"
	"prods/internal/storage/sqlite"
)

func (s *Server) adminProductContent(w http.ResponseWriter, r *http.Request) {
	if _, ok := s.requireCapability(w, r, identity.CapabilityCatalogView, false); !ok {
		return
	}
	content, err := s.store.ProductContent(r.Context(), r.PathValue("id"))
	if errors.Is(err, sql.ErrNoRows) {
		s.writeAPIError(w, r, http.StatusNotFound, apiCodeNotFound)
		return
	}
	if err != nil {
		s.internalAPIError(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, content)
}

func (s *Server) adminCatalogContentSettings(w http.ResponseWriter, r *http.Request) {
	if _, ok := s.requireCapability(w, r, identity.CapabilityCatalogView, false); !ok {
		return
	}
	settings, err := s.store.WebsiteState(r.Context())
	if err != nil {
		s.internalAPIError(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"default_locale": settings.WorkingLocalization.DefaultLocale, "supported_locales": localization.BuiltinLocaleCodes(), "content_multilingual_enabled": settings.WorkingLocalization.ContentEditingEnabled})
}

func (s *Server) adminSaveProductSourceLocales(w http.ResponseWriter, r *http.Request) {
	current, ok := s.requireCapability(w, r, identity.CapabilityCatalogEdit, true)
	if !ok {
		return
	}
	var request struct {
		ExpectedRevision int64             `json:"expected_revision"`
		SourceLocale     string            `json:"source_locale"`
		SourceLocales    map[string]string `json:"source_locales"`
	}
	if err := decodeJSON(r.Body, &request); err != nil {
		s.writeAPIError(w, r, http.StatusBadRequest, apiCodeInvalidJSON)
		return
	}
	content, err := s.store.SaveProductSourceLocales(r.Context(), current.UserID, r.PathValue("id"), request.ExpectedRevision, request.SourceLocale, request.SourceLocales)
	if err != nil {
		s.writeContentLocaleError(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, content)
}

func (s *Server) writeContentLocaleError(w http.ResponseWriter, r *http.Request, err error) {
	switch {
	case errors.Is(err, sql.ErrNoRows):
		s.writeAPIError(w, r, http.StatusNotFound, apiCodeNotFound)
	case errors.Is(err, catalog.ErrRevisionConflict):
		s.writeAPIError(w, r, http.StatusConflict, apiCodeRevisionConflict)
	case errors.Is(err, catalog.ErrArchivedProduct):
		s.writeAPIError(w, r, http.StatusConflict, apiCodeValidationFailed)
	case errors.Is(err, catalog.ErrInvalidProduct), errors.Is(err, catalog.ErrInvalidDictionary), errors.Is(err, catalog.ErrInvalidContentLocale), errors.Is(err, sqlite.ErrContentLocaleUnavailable):
		s.writeAPIError(w, r, http.StatusUnprocessableEntity, apiCodeValidationFailed)
	case errors.Is(err, sqlite.ErrPermissionDenied):
		s.writeAPIError(w, r, http.StatusForbidden, apiCodeForbidden)
	default:
		s.internalAPIError(w, r, err)
	}
}

func (s *Server) adminSaveProductTranslation(w http.ResponseWriter, r *http.Request) {
	current, ok := s.requireCapability(w, r, identity.CapabilityCatalogEdit, true)
	if !ok {
		return
	}
	var request struct {
		ExpectedRevision int64                      `json:"expected_revision"`
		Translation      catalog.ProductTranslation `json:"translation"`
	}
	if err := decodeJSON(r.Body, &request); err != nil {
		s.writeAPIError(w, r, http.StatusBadRequest, apiCodeInvalidJSON)
		return
	}
	request.Translation.Locale = r.PathValue("locale")
	content, err := s.store.SaveProductTranslation(r.Context(), current.UserID, r.PathValue("id"), request.ExpectedRevision, request.Translation)
	if err != nil {
		switch {
		case errors.Is(err, sql.ErrNoRows):
			s.writeAPIError(w, r, http.StatusNotFound, apiCodeNotFound)
		case errors.Is(err, catalog.ErrRevisionConflict):
			s.writeAPIError(w, r, http.StatusConflict, apiCodeRevisionConflict)
		case errors.Is(err, catalog.ErrArchivedProduct):
			s.writeAPIError(w, r, http.StatusConflict, apiCodeConflict)
		case errors.Is(err, catalog.ErrInvalidProduct), errors.Is(err, sqlite.ErrContentLocaleUnavailable):
			s.writeAPIError(w, r, http.StatusUnprocessableEntity, apiCodeValidationFailed)
		case errors.Is(err, sqlite.ErrPermissionDenied):
			s.writeAPIError(w, r, http.StatusForbidden, apiCodeForbidden)
		default:
			s.internalAPIError(w, r, err)
		}
		return
	}
	writeJSON(w, http.StatusOK, content)
}
