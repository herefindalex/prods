package webapp

import (
	"errors"
	"net/http"
	"time"

	"prods/internal/catalog"
	"prods/internal/identity"
	"prods/internal/recovery"
	"prods/internal/storage/sqlite"
)

type backupStatusResponse struct {
	Settings              sqlite.BackupSettings    `json:"settings"`
	Runs                  []sqlite.BackupRun       `json:"runs"`
	NextRunUTC            string                   `json:"next_run_utc,omitempty"`
	Due                   bool                     `json:"due"`
	Retention             recovery.RetentionResult `json:"retention"`
	ContainsSensitiveData bool                     `json:"contains_sensitive_data"`
	ExternalRequirements  []string                 `json:"external_requirements,omitempty"`
}

func (s *Server) adminBackupStatus(w http.ResponseWriter, r *http.Request) {
	if _, ok := s.requireCapability(w, r, identity.CapabilitySystemManage, false); !ok {
		return
	}
	settings, err := s.store.BackupSettings(r.Context())
	if err != nil {
		s.internalAPIError(w, r, err)
		return
	}
	runs, err := s.store.RecentBackupRuns(r.Context(), 20)
	if err != nil {
		s.internalAPIError(w, r, err)
		return
	}
	next, due, err := recovery.BackupScheduleState(time.Now(), settings, runs)
	if err != nil {
		s.internalAPIError(w, r, err)
		return
	}
	retention, err := recovery.EvaluateBackupRetention(r.Context(), s.config.BackupDir, settings, time.Now())
	if err != nil {
		s.internalAPIError(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, backupStatusResponse{
		Settings: settings, Runs: runs, NextRunUTC: next, Due: due, Retention: retention, ContainsSensitiveData: true,
		ExternalRequirements: s.config.BackupExternalRequirements,
	})
}

func (s *Server) adminUpdateBackupSettings(w http.ResponseWriter, r *http.Request) {
	current, ok := s.requireCapability(w, r, identity.CapabilitySystemManage, true)
	if !ok {
		return
	}
	var request struct {
		ExpectedVersion int64                 `json:"expected_version"`
		Settings        sqlite.BackupSettings `json:"settings"`
	}
	if err := decodeJSON(r.Body, &request); err != nil {
		s.writeAPIError(w, r, http.StatusBadRequest, apiCodeInvalidJSON)
		return
	}
	settings, err := s.store.UpdateBackupSettings(r.Context(), current.UserID, request.ExpectedVersion, request.Settings)
	if errors.Is(err, catalog.ErrRevisionConflict) {
		s.writeAPIError(w, r, http.StatusConflict, apiCodeRevisionConflict)
		return
	}
	if errors.Is(err, sqlite.ErrInvalidBackupState) {
		s.writeAPIError(w, r, http.StatusUnprocessableEntity, apiCodeValidationFailed)
		return
	}
	if err != nil {
		s.internalAPIError(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, settings)
}

func (s *Server) adminRunBackup(w http.ResponseWriter, r *http.Request) {
	if _, ok := s.requireCapability(w, r, identity.CapabilitySystemManage, true); !ok {
		return
	}
	if s.backupManager == nil {
		s.writeAPIError(w, r, http.StatusServiceUnavailable, apiCodeServiceUnavailable)
		return
	}
	run, err := s.backupManager.RunManual(r.Context())
	if err != nil {
		if errors.Is(err, recovery.ErrBackupManagerClosed) {
			s.writeAPIError(w, r, http.StatusServiceUnavailable, apiCodeServiceUnavailable)
			return
		}
		s.writeResourceError(w, r, err)
		return
	}
	writeJSON(w, http.StatusCreated, run)
}
