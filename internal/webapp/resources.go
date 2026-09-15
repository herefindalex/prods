package webapp

import (
	"context"
	"errors"
	"net/http"
	"time"

	"prods/internal/identity"
	"prods/internal/platform"
	"prods/internal/recovery"
)

const (
	resourceByteHeadroom   = 64 << 20
	resourceInodeHeadroom  = 128
	resourceMinFreePercent = 1
)

func (s *Server) admitResource(ctx context.Context, operation, path string, requiredBytes, requiredInodes uint64) (*platform.Reservation, error) {
	reservation, _, err := s.config.ResourceGate.Admit(ctx, platform.ResourceRequest{
		Operation:      operation,
		Path:           path,
		RequiredBytes:  requiredBytes,
		ByteHeadroom:   resourceByteHeadroom,
		MinFreePercent: resourceMinFreePercent,
		RequiredInodes: requiredInodes,
		InodeHeadroom:  resourceInodeHeadroom,
	})
	return reservation, err
}

func (s *Server) writeResourceError(w http.ResponseWriter, r *http.Request, err error) {
	if errors.Is(err, platform.ErrResourceCritical) || errors.Is(err, platform.ErrResourceUnavailable) {
		s.writeAPIError(w, r, http.StatusInsufficientStorage, apiCodeResourceUnavailable)
		return
	}
	s.internalAPIError(w, r, err)
}

type systemResourceHealth struct {
	Resource           string   `json:"resource"`
	Status             string   `json:"status"`
	Reason             string   `json:"reason,omitempty"`
	CheckedUTC         string   `json:"checked_utc"`
	TotalBytes         uint64   `json:"total_bytes,omitempty"`
	FreeBytes          uint64   `json:"free_bytes,omitempty"`
	ReservedBytes      uint64   `json:"reserved_bytes,omitempty"`
	FreeInodes         uint64   `json:"free_inodes,omitempty"`
	ReservedInodes     uint64   `json:"reserved_inodes,omitempty"`
	SupportsInodes     bool     `json:"supports_inodes"`
	AffectedOperations []string `json:"affected_operations"`
}

type systemComponentHealth struct {
	Component             string           `json:"component"`
	Status                string           `json:"status"`
	Summary               string           `json:"summary"`
	CheckedUTC            string           `json:"checked_utc"`
	Configured            *bool            `json:"configured,omitempty"`
	LastSuccessUTC        string           `json:"last_success_utc,omitempty"`
	LastRunStatus         string           `json:"last_run_status,omitempty"`
	NextRunUTC            string           `json:"next_run_utc,omitempty"`
	Due                   bool             `json:"due,omitempty"`
	Counters              map[string]int64 `json:"counters,omitempty"`
	RuntimeRunning        *bool            `json:"runtime_running,omitempty"`
	LastRuntimeAttemptUTC string           `json:"last_runtime_attempt_utc,omitempty"`
	LastRuntimeSuccessUTC string           `json:"last_runtime_success_utc,omitempty"`
	LastRuntimeFailureUTC string           `json:"last_runtime_failure_utc,omitempty"`
	RuntimeFailureStage   string           `json:"runtime_failure_stage,omitempty"`
	ConsecutiveFailures   int64            `json:"consecutive_runtime_failures,omitempty"`
}

type systemHealthResponse struct {
	Status     string                  `json:"status"`
	CheckedUTC string                  `json:"checked_utc"`
	Components []systemComponentHealth `json:"components"`
	Resources  []systemResourceHealth  `json:"resources"`
}

func (s *Server) adminSystemHealth(w http.ResponseWriter, r *http.Request) {
	if _, ok := s.requireCapability(w, r, identity.CapabilitySystemManage, false); !ok {
		return
	}
	checked := time.Now().UTC().Format(time.RFC3339Nano)
	resources := []struct {
		name       string
		path       string
		operations []string
	}{
		{name: "database", path: s.config.DatabasePath, operations: []string{"login", "RFQ", "Admin writes"}},
		{name: "assets", path: s.config.AssetDir, operations: []string{"product asset upload", "website asset upload"}},
		{name: "generated", path: s.config.PublicDir, operations: []string{"product publication", "website version publish"}},
		{name: "work", path: s.config.WorkDir, operations: []string{"import", "private preview"}},
		{name: "backups", path: s.config.BackupDir, operations: []string{"backup", "migration pre-backup"}},
	}
	response := systemHealthResponse{
		Status:     "Normal",
		CheckedUTC: checked,
		Components: s.systemComponents(r.Context(), checked),
		Resources:  make([]systemResourceHealth, 0, len(resources)),
	}
	for _, component := range response.Components {
		raiseHealthStatus(&response.Status, component.Status)
	}
	for _, resource := range resources {
		item := systemResourceHealth{
			Resource: resource.name, Status: "Normal", CheckedUTC: checked, AffectedOperations: resource.operations,
		}
		snapshot, err := s.config.ResourceGate.Inspect(r.Context(), resource.path)
		if err == nil {
			item.TotalBytes = snapshot.TotalBytes
			item.FreeBytes = snapshot.FreeBytes
			item.ReservedBytes = snapshot.ReservedBytes
			item.FreeInodes = snapshot.FreeInodes
			item.ReservedInodes = snapshot.ReservedInodes
			item.SupportsInodes = snapshot.SupportsInodes
			var reservation *platform.Reservation
			reservation, _, err = s.config.ResourceGate.Admit(r.Context(), platform.ResourceRequest{
				Operation: "health check " + resource.name, Path: resource.path,
				ByteHeadroom: resourceByteHeadroom, MinFreePercent: resourceMinFreePercent,
				InodeHeadroom: resourceInodeHeadroom,
			})
			if reservation != nil {
				reservation.Release()
			}
		}
		if err != nil {
			item.Status = "Critical"
			item.Reason = "capacity is unavailable or below the admission floor"
			raiseHealthStatus(&response.Status, item.Status)
		}
		response.Resources = append(response.Resources, item)
	}
	writeJSON(w, http.StatusOK, response)
}

func (s *Server) systemComponents(ctx context.Context, checked string) []systemComponentHealth {
	components := make([]systemComponentHealth, 0, 8)
	database := systemComponentHealth{
		Component:  "database",
		Status:     "Normal",
		Summary:    "SQLite is ready for admitted application work.",
		CheckedUTC: checked,
	}
	if err := s.store.Ready(ctx); err != nil {
		database.Status = "Critical"
		database.Summary = "SQLite readiness verification failed."
	}
	components = append(components, database)
	maintenance := s.maintenanceSnapshot()
	maintenanceHealth := systemComponentHealth{
		Component:  "maintenance",
		Status:     "Normal",
		Summary:    "Manual site maintenance is inactive.",
		CheckedUTC: checked,
	}
	if maintenance.Active {
		maintenanceHealth.Status = "Warning"
		maintenanceHealth.Summary = "Manual maintenance is active; public requests and new RFQs are paused."
	}
	components = append(components, maintenanceHealth)

	operational, operationalErr := s.store.OperationalHealth(ctx)
	public := systemComponentHealth{
		Component:  "public_generation",
		Status:     "Normal",
		Summary:    "Public index and durable publication work are consistent.",
		CheckedUTC: checked,
	}
	background := systemComponentHealth{
		Component:  "background_jobs",
		Status:     "Normal",
		Summary:    "Durable background work is observable.",
		CheckedUTC: checked,
	}
	searchConfigured := true
	search := systemComponentHealth{
		Component:  "search",
		Status:     "Normal",
		Summary:    "The built-in Unicode search projection matches active publications.",
		CheckedUTC: checked,
		Configured: &searchConfigured,
	}
	if operationalErr != nil {
		public.Status = "Critical"
		public.Summary = "Durable publication state could not be inspected."
		background.Status = "Critical"
		background.Summary = "Durable background work could not be inspected."
		search.Status = "Critical"
		search.Summary = "Search projection state could not be inspected."
	} else {
		public.Counters = map[string]int64{
			"pending":    operational.PublicationPending,
			"processing": operational.PublicationProcessing,
			"failed":     operational.PublicationFailed,
			"dirty":      operational.PublicationDirty,
		}
		if operational.PublicationFailed > 0 || operational.PublicationDirty > 0 {
			public.Status = "Warning"
			public.Summary = "Publication work needs reconciliation or aggregate convergence."
		}
		background.Counters = map[string]int64{
			"active_imports":  operational.ActiveImports,
			"running_backups": operational.RunningBackups,
		}
		search.Counters = map[string]int64{
			"active_products":  operational.ActivePublicProducts,
			"projection_drift": operational.SearchProjectionDrift,
		}
		if operational.SearchProjectionDrift > 0 {
			search.Status = "Warning"
			search.Summary = "One or more active publications lack the current search projection."
		}
	}
	if s.publisher != nil {
		if _, _, err := s.publisher.PublicSite(); err != nil {
			public.Status = "Critical"
			public.Summary = "The active public index is unavailable."
		}
	} else if s.config.EnablePOCAdmin {
		public.Summary = "Explicit PoC mode uses the direct fixture renderer."
	}
	components = append(components, public, background)

	backup := systemComponentHealth{
		Component:  "backup",
		Status:     "Normal",
		Summary:    "Backup scheduling and durable receipts are available.",
		CheckedUTC: checked,
	}
	settings, settingsErr := s.store.BackupSettings(ctx)
	runs, runsErr := s.store.RecentBackupRuns(ctx, 20)
	if settingsErr != nil || runsErr != nil {
		backup.Status = "Warning"
		backup.Summary = "Backup settings or run receipts could not be inspected."
	} else {
		if len(runs) > 0 {
			backup.LastRunStatus = runs[0].Status
		}
		for _, run := range runs {
			if run.Status == "succeeded" {
				backup.LastSuccessUTC = run.CompletedAt
				if !run.ReadOnlyApplied {
					backup.Status = "Warning"
					backup.Summary = "The latest successful backup is restorable but host read-only protection was not applied."
				}
				break
			}
		}
		if backup.LastSuccessUTC == "" {
			backup.Status = "Warning"
			backup.Summary = "No successful backup has been recorded."
		}
		if len(runs) > 0 && runs[0].Status == "failed" {
			backup.Status = "Warning"
			backup.Summary = "The latest backup attempt failed."
		}
		next, due, err := recovery.BackupScheduleState(time.Now(), settings, runs)
		if err != nil {
			backup.Status = "Warning"
			backup.Summary = "The next backup schedule cannot be calculated."
		} else {
			backup.NextRunUTC = next
			backup.Due = due
			if due {
				backup.Status = "Warning"
				backup.Summary = "A scheduled backup is due and has not completed."
			}
		}
	}
	if s.backupManager == nil && !s.config.EnablePOCAdmin {
		backup.Status = "Warning"
		backup.Summary = "The automated backup manager is not running."
	} else if s.backupManager != nil {
		applyRuntimeHealth(&backup, s.backupManager.RuntimeHealth(), "The automated backup manager failed during its latest runtime cycle.")
	}
	assetGC := systemComponentHealth{
		Component:  "asset_gc",
		Status:     "Normal",
		Summary:    "Orphan grace, backup pins, and durable file deletion are available.",
		CheckedUTC: checked,
	}
	if operationalErr != nil {
		assetGC.Status = "Critical"
		assetGC.Summary = "Durable asset lifecycle state could not be inspected."
	} else {
		assetGC.Counters = map[string]int64{
			"orphans_in_grace": operational.AssetOrphans,
			"deletion_pending": operational.AssetDeletionPending,
			"deletion_failed":  operational.AssetDeletionFailed,
		}
		if operational.AssetDeletionFailed > 0 {
			assetGC.Status = "Warning"
			assetGC.Summary = "One or more private asset files could not be deleted and remain queued for retry."
		}
	}
	if s.assetGC == nil && !s.config.EnablePOCAdmin {
		assetGC.Status = "Warning"
		assetGC.Summary = "The asset garbage collector is not running."
	} else if s.assetGC != nil {
		applyRuntimeHealth(&assetGC, s.assetGC.RuntimeHealth(), "The asset garbage collector failed during its latest runtime cycle.")
	}
	components = append(components, backup, assetGC, search)

	smtpConfigured := s.mailManager != nil
	smtp := systemComponentHealth{
		Component:  "smtp",
		Status:     "Normal",
		Summary:    "Optional SMTP delivery is not configured; RFQ durability does not depend on it.",
		CheckedUTC: checked,
		Configured: &smtpConfigured,
	}
	if s.mailManager != nil {
		smtp.Summary = "The SMTP delivery worker is running; delivery outcomes remain durable."
		applyRuntimeHealth(&smtp, s.mailManager.RuntimeHealth(), "The SMTP delivery worker could not claim or persist durable delivery work.")
	}
	components = append(components, smtp)
	return components
}

func applyRuntimeHealth(component *systemComponentHealth, snapshot platform.RuntimeHealthSnapshot, failureSummary string) {
	running := snapshot.Running
	component.RuntimeRunning = &running
	component.LastRuntimeAttemptUTC = formatRuntimeHealthTime(snapshot.LastAttemptUTC)
	component.LastRuntimeSuccessUTC = formatRuntimeHealthTime(snapshot.LastSuccessUTC)
	component.LastRuntimeFailureUTC = formatRuntimeHealthTime(snapshot.LastFailureUTC)
	component.RuntimeFailureStage = snapshot.FailureStage
	component.ConsecutiveFailures = snapshot.ConsecutiveFailures
	if !snapshot.Running {
		raiseHealthStatus(&component.Status, "Warning")
		if component.Status == "Warning" {
			component.Summary = "The configured background manager is not running."
		}
	}
	if snapshot.ConsecutiveFailures > 0 {
		raiseHealthStatus(&component.Status, "Warning")
		if component.Status != "Critical" && component.Status != "Recovery Required" {
			component.Summary = failureSummary
		}
	}
}

func formatRuntimeHealthTime(value time.Time) string {
	if value.IsZero() {
		return ""
	}
	return value.UTC().Format(time.RFC3339Nano)
}

func raiseHealthStatus(current *string, candidate string) {
	severity := map[string]int{"Normal": 0, "Warning": 1, "Critical": 2, "Recovery Required": 3}
	if severity[candidate] > severity[*current] {
		*current = candidate
	}
}
