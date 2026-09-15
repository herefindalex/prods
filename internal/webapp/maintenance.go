package webapp

import (
	"database/sql"
	"errors"
	"net/http"
	"strings"

	"prods/internal/catalog"
	"prods/internal/identity"
)

func (s *Server) adminProducts(w http.ResponseWriter, r *http.Request) {
	if _, ok := s.requireCapability(w, r, identity.CapabilityCatalogView, false); !ok {
		return
	}
	products, err := s.store.ListProducts(r.Context(), r.URL.Query().Get("include_archived") == "true", 500)
	if err != nil {
		s.internalAPIError(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, products)
}

func (s *Server) adminCategories(w http.ResponseWriter, r *http.Request) {
	if _, ok := s.requireCapability(w, r, identity.CapabilityCatalogView, false); !ok {
		return
	}
	categories, err := s.store.ListCategories(r.Context())
	if err != nil {
		s.internalAPIError(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, categories)
}

func (s *Server) adminDictionaries(w http.ResponseWriter, r *http.Request) {
	if _, ok := s.requireCapability(w, r, identity.CapabilityCatalogView, false); !ok {
		return
	}
	kind := catalog.DictionaryKind(strings.TrimSpace(r.URL.Query().Get("kind")))
	entries, err := s.store.ListDictionaryEntries(r.Context(), kind)
	if err != nil {
		s.internalAPIError(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, entries)
}

func (s *Server) adminSpecs(w http.ResponseWriter, r *http.Request) {
	if _, ok := s.requireCapability(w, r, identity.CapabilityCatalogView, false); !ok {
		return
	}
	specs, err := s.store.ListSpecDefinitions(r.Context())
	if err != nil {
		s.internalAPIError(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, specs)
}

func (s *Server) adminSpecSets(w http.ResponseWriter, r *http.Request) {
	if _, ok := s.requireCapability(w, r, identity.CapabilityCatalogView, false); !ok {
		return
	}
	sets, err := s.store.ListSpecSets(r.Context())
	if err != nil {
		s.internalAPIError(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, sets)
}

func (s *Server) adminCategorySpecSet(w http.ResponseWriter, r *http.Request) {
	if _, ok := s.requireCapability(w, r, identity.CapabilityCatalogView, false); !ok {
		return
	}
	set, err := s.store.CategorySpecSet(r.Context(), r.PathValue("id"))
	if errors.Is(err, sql.ErrNoRows) {
		s.writeAPIError(w, r, http.StatusNotFound, apiCodeNotFound)
		return
	}
	if err != nil {
		s.internalAPIError(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, set)
}

func (s *Server) adminProductSpecValues(w http.ResponseWriter, r *http.Request) {
	if _, ok := s.requireCapability(w, r, identity.CapabilityCatalogView, false); !ok {
		return
	}
	values, err := s.store.ProductSpecValues(r.Context(), r.PathValue("id"))
	if errors.Is(err, sql.ErrNoRows) {
		s.writeAPIError(w, r, http.StatusNotFound, apiCodeNotFound)
		return
	}
	if err != nil {
		s.internalAPIError(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, values)
}

func (s *Server) adminCloneProduct(w http.ResponseWriter, r *http.Request) {
	current, ok := s.requireCapability(w, r, identity.CapabilityCatalogEdit, true)
	if !ok {
		return
	}
	var request struct {
		ExpectedRevision int64           `json:"expected_revision"`
		Product          catalog.Product `json:"product"`
	}
	if err := decodeJSON(r.Body, &request); err != nil {
		s.writeAPIError(w, r, http.StatusBadRequest, apiCodeInvalidJSON)
		return
	}
	product, err := s.store.CloneProduct(r.Context(), current.UserID, r.PathValue("id"), request.ExpectedRevision, request.Product)
	if err != nil {
		s.writeCatalogError(w, r, err)
		return
	}
	writeJSON(w, http.StatusCreated, product)
}

func (s *Server) adminCreateCategory(w http.ResponseWriter, r *http.Request) {
	current, ok := s.requireCapability(w, r, identity.CapabilityCatalogEdit, true)
	if !ok {
		return
	}
	var category catalog.Category
	if err := decodeJSON(r.Body, &category); err != nil {
		s.writeAPIError(w, r, http.StatusBadRequest, apiCodeInvalidJSON)
		return
	}
	category, err := s.store.CreateCategory(r.Context(), current.UserID, category)
	if err != nil {
		s.writeCatalogError(w, r, err)
		return
	}
	writeJSON(w, http.StatusCreated, category)
}

func (s *Server) adminMoveCategory(w http.ResponseWriter, r *http.Request) {
	current, ok := s.requireCapability(w, r, identity.CapabilityCatalogEdit, true)
	if !ok {
		return
	}
	var request struct {
		ExpectedRevision int64  `json:"expected_revision"`
		ParentID         string `json:"parent_id"`
	}
	if err := decodeJSON(r.Body, &request); err != nil {
		s.writeAPIError(w, r, http.StatusBadRequest, apiCodeInvalidJSON)
		return
	}
	if err := s.store.MoveCategory(r.Context(), current.UserID, r.PathValue("id"), request.ParentID, request.ExpectedRevision); err != nil {
		s.writeCatalogError(w, r, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

type categoryUpdateRequest struct {
	ExpectedRevision int64  `json:"expected_revision"`
	ParentID         string `json:"parent_id"`
	Name             string `json:"name"`
	Slug             string `json:"slug"`
}

func (s *Server) adminPreviewCategoryUpdate(w http.ResponseWriter, r *http.Request) {
	current, ok := s.requireCapability(w, r, identity.CapabilityCatalogEdit, true)
	if !ok {
		return
	}
	var request categoryUpdateRequest
	if err := decodeJSON(r.Body, &request); err != nil {
		s.writeAPIError(w, r, http.StatusBadRequest, apiCodeInvalidJSON)
		return
	}
	impact, err := s.store.PreviewCategoryUpdate(r.Context(), current.UserID, r.PathValue("id"), request.ExpectedRevision, catalog.Category{
		ParentID: request.ParentID, Name: request.Name, Slug: request.Slug,
	})
	if err != nil {
		s.writeCatalogError(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, impact)
}

func (s *Server) adminUpdateCategory(w http.ResponseWriter, r *http.Request) {
	current, ok := s.requireCapability(w, r, identity.CapabilityCatalogEdit, true)
	if !ok {
		return
	}
	var request categoryUpdateRequest
	if err := decodeJSON(r.Body, &request); err != nil {
		s.writeAPIError(w, r, http.StatusBadRequest, apiCodeInvalidJSON)
		return
	}
	category, impact, err := s.store.UpdateCategory(r.Context(), current.UserID, r.PathValue("id"), request.ExpectedRevision, catalog.Category{
		ParentID: request.ParentID, Name: request.Name, Slug: request.Slug,
	})
	if err != nil {
		s.writeCatalogError(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, struct {
		Category catalog.Category       `json:"category"`
		Impact   catalog.TaxonomyImpact `json:"impact"`
	}{Category: category, Impact: impact})
}

func (s *Server) adminDisableCategory(w http.ResponseWriter, r *http.Request) {
	current, ok := s.requireCapability(w, r, identity.CapabilityCatalogEdit, true)
	if !ok {
		return
	}
	var request struct {
		ExpectedRevision int64 `json:"expected_revision"`
	}
	if err := decodeJSON(r.Body, &request); err != nil {
		s.writeAPIError(w, r, http.StatusBadRequest, apiCodeInvalidJSON)
		return
	}
	if err := s.store.DisableCategory(r.Context(), current.UserID, r.PathValue("id"), request.ExpectedRevision); err != nil {
		s.writeCatalogError(w, r, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (s *Server) adminCreateDictionary(w http.ResponseWriter, r *http.Request) {
	current, ok := s.requireCapability(w, r, identity.CapabilityCatalogEdit, true)
	if !ok {
		return
	}
	var entry catalog.DictionaryEntry
	if err := decodeJSON(r.Body, &entry); err != nil {
		s.writeAPIError(w, r, http.StatusBadRequest, apiCodeInvalidJSON)
		return
	}
	entry, err := s.store.CreateDictionaryEntry(r.Context(), current.UserID, entry)
	if err != nil {
		s.writeCatalogError(w, r, err)
		return
	}
	writeJSON(w, http.StatusCreated, entry)
}

func (s *Server) adminDisableDictionary(w http.ResponseWriter, r *http.Request) {
	current, ok := s.requireCapability(w, r, identity.CapabilityCatalogEdit, true)
	if !ok {
		return
	}
	var request struct {
		ExpectedRevision int64 `json:"expected_revision"`
	}
	if err := decodeJSON(r.Body, &request); err != nil {
		s.writeAPIError(w, r, http.StatusBadRequest, apiCodeInvalidJSON)
		return
	}
	if err := s.store.DisableDictionaryEntry(r.Context(), current.UserID, r.PathValue("id"), request.ExpectedRevision); err != nil {
		s.writeCatalogError(w, r, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

type dictionaryUpdateRequest struct {
	ExpectedRevision int64  `json:"expected_revision"`
	Name             string `json:"name"`
	Slug             string `json:"slug"`
}

func (s *Server) adminPreviewDictionaryUpdate(w http.ResponseWriter, r *http.Request) {
	current, ok := s.requireCapability(w, r, identity.CapabilityCatalogEdit, true)
	if !ok {
		return
	}
	var request dictionaryUpdateRequest
	if err := decodeJSON(r.Body, &request); err != nil {
		s.writeAPIError(w, r, http.StatusBadRequest, apiCodeInvalidJSON)
		return
	}
	impact, err := s.store.PreviewDictionaryUpdate(r.Context(), current.UserID, r.PathValue("id"), request.ExpectedRevision, catalog.DictionaryEntry{Name: request.Name, Slug: request.Slug})
	if err != nil {
		s.writeCatalogError(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, impact)
}

func (s *Server) adminUpdateDictionary(w http.ResponseWriter, r *http.Request) {
	current, ok := s.requireCapability(w, r, identity.CapabilityCatalogEdit, true)
	if !ok {
		return
	}
	var request dictionaryUpdateRequest
	if err := decodeJSON(r.Body, &request); err != nil {
		s.writeAPIError(w, r, http.StatusBadRequest, apiCodeInvalidJSON)
		return
	}
	entry, impact, err := s.store.UpdateDictionaryEntry(r.Context(), current.UserID, r.PathValue("id"), request.ExpectedRevision, catalog.DictionaryEntry{Name: request.Name, Slug: request.Slug})
	if err != nil {
		s.writeCatalogError(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, struct {
		Entry  catalog.DictionaryEntry `json:"entry"`
		Impact catalog.TaxonomyImpact  `json:"impact"`
	}{Entry: entry, Impact: impact})
}

func (s *Server) adminCreateSpec(w http.ResponseWriter, r *http.Request) {
	current, ok := s.requireCapability(w, r, identity.CapabilityCatalogEdit, true)
	if !ok {
		return
	}
	var spec catalog.SpecDefinition
	if err := decodeJSON(r.Body, &spec); err != nil {
		s.writeAPIError(w, r, http.StatusBadRequest, apiCodeInvalidJSON)
		return
	}
	spec, err := s.store.CreateSpecDefinition(r.Context(), current.UserID, spec)
	if err != nil {
		s.writeCatalogError(w, r, err)
		return
	}
	writeJSON(w, http.StatusCreated, spec)
}

func (s *Server) adminCreateSpecSet(w http.ResponseWriter, r *http.Request) {
	current, ok := s.requireCapability(w, r, identity.CapabilityCatalogEdit, true)
	if !ok {
		return
	}
	var set catalog.SpecSet
	if err := decodeJSON(r.Body, &set); err != nil {
		s.writeAPIError(w, r, http.StatusBadRequest, apiCodeInvalidJSON)
		return
	}
	set, err := s.store.CreateSpecSet(r.Context(), current.UserID, set)
	if err != nil {
		s.writeCatalogError(w, r, err)
		return
	}
	writeJSON(w, http.StatusCreated, set)
}

func (s *Server) adminSetCategorySpecSet(w http.ResponseWriter, r *http.Request) {
	current, ok := s.requireCapability(w, r, identity.CapabilityCatalogEdit, true)
	if !ok {
		return
	}
	var request struct {
		ExpectedRevision int64  `json:"expected_revision"`
		SpecSetID        string `json:"spec_set_id"`
	}
	if err := decodeJSON(r.Body, &request); err != nil {
		s.writeAPIError(w, r, http.StatusBadRequest, apiCodeInvalidJSON)
		return
	}
	if err := s.store.SetCategorySpecSet(r.Context(), current.UserID, r.PathValue("id"), request.SpecSetID, request.ExpectedRevision); err != nil {
		s.writeCatalogError(w, r, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (s *Server) adminSaveSpecValue(w http.ResponseWriter, r *http.Request) {
	current, ok := s.requireCapability(w, r, identity.CapabilityCatalogEdit, true)
	if !ok {
		return
	}
	var request struct {
		ExpectedRevision int64  `json:"expected_revision"`
		SpecID           string `json:"spec_id"`
		RawValue         string `json:"raw_value"`
		SourceLocale     string `json:"source_locale"`
	}
	if err := decodeJSON(r.Body, &request); err != nil {
		s.writeAPIError(w, r, http.StatusBadRequest, apiCodeInvalidJSON)
		return
	}
	value, err := s.store.SaveSpecValue(r.Context(), current.UserID, r.PathValue("id"), request.SpecID,
		request.RawValue, request.SourceLocale, request.ExpectedRevision)
	if err != nil {
		s.writeCatalogError(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, value)
}

func (s *Server) adminSaveNormalizedValue(w http.ResponseWriter, r *http.Request) {
	current, ok := s.requireCapability(w, r, identity.CapabilityCatalogEdit, true)
	if !ok {
		return
	}
	var normalized catalog.NormalizedValue
	if err := decodeJSON(r.Body, &normalized); err != nil {
		s.writeAPIError(w, r, http.StatusBadRequest, apiCodeInvalidJSON)
		return
	}
	if normalized.SpecValueID != "" && normalized.SpecValueID != r.PathValue("id") {
		s.writeAPIError(w, r, http.StatusUnprocessableEntity, apiCodeValidationFailed)
		return
	}
	normalized.SpecValueID = r.PathValue("id")
	normalized, err := s.store.SaveNormalizedValue(r.Context(), current.UserID, normalized)
	if err != nil {
		s.writeCatalogError(w, r, err)
		return
	}
	writeJSON(w, http.StatusCreated, normalized)
}

func (s *Server) adminProductDocuments(w http.ResponseWriter, r *http.Request) {
	if _, ok := s.requireCapability(w, r, identity.CapabilityCatalogView, false); !ok {
		return
	}
	documents, err := s.store.ProductDocuments(r.Context(), r.PathValue("id"))
	if err != nil {
		s.writeCatalogError(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, documents)
}

func (s *Server) adminAddProductDocument(w http.ResponseWriter, r *http.Request) {
	current, ok := s.requireCapability(w, r, identity.CapabilityCatalogEdit, true)
	if !ok {
		return
	}
	var request struct {
		ExpectedRevision int64                   `json:"expected_revision"`
		Document         catalog.ProductDocument `json:"document"`
	}
	if err := decodeJSON(r.Body, &request); err != nil {
		s.writeAPIError(w, r, http.StatusBadRequest, apiCodeInvalidJSON)
		return
	}
	if request.Document.ProductID != "" && request.Document.ProductID != r.PathValue("id") {
		s.writeAPIError(w, r, http.StatusUnprocessableEntity, apiCodeValidationFailed)
		return
	}
	request.Document.ProductID = r.PathValue("id")
	document, err := s.store.AddProductDocument(r.Context(), current.UserID, request.ExpectedRevision, request.Document)
	if err != nil {
		s.writeCatalogError(w, r, err)
		return
	}
	writeJSON(w, http.StatusCreated, document)
}
