package webapp

import (
	"errors"
	"net/http"

	"prods/internal/catalog"
	"prods/internal/identity"
	"prods/internal/localization"
	"prods/internal/site"
	"prods/internal/storage/sqlite"
)

func (s *Server) adminPublicCopy(w http.ResponseWriter, r *http.Request) {
	if _, ok := s.requireCapability(w, r, identity.CapabilitySystemManage, false); !ok {
		return
	}
	state, err := s.store.WebsiteState(r.Context())
	if err != nil {
		s.internalAPIError(w, r, err)
		return
	}
	catalogValue, err := s.store.PublicCopyCatalog(r.Context())
	if err != nil {
		s.internalAPIError(w, r, err)
		return
	}
	for index := range catalogValue.Definitions {
		catalogValue.Definitions[index].WhereUsed = localization.PublicCopyWhereUsed(catalogValue.Definitions[index].Key)
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"working_revision": state.WorkingRevision,
		"default_locale":   state.WorkingLocalization.DefaultLocale,
		"enabled_locales":  state.WorkingLocalization.EnabledLocales,
		"overrides":        state.WorkingLocalization.PublicCopyOverrides,
		"catalog":          catalogValue,
	})
}

func (s *Server) adminSavePublicCopy(w http.ResponseWriter, r *http.Request) {
	current, ok := s.requireCapability(w, r, identity.CapabilitySystemManage, true)
	if !ok {
		return
	}
	var request struct {
		ExpectedWorkingRevision int64  `json:"expected_working_revision"`
		Value                   string `json:"value"`
		DefinitionVersion       int64  `json:"definition_version"`
	}
	if err := decodeJSON(r.Body, &request); err != nil {
		s.writeAPIError(w, r, http.StatusBadRequest, apiCodeInvalidJSON)
		return
	}
	state, err := s.store.WebsiteState(r.Context())
	if err != nil {
		s.internalAPIError(w, r, err)
		return
	}
	settings := cloneWebsiteLocalization(state.WorkingLocalization)
	key, locale := r.PathValue("key"), r.PathValue("locale")
	if settings.PublicCopyOverrides[key] == nil {
		settings.PublicCopyOverrides[key] = make(map[string]localization.PublicCopyOverride)
	}
	settings.PublicCopyOverrides[key][locale] = localization.PublicCopyOverride{Key: key, Locale: locale, Value: request.Value, DefinitionVersion: request.DefinitionVersion}
	updated, err := s.store.SaveWebsiteLocalization(r.Context(), current.UserID, request.ExpectedWorkingRevision, settings)
	if err != nil {
		s.writePublicCopyError(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, updated)
}

func (s *Server) adminResetPublicCopy(w http.ResponseWriter, r *http.Request) {
	current, ok := s.requireCapability(w, r, identity.CapabilitySystemManage, true)
	if !ok {
		return
	}
	var request struct {
		ExpectedWorkingRevision int64 `json:"expected_working_revision"`
	}
	if err := decodeJSON(r.Body, &request); err != nil {
		s.writeAPIError(w, r, http.StatusBadRequest, apiCodeInvalidJSON)
		return
	}
	state, err := s.store.WebsiteState(r.Context())
	if err != nil {
		s.internalAPIError(w, r, err)
		return
	}
	settings := cloneWebsiteLocalization(state.WorkingLocalization)
	key, locale := r.PathValue("key"), r.PathValue("locale")
	if byLocale := settings.PublicCopyOverrides[key]; byLocale != nil {
		delete(byLocale, locale)
		if len(byLocale) == 0 {
			delete(settings.PublicCopyOverrides, key)
		}
	}
	updated, err := s.store.SaveWebsiteLocalization(r.Context(), current.UserID, request.ExpectedWorkingRevision, settings)
	if err != nil {
		s.writePublicCopyError(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, updated)
}

func cloneWebsiteLocalization(value site.WebsiteLocalization) site.WebsiteLocalization {
	clone := value
	clone.EnabledLocales = append([]string(nil), value.EnabledLocales...)
	clone.PublicCopyOverrides = make(localization.PublicCopyOverrideMap, len(value.PublicCopyOverrides))
	for key, byLocale := range value.PublicCopyOverrides {
		clone.PublicCopyOverrides[key] = make(map[string]localization.PublicCopyOverride, len(byLocale))
		for locale, override := range byLocale {
			clone.PublicCopyOverrides[key][locale] = override
		}
	}
	return clone
}

func (s *Server) writePublicCopyError(w http.ResponseWriter, r *http.Request, err error) {
	switch {
	case errors.Is(err, catalog.ErrRevisionConflict):
		s.writeAPIError(w, r, http.StatusConflict, apiCodeRevisionConflict)
	case errors.Is(err, site.ErrInvalidSettings):
		s.writeAPIError(w, r, http.StatusUnprocessableEntity, apiCodeValidationFailed)
	case errors.Is(err, sqlite.ErrPermissionDenied):
		s.writeAPIError(w, r, http.StatusForbidden, apiCodeForbidden)
	default:
		s.internalAPIError(w, r, err)
	}
}
