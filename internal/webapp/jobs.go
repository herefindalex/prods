package webapp

import (
	"errors"
	"net/http"
	"strconv"

	"prods/internal/identity"
	"prods/internal/storage/sqlite"
)

type adminJobsResponse struct {
	Jobs        []sqlite.JobRecord `json:"jobs"`
	RetryPolicy jobRetryPolicy     `json:"retry_policy"`
}

type jobRetryPolicy struct {
	SafeKinds      []string `json:"safe_kinds"`
	NewOperation   []string `json:"new_operation_only"`
	NeverAutomatic []string `json:"never_automatic"`
}

func (s *Server) adminJobs(w http.ResponseWriter, r *http.Request) {
	current, ok := s.requireCapability(w, r, identity.CapabilityAdminAccess, false)
	if !ok {
		return
	}
	limit := 50
	if raw := r.URL.Query().Get("limit"); raw != "" {
		parsed, err := strconv.Atoi(raw)
		if err != nil || parsed < 1 || parsed > 100 {
			s.writeAPIError(w, r, http.StatusBadRequest, apiCodeValidationFailed)
			return
		}
		limit = parsed
	}
	jobs, err := s.store.RecentJobs(r.Context(), sqlite.JobKinds{
		Publication: current.can(identity.CapabilityCatalogPublish),
		Import:      current.can(identity.CapabilityCatalogImport),
		Backup:      current.can(identity.CapabilitySystemManage),
		SMTP:        current.can(identity.CapabilityRFQManage),
		Search:      current.can(identity.CapabilitySystemManage),
	}, limit)
	if err != nil {
		s.internalAPIError(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, adminJobsResponse{
		Jobs: jobs,
		RetryPolicy: jobRetryPolicy{
			SafeKinds:      []string{"publication", "search_submission"},
			NewOperation:   []string{"backup"},
			NeverAutomatic: []string{"import_commit", "smtp_unknown", "restore", "migration"},
		},
	})
}

func (s *Server) adminRetryPublicationJob(w http.ResponseWriter, r *http.Request) {
	current, ok := s.requireCapability(w, r, identity.CapabilityCatalogPublish, true)
	if !ok {
		return
	}
	if s.publisher == nil {
		s.writeAPIError(w, r, http.StatusServiceUnavailable, apiCodeServiceUnavailable)
		return
	}
	job, err := s.store.RetryPublicationJob(r.Context(), current.UserID, r.PathValue("id"))
	if err != nil {
		switch {
		case errors.Is(err, sqlite.ErrJobNotRetryable):
			s.writeAPIError(w, r, http.StatusConflict, apiCodeJobNotRetryable)
		case errors.Is(err, sqlite.ErrPermissionDenied):
			s.writeAPIError(w, r, http.StatusForbidden, apiCodeForbidden)
		default:
			s.internalAPIError(w, r, err)
		}
		return
	}
	s.publisher.Wake()
	writeJSON(w, http.StatusAccepted, job)
}
