package main

import (
	"context"
	"crypto/subtle"
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"net"
	"net/http"
	"net/url"
	"path/filepath"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"prods/internal/hostconfig"
	"prods/internal/identity"
	"prods/internal/platform"
	"prods/internal/recovery"
	"prods/internal/webapp"
)

const recoverySessionCookie = "prods_recovery"

type recoveryUIConfig struct {
	Context         context.Context
	Diagnostic      string
	BackupDir       string
	ControlDir      string
	Restore         func(context.Context, string) error
	Now             func() time.Time
	SessionTTL      time.Duration
	Completed       chan struct{}
	AllowedBackupID string
	RestoreDisabled bool
}

type recoveryUI struct {
	config        recoveryUIConfig
	tokenDigest   string
	expiresAt     time.Time
	mu            sync.Mutex
	claimed       bool
	sessionDigest string
	csrf          string
	restoring     atomic.Bool
	finished      atomic.Bool
	completeOnce  sync.Once
	systemStatic  fs.FS
}

type recoveryState struct {
	Diagnostic      string              `json:"diagnostic"`
	CSRFToken       string              `json:"csrf_token"`
	Backups         []recovery.Manifest `json:"backups"`
	Success         bool                `json:"success"`
	Restoring       bool                `json:"restoring"`
	RestoreDisabled bool                `json:"restore_disabled"`
}

const recoveryShell = `<!doctype html><html lang="en"><head><meta charset="utf-8"><meta name="viewport" content="width=device-width,initial-scale=1"><meta name="referrer" content="no-referrer"><title>Prods Recovery</title><link rel="stylesheet" href="/static/system/system.css"></head><body><div id="system-root" data-mode="recovery"></div><noscript><main><h1>Recovery Required</h1><p>This recovery workflow requires JavaScript. Enable JavaScript to inspect verified restore points and operate recovery controls.</p></main></noscript><script type="module" src="/static/system/system.js"></script></body></html>`

func newRecoveryUI(config recoveryUIConfig) (*recoveryUI, string, error) {
	if config.Context == nil {
		config.Context = context.Background()
	}
	if config.Now == nil {
		config.Now = time.Now
	}
	if config.SessionTTL <= 0 {
		config.SessionTTL = 30 * time.Minute
	}
	if config.Completed == nil {
		config.Completed = make(chan struct{})
	}
	if strings.TrimSpace(config.BackupDir) == "" || strings.TrimSpace(config.ControlDir) == "" ||
		(config.Restore == nil && !config.RestoreDisabled) {
		return nil, "", errors.New("invalid Recovery UI configuration")
	}
	systemStatic, err := webapp.SystemStaticFS()
	if err != nil {
		return nil, "", fmt.Errorf("load embedded system UI: %w", err)
	}
	token, err := identity.NewToken(32)
	if err != nil {
		return nil, "", err
	}
	return &recoveryUI{
		config: config, tokenDigest: identity.TokenDigest(token),
		expiresAt:    config.Now().UTC().Add(config.SessionTTL),
		systemStatic: systemStatic,
	}, token, nil
}

func (ui *recoveryUI) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Cache-Control", "no-store")
	w.Header().Set("Referrer-Policy", "no-referrer")
	w.Header().Set("X-Content-Type-Options", "nosniff")
	w.Header().Set("Content-Security-Policy", "default-src 'none'; script-src 'self'; style-src 'self' 'unsafe-inline'; img-src 'self' data:; connect-src 'self'; form-action 'self'; frame-ancestors 'none'; base-uri 'none'")
	switch {
	case r.Method == http.MethodGet && r.URL.Path == "/health/live":
		w.Header().Set("Content-Type", "text/plain; charset=utf-8")
		_, _ = w.Write([]byte("live\n"))
	case r.Method == http.MethodGet && r.URL.Path == "/health/ready":
		w.Header().Set("Content-Type", "text/plain; charset=utf-8")
		w.WriteHeader(http.StatusServiceUnavailable)
		_, _ = w.Write([]byte("not ready\n"))
	case r.Method == http.MethodGet && r.URL.Path == "/recovery":
		ui.getRecovery(w, r)
	case r.Method == http.MethodGet && r.URL.Path == "/recovery/api/state":
		ui.getRecoveryState(w, r)
	case r.Method == http.MethodPost && r.URL.Path == "/recovery/api/restore":
		ui.postRestoreJSON(w, r)
	case r.Method == http.MethodPost && r.URL.Path == "/recovery/restore":
		ui.postRestore(w, r)
	case r.Method == http.MethodGet && strings.HasPrefix(r.URL.Path, "/static/system/"):
		http.StripPrefix("/static/system/", http.FileServer(http.FS(ui.systemStatic))).ServeHTTP(w, r)
	default:
		http.NotFound(w, r)
	}
}

func (ui *recoveryUI) getRecovery(w http.ResponseWriter, r *http.Request) {
	if token := r.URL.Query().Get("token"); token != "" {
		session, ok := ui.exchangeToken(token)
		if !ok {
			ui.writeDenied(w)
			return
		}
		http.SetCookie(w, &http.Cookie{
			Name: recoverySessionCookie, Value: session, Path: "/recovery", HttpOnly: true,
			SameSite: http.SameSiteStrictMode, MaxAge: int(ui.config.SessionTTL.Seconds()),
		})
		http.Redirect(w, r, "/recovery", http.StatusSeeOther)
		return
	}
	if !ui.authorized(r) {
		ui.writeDenied(w)
		return
	}
	ui.renderPage(w)
}

func (ui *recoveryUI) getRecoveryState(w http.ResponseWriter, r *http.Request) {
	if !ui.authorized(r) {
		ui.writeDenied(w)
		return
	}
	state, err := ui.currentState()
	if err != nil {
		writeRecoveryJSON(w, http.StatusInternalServerError, map[string]string{"error": "Configured backup directory could not be read."})
		return
	}
	writeRecoveryJSON(w, http.StatusOK, state)
}

func (ui *recoveryUI) postRestore(w http.ResponseWriter, r *http.Request) {
	if !ui.authorized(r) {
		ui.writeDenied(w)
		return
	}
	r.Body = http.MaxBytesReader(w, r.Body, 8<<10)
	if err := r.ParseForm(); err != nil {
		http.Error(w, "invalid recovery form", http.StatusBadRequest)
		return
	}
	if subtle.ConstantTimeCompare([]byte(r.FormValue("csrf")), []byte(ui.csrf)) != 1 {
		http.Error(w, "invalid recovery request", http.StatusForbidden)
		return
	}
	status, message := ui.performRestore(strings.TrimSpace(r.FormValue("backup_id")), r.FormValue("confirmation"))
	if status != http.StatusOK {
		http.Error(w, message, status)
		return
	}
	ui.renderPage(w)
}

func (ui *recoveryUI) postRestoreJSON(w http.ResponseWriter, r *http.Request) {
	if !ui.authorized(r) {
		ui.writeDenied(w)
		return
	}
	if subtle.ConstantTimeCompare([]byte(r.Header.Get("X-CSRF-Token")), []byte(ui.csrf)) != 1 {
		writeRecoveryJSON(w, http.StatusForbidden, map[string]string{"error": "invalid recovery request"})
		return
	}
	r.Body = http.MaxBytesReader(w, r.Body, 8<<10)
	var request struct {
		BackupID     string `json:"backup_id"`
		Confirmation string `json:"confirmation"`
	}
	decoder := json.NewDecoder(r.Body)
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&request); err != nil {
		writeRecoveryJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid recovery request"})
		return
	}
	status, message := ui.performRestore(strings.TrimSpace(request.BackupID), request.Confirmation)
	if status != http.StatusOK {
		writeRecoveryJSON(w, status, map[string]string{"error": message})
		return
	}
	writeRecoveryJSON(w, http.StatusOK, map[string]bool{"success": true})
}

func (ui *recoveryUI) performRestore(backupID, confirmation string) (int, string) {
	if ui.finished.Load() {
		return http.StatusOK, ""
	}
	if confirmation != "RESTORE" {
		return http.StatusUnprocessableEntity, "Confirmation must exactly match RESTORE."
	}
	backupPath, err := ui.selectedBackup(backupID)
	if err != nil {
		return http.StatusUnprocessableEntity, err.Error()
	}
	if !ui.restoring.CompareAndSwap(false, true) {
		return http.StatusConflict, "a restore is already in progress"
	}
	defer ui.restoring.Store(false)
	audit, err := recovery.WriteAuditRecord(ui.config.ControlDir, "", "restore.requested", backupID, "started")
	if err != nil {
		return http.StatusInternalServerError, "Could not durably record the recovery request; no restore was started."
	}
	err = ui.config.Restore(ui.config.Context, backupPath)
	if err != nil {
		_, auditErr := recovery.WriteAuditRecord(ui.config.ControlDir, audit.OperationID, "restore.failed", backupID, "failure")
		pending, pendingErr := recovery.PendingRestoreJournals(ui.config.ControlDir)
		message := err.Error()
		if auditErr != nil {
			message += "; recovery failure audit could not be written"
		}
		if pendingErr == nil && len(pending) != 0 {
			message += "; a prepared restore journal exists, so restart Prods to continue the same roll-forward operation"
			ui.finish()
		}
		return http.StatusInternalServerError, message
	}
	if _, err := recovery.WriteAuditRecord(ui.config.ControlDir, audit.OperationID, "restore.completed", backupID, "success"); err != nil {
		ui.finished.Store(true)
		ui.finish()
		return http.StatusInternalServerError, "Restore completed, but final recovery audit could not be written. Restart Prods and inspect the control directory."
	}
	ui.finished.Store(true)
	ui.finish()
	return http.StatusOK, ""
}

func (ui *recoveryUI) exchangeToken(raw string) (string, bool) {
	ui.mu.Lock()
	defer ui.mu.Unlock()
	if ui.claimed || !ui.config.Now().UTC().Before(ui.expiresAt) ||
		subtle.ConstantTimeCompare([]byte(identity.TokenDigest(raw)), []byte(ui.tokenDigest)) != 1 {
		return "", false
	}
	session, err := identity.NewToken(32)
	if err != nil {
		return "", false
	}
	csrf, err := identity.NewToken(24)
	if err != nil {
		return "", false
	}
	ui.claimed = true
	ui.tokenDigest = ""
	ui.sessionDigest = identity.TokenDigest(session)
	ui.csrf = csrf
	return session, true
}

func (ui *recoveryUI) authorized(r *http.Request) bool {
	cookie, err := r.Cookie(recoverySessionCookie)
	if err != nil {
		return false
	}
	ui.mu.Lock()
	defer ui.mu.Unlock()
	return ui.claimed && ui.config.Now().UTC().Before(ui.expiresAt) &&
		subtle.ConstantTimeCompare([]byte(identity.TokenDigest(cookie.Value)), []byte(ui.sessionDigest)) == 1
}

func (ui *recoveryUI) selectedBackup(id string) (string, error) {
	if ui.config.RestoreDisabled {
		return "", errors.New("restore is disabled while recovery control records are ambiguous")
	}
	if id == "" || filepath.Base(id) != id || strings.ContainsAny(id, `/\\`) {
		return "", errors.New("invalid restore point")
	}
	if ui.config.AllowedBackupID != "" && id != ui.config.AllowedBackupID {
		return "", errors.New("only the backup already recorded by the prepared restore may be used")
	}
	manifests, err := recovery.ListBackups(ui.config.BackupDir)
	if err != nil {
		return "", fmt.Errorf("list configured restore points: %w", err)
	}
	for _, manifest := range manifests {
		if manifest.ID == id && manifest.ContentVerified {
			return filepath.Join(ui.config.BackupDir, manifest.ID), nil
		}
	}
	return "", errors.New("selected restore point is not a verified completed backup")
}

func (ui *recoveryUI) currentState() (recoveryState, error) {
	state := recoveryState{
		Diagnostic: ui.config.Diagnostic, CSRFToken: ui.csrf, Success: ui.finished.Load(),
		Restoring: ui.restoring.Load(), RestoreDisabled: ui.config.RestoreDisabled,
		Backups: make([]recovery.Manifest, 0),
	}
	if state.Success || state.RestoreDisabled {
		return state, nil
	}
	manifests, err := recovery.ListBackups(ui.config.BackupDir)
	if err != nil {
		return recoveryState{}, err
	}
	for _, manifest := range manifests {
		if manifest.ContentVerified && (ui.config.AllowedBackupID == "" || manifest.ID == ui.config.AllowedBackupID) {
			state.Backups = append(state.Backups, manifest)
		}
	}
	return state, nil
}

func (ui *recoveryUI) renderPage(w http.ResponseWriter) {
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	_, _ = w.Write([]byte(recoveryShell))
}

func writeRecoveryJSON(w http.ResponseWriter, status int, value any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(value)
}

func runRecoveryDiagnosticUI(ctx context.Context, options hostconfig.Options, diagnostic string, stop <-chan struct{}) error {
	return serveRecoveryUI(ctx, options, diagnostic, stop, "", nil)
}

func (ui *recoveryUI) writeDenied(w http.ResponseWriter) {
	w.Header().Set("Content-Type", "text/plain; charset=utf-8")
	w.WriteHeader(http.StatusForbidden)
	_, _ = w.Write([]byte("Recovery access requires the one-time URL shown in the Prods console.\n"))
}

func (ui *recoveryUI) finish() {
	ui.completeOnce.Do(func() { close(ui.config.Completed) })
}

func runRecoveryUI(ctx context.Context, options hostconfig.Options, diagnostic string, stop <-chan struct{}) error {
	gate := platform.NewResourceGate(nil)
	return serveRecoveryUI(ctx, options, diagnostic, stop, "", func(restoreCtx context.Context, selected string) error {
		return runOfflineRestore(restoreCtx, filepath.Join(options.DataDir, "prods.db"), options.DataDir,
			options.BackupDir, options.ConfigPath, selected, true, gate, externalBackupRequirements(options))
	})
}

type preparedRestoreRecoveryPlan struct {
	allowedBackupID string
	restore         func(context.Context, string) error
}

func newPreparedRestoreRecoveryPlan(options hostconfig.Options, journalPath string, resume func(context.Context, string) error) (preparedRestoreRecoveryPlan, error) {
	journal, err := recovery.LoadRestoreJournal(journalPath)
	if err != nil {
		return preparedRestoreRecoveryPlan{}, fmt.Errorf("load prepared restore for Recovery UI: %w", err)
	}
	if journal.Phase != recovery.RestorePrepared {
		return preparedRestoreRecoveryPlan{}, errors.New("Recovery UI may only resume a prepared restore")
	}
	if resume == nil {
		return preparedRestoreRecoveryPlan{}, errors.New("prepared restore resume function is required")
	}
	return preparedRestoreRecoveryPlan{
		allowedBackupID: journal.ManifestID,
		restore: func(resumeCtx context.Context, _ string) error {
			if err := resume(resumeCtx, journalPath); err != nil {
				return err
			}
			return recovery.SupersedeMigrationJournals(filepath.Join(options.DataDir, "control"), journal.OperationID)
		},
	}, nil
}

func runPreparedRestoreRecoveryUI(ctx context.Context, options hostconfig.Options, diagnostic, journalPath string, stop <-chan struct{}) error {
	plan, err := newPreparedRestoreRecoveryPlan(options, journalPath, recovery.ResumeRestore)
	if err != nil {
		return err
	}
	return serveRecoveryUI(ctx, options, diagnostic, stop, plan.allowedBackupID, plan.restore)
}

func serveRecoveryUI(ctx context.Context, options hostconfig.Options, diagnostic string, stop <-chan struct{},
	allowedBackupID string, restore func(context.Context, string) error) error {
	if stop != nil {
		return errors.New("Recovery Required: stop the Prods service and run the same binary interactively to receive a one-time recovery URL")
	}
	listener, err := net.Listen("tcp", options.Listen)
	if err != nil {
		return fmt.Errorf("listen for Recovery UI on %s: %w", options.Listen, err)
	}
	completed := make(chan struct{})
	controlDir := filepath.Join(options.DataDir, "control")
	ui, token, err := newRecoveryUI(recoveryUIConfig{
		Context: ctx, Diagnostic: diagnostic, BackupDir: options.BackupDir, ControlDir: controlDir, Completed: completed,
		AllowedBackupID: allowedBackupID, Restore: restore, RestoreDisabled: restore == nil,
	})
	if err != nil {
		_ = listener.Close()
		return err
	}
	fmt.Printf("Recovery Required. Open this one-time URL within 30 minutes:\n%s\n",
		localRecoveryURL(listener.Addr().String(), token))
	return serveListenerWithStop(listener, ui, completed, 10*time.Second, nil)
}

func localRecoveryURL(listen, token string) string {
	host, port, err := net.SplitHostPort(listen)
	if err != nil {
		host, port = "127.0.0.1", "8080"
	}
	if host == "" || host == "0.0.0.0" || host == "::" || host == "[::]" {
		host = "127.0.0.1"
	}
	location := &url.URL{Scheme: "http", Host: net.JoinHostPort(host, port), Path: "/recovery"}
	query := location.Query()
	query.Set("token", token)
	location.RawQuery = query.Encode()
	return location.String()
}

func reconcileRecoveryAudit(ctx context.Context, store interface {
	ImportRecoveryAudit(context.Context, string, string, string, string, string, string) error
}, controlDir string) error {
	records, err := recovery.ListAuditRecords(controlDir)
	if err != nil {
		return err
	}
	for _, record := range records {
		if err := store.ImportRecoveryAudit(ctx, record.EventID, record.OperationID, record.Action,
			record.BackupID, record.Result, record.CreatedUTC); err != nil {
			return err
		}
	}
	return nil
}
