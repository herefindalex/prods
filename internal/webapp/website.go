package webapp

import (
	"database/sql"
	"errors"
	"net/http"
	"strconv"
	"strings"

	"prods/internal/catalog"
	"prods/internal/identity"
	"prods/internal/publishing"
	"prods/internal/site"
	"prods/internal/storage/sqlite"
)

type siteRouteState struct {
	CurrentEpoch int64                      `json:"current_epoch"`
	Config       publishing.SiteRouteConfig `json:"config"`
}

func (s *Server) adminSiteRoutes(w http.ResponseWriter, r *http.Request) {
	if _, ok := s.requireCapability(w, r, identity.CapabilitySystemManage, false); !ok {
		return
	}
	epoch, config, err := s.store.PublicSiteRouteConfig(r.Context())
	if err != nil {
		s.internalError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, siteRouteState{CurrentEpoch: epoch, Config: config})
}

func (s *Server) adminPreviewSiteRoutes(w http.ResponseWriter, r *http.Request) {
	if _, ok := s.requireCapability(w, r, identity.CapabilitySystemManage, true); !ok {
		return
	}
	if s.publisher == nil {
		http.Error(w, "public architecture unavailable", http.StatusServiceUnavailable)
		return
	}
	var config publishing.SiteRouteConfig
	if err := decodeJSON(r.Body, &config); err != nil {
		http.Error(w, "invalid JSON", http.StatusBadRequest)
		return
	}
	preview, err := s.publisher.PreviewSiteRoutes(r.Context(), config)
	if err != nil {
		if errors.Is(err, publishing.ErrSiteRouteInvalid) {
			writeJSON(w, http.StatusUnprocessableEntity, map[string]any{"error": err.Error(), "preview": preview})
			return
		}
		s.internalError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, preview)
}

func (s *Server) adminPublishSiteRoutes(w http.ResponseWriter, r *http.Request) {
	current, ok := s.requireCapability(w, r, identity.CapabilitySystemManage, true)
	if !ok {
		return
	}
	if !current.can(identity.CapabilityCatalogPublish) {
		http.Error(w, "forbidden", http.StatusForbidden)
		return
	}
	if s.publisher == nil {
		http.Error(w, "public architecture unavailable", http.StatusServiceUnavailable)
		return
	}
	var request struct {
		ExpectedEpoch           int64                      `json:"expected_epoch"`
		ExpectedWorkingRevision int64                      `json:"expected_working_revision"`
		Config                  publishing.SiteRouteConfig `json:"config"`
	}
	if err := decodeJSON(r.Body, &request); err != nil {
		http.Error(w, "invalid JSON", http.StatusBadRequest)
		return
	}
	preview, err := s.publisher.PublishSiteRoutes(r.Context(), publishing.SiteRouteRequest{
		ActorID: current.UserID, ExpectedEpoch: request.ExpectedEpoch,
		ExpectedWorkingRevision: request.ExpectedWorkingRevision, Config: request.Config,
	})
	if err != nil {
		if errors.Is(err, publishing.ErrSiteRouteInvalid) {
			writeJSON(w, http.StatusConflict, map[string]any{"error": err.Error(), "preview": preview})
			return
		}
		s.internalError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, preview)
}

func (s *Server) adminWebsiteConfiguration(w http.ResponseWriter, r *http.Request) {
	if _, ok := s.requireCapability(w, r, identity.CapabilitySystemManage, false); !ok {
		return
	}
	state, err := s.store.WebsiteState(r.Context())
	if err != nil {
		s.internalError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, state)
}

func (s *Server) adminSaveWebsiteConfiguration(w http.ResponseWriter, r *http.Request) {
	current, ok := s.requireCapability(w, r, identity.CapabilitySystemManage, true)
	if !ok {
		return
	}
	var request struct {
		ExpectedRevision int64              `json:"expected_revision"`
		Configuration    site.Configuration `json:"configuration"`
	}
	if err := decodeJSON(r.Body, &request); err != nil {
		http.Error(w, "invalid JSON", http.StatusBadRequest)
		return
	}
	if err := request.Configuration.Prepare(); err != nil {
		writeJSON(w, http.StatusUnprocessableEntity, map[string]string{"error": err.Error()})
		return
	}
	state, err := s.store.SaveWebsiteWorking(r.Context(), current.UserID, request.ExpectedRevision, request.Configuration)
	if err != nil {
		if errors.Is(err, catalog.ErrRevisionConflict) {
			writeJSON(w, http.StatusConflict, map[string]string{"error": err.Error()})
			return
		}
		if errors.Is(err, catalog.ErrInvalidAsset) {
			writeJSON(w, http.StatusUnprocessableEntity, map[string]string{"error": err.Error()})
			return
		}
		s.internalError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, state)
}

func (s *Server) adminWebsiteVersions(w http.ResponseWriter, r *http.Request) {
	if _, ok := s.requireCapability(w, r, identity.CapabilitySystemManage, false); !ok {
		return
	}
	versions, err := s.store.WebsiteVersions(r.Context(), 50)
	if err != nil {
		s.internalError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, versions)
}

func (s *Server) adminRestoreWebsiteVersion(w http.ResponseWriter, r *http.Request) {
	current, ok := s.requireCapability(w, r, identity.CapabilitySystemManage, true)
	if !ok {
		return
	}
	var request struct {
		ExpectedWorkingRevision int64 `json:"expected_working_revision"`
		Version                 int64 `json:"version"`
	}
	if err := decodeJSON(r.Body, &request); err != nil {
		http.Error(w, "invalid JSON", http.StatusBadRequest)
		return
	}
	state, err := s.store.RestoreWebsiteVersion(r.Context(), current.UserID, request.ExpectedWorkingRevision, request.Version)
	if err != nil {
		switch {
		case errors.Is(err, catalog.ErrRevisionConflict):
			writeJSON(w, http.StatusConflict, map[string]string{"error": err.Error()})
		case errors.Is(err, sql.ErrNoRows):
			http.NotFound(w, r)
		default:
			s.internalError(w, err)
		}
		return
	}
	writeJSON(w, http.StatusOK, state)
}

func (s *Server) adminCreateWebsitePreview(w http.ResponseWriter, r *http.Request) {
	if _, ok := s.requireCapability(w, r, identity.CapabilitySystemManage, true); !ok {
		return
	}
	reservation, err := s.admitResource(r.Context(), "website preview", s.config.WorkDir, maxProductPreviewBytes, 2)
	if err != nil {
		s.writeResourceError(w, err)
		return
	}
	defer reservation.Release()
	var request struct {
		ExpectedWorkingRevision int64 `json:"expected_working_revision"`
	}
	if err := decodeJSON(r.Body, &request); err != nil {
		http.Error(w, "invalid JSON", http.StatusBadRequest)
		return
	}
	state, err := s.store.WebsiteState(r.Context())
	if err != nil {
		s.internalError(w, err)
		return
	}
	if state.WorkingRevision != request.ExpectedWorkingRevision {
		writeJSON(w, http.StatusConflict, map[string]string{"error": catalog.ErrRevisionConflict.Error()})
		return
	}
	locale, err := s.store.DefaultLocale(r.Context())
	if err != nil {
		s.internalError(w, err)
		return
	}
	body, err := publishing.HTML(publishing.PublicView{
		ID: "website-preview", Revision: state.WorkingRevision, SiteEpoch: state.ActiveEpoch + 1,
		PartNumber: "Website preview", Name: "Website preview", Description: "Preview of the unpublished website configuration.",
		Language: locale, Site: state.Working,
	})
	if err != nil {
		s.internalError(w, err)
		return
	}
	if customCSS := state.Working.CustomStylesheet(); customCSS != "" {
		body = []byte(strings.ReplaceAll(string(body),
			`<link rel="stylesheet" href="/site.css?v=`+strconv.FormatInt(state.ActiveEpoch+1, 10)+`">`,
			`<style>`+customCSS+`</style>`))
	}
	for _, assetID := range state.Working.AssetIDs() {
		body = []byte(strings.ReplaceAll(string(body), `"/assets/`+assetID+`"`, `"/admin/previews/website-assets/`+assetID+`"`))
	}
	token, expiresAt, err := s.savePrivatePreviewHTML(body)
	if err != nil {
		s.internalError(w, err)
		return
	}
	writeJSON(w, http.StatusCreated, map[string]any{"url": "/admin/previews/website/" + token, "expires_at": expiresAt})
}

func (s *Server) adminWebsitePreview(w http.ResponseWriter, r *http.Request) {
	if _, ok := s.requireCapability(w, r, identity.CapabilitySystemManage, false); !ok {
		return
	}
	s.servePrivatePreviewHTML(w, r, r.PathValue("token"))
}

func (s *Server) publicSiteCustomCSS(w http.ResponseWriter, r *http.Request) {
	if s.publisher == nil || !s.publisher.ServeSiteCustomCSS(w, r) {
		http.NotFound(w, r)
	}
}

func (s *Server) adminSetWebsiteCustomCSSState(w http.ResponseWriter, r *http.Request) {
	current, ok := s.requireCapability(w, r, identity.CapabilitySystemManage, true)
	if !ok {
		return
	}
	if s.publisher == nil {
		http.Error(w, "public architecture unavailable", http.StatusServiceUnavailable)
		return
	}
	var request struct {
		Disabled bool `json:"disabled"`
	}
	if err := decodeJSON(r.Body, &request); err != nil {
		http.Error(w, "invalid JSON", http.StatusBadRequest)
		return
	}
	if err := s.store.SetWebsiteCustomCSSDisabled(r.Context(), current.UserID, request.Disabled, s.publisher); err != nil {
		if errors.Is(err, sqlite.ErrPermissionDenied) {
			http.Error(w, "forbidden", http.StatusForbidden)
			return
		}
		s.internalError(w, err)
		return
	}
	state, err := s.store.WebsiteState(r.Context())
	if err != nil {
		s.internalError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, state)
}
