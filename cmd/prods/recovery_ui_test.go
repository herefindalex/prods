package main

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/cookiejar"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"prods/internal/hostconfig"
	"prods/internal/identity"
	"prods/internal/recovery"
	"prods/internal/storage/sqlite"
)

func TestRecoveryUIExchangesOneTimeTokenListsManifestAndRequiresCSRFConfirmation(t *testing.T) {
	root := t.TempDir()
	store, owner := recoveryUITestStore(t, root)
	_ = owner
	assetDir := filepath.Join(root, "assets")
	backupDir := filepath.Join(root, "backups")
	controlDir := filepath.Join(root, "control")
	if err := os.MkdirAll(assetDir, 0o700); err != nil {
		t.Fatal(err)
	}
	manifest, err := recovery.CreateBackup(t.Context(), store, recovery.BackupConfig{
		BackupDir: backupDir, AssetDir: assetDir, ApplicationVersion: "test", Kind: "manual",
	})
	if err != nil {
		t.Fatal(err)
	}
	completed := make(chan struct{})
	var restoredPath string
	ui, token, err := newRecoveryUI(recoveryUIConfig{
		Context: t.Context(), Diagnostic: "database quick-check failed", BackupDir: backupDir,
		ControlDir: controlDir, Completed: completed,
		Restore: func(_ context.Context, selected string) error {
			restoredPath = selected
			return nil
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	server := httptest.NewServer(ui)
	t.Cleanup(server.Close)
	jar, err := cookiejar.New(nil)
	if err != nil {
		t.Fatal(err)
	}
	client := &http.Client{Jar: jar}
	response, err := client.Get(server.URL + "/recovery?token=" + url.QueryEscape(token))
	if err != nil {
		t.Fatal(err)
	}
	body, err := io.ReadAll(response.Body)
	response.Body.Close()
	if err != nil {
		t.Fatal(err)
	}
	if response.StatusCode != http.StatusOK || !strings.Contains(string(body), `data-mode="recovery"`) ||
		!strings.Contains(string(body), `/static/system/system.css`) || !strings.Contains(string(body), `/static/system/system.js`) ||
		strings.Contains(string(body), manifest.ID) || strings.Contains(string(body), "database quick-check failed") || strings.Contains(string(body), token) {
		t.Fatalf("claimed recovery page status=%d body=%s", response.StatusCode, body)
	}
	stateResponse, err := client.Get(server.URL + "/recovery/api/state")
	if err != nil {
		t.Fatal(err)
	}
	var state recoveryState
	if err := json.NewDecoder(stateResponse.Body).Decode(&state); err != nil {
		stateResponse.Body.Close()
		t.Fatal(err)
	}
	stateResponse.Body.Close()
	if stateResponse.StatusCode != http.StatusOK || state.Diagnostic != "database quick-check failed" ||
		len(state.Backups) != 1 || state.Backups[0].ID != manifest.ID || state.CSRFToken == "" {
		t.Fatalf("recovery state status=%d state=%+v", stateResponse.StatusCode, state)
	}
	second, err := http.Get(server.URL + "/recovery?token=" + url.QueryEscape(token))
	if err != nil {
		t.Fatal(err)
	}
	second.Body.Close()
	if second.StatusCode != http.StatusForbidden {
		t.Fatalf("reused recovery token=%d", second.StatusCode)
	}

	wrong := postRecoveryJSON(t, client, server.URL, state.CSRFToken, manifest.ID, "restore")
	wrong.Body.Close()
	if wrong.StatusCode != http.StatusUnprocessableEntity || restoredPath != "" {
		t.Fatalf("wrong confirmation status=%d restored=%q", wrong.StatusCode, restoredPath)
	}
	withoutCSRF := postRecoveryJSON(t, client, server.URL, "", manifest.ID, "RESTORE")
	withoutCSRF.Body.Close()
	if withoutCSRF.StatusCode != http.StatusForbidden || restoredPath != "" {
		t.Fatalf("restore without CSRF status=%d restored=%q", withoutCSRF.StatusCode, restoredPath)
	}
	invalidPath := postRecoveryJSON(t, client, server.URL, state.CSRFToken, "../outside", "RESTORE")
	invalidPath.Body.Close()
	if invalidPath.StatusCode != http.StatusUnprocessableEntity || restoredPath != "" {
		t.Fatalf("unsafe restore selection status=%d restored=%q", invalidPath.StatusCode, restoredPath)
	}

	success := postRecoveryJSON(t, client, server.URL, state.CSRFToken, manifest.ID, "RESTORE")
	successBody, err := io.ReadAll(success.Body)
	success.Body.Close()
	if err != nil {
		t.Fatal(err)
	}
	if success.StatusCode != http.StatusOK || !strings.Contains(string(successBody), `"success":true`) ||
		restoredPath != filepath.Join(backupDir, manifest.ID) {
		t.Fatalf("restore status=%d path=%q body=%s", success.StatusCode, restoredPath, successBody)
	}
	select {
	case <-completed:
	case <-time.After(time.Second):
		t.Fatal("successful recovery did not close the recovery session")
	}
	audits, err := recovery.ListAuditRecords(controlDir)
	if err != nil {
		t.Fatal(err)
	}
	if len(audits) != 2 || audits[0].Action != "restore.requested" || audits[1].Action != "restore.completed" ||
		audits[0].OperationID != audits[1].OperationID {
		t.Fatalf("recovery audit=%+v", audits)
	}
	for _, audit := range audits {
		if strings.Contains(audit.EventID+audit.OperationID+audit.Action+audit.BackupID+audit.Result+audit.CreatedUTC, token) {
			t.Fatal("raw Recovery UI token leaked into durable recovery audit")
		}
	}
}

func TestRecoveryUIAllowedBackupIDRestrictsPreparedRestoreSelection(t *testing.T) {
	root := t.TempDir()
	store, _ := recoveryUITestStore(t, root)
	assetDir := filepath.Join(root, "assets")
	backupDir := filepath.Join(root, "backups")
	if err := os.MkdirAll(assetDir, 0o700); err != nil {
		t.Fatal(err)
	}
	first, err := recovery.CreateBackup(t.Context(), store, recovery.BackupConfig{
		BackupDir: backupDir, AssetDir: assetDir, ApplicationVersion: "test", Kind: "manual",
	})
	if err != nil {
		t.Fatal(err)
	}
	second, err := recovery.CreateBackup(t.Context(), store, recovery.BackupConfig{
		BackupDir: backupDir, AssetDir: assetDir, ApplicationVersion: "test", Kind: "manual",
	})
	if err != nil {
		t.Fatal(err)
	}
	ui, _, err := newRecoveryUI(recoveryUIConfig{
		BackupDir: backupDir, ControlDir: filepath.Join(root, "control"), AllowedBackupID: first.ID,
		Restore: func(context.Context, string) error { return nil },
	})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := ui.selectedBackup(second.ID); err == nil {
		t.Fatal("prepared Recovery UI accepted a different verified backup")
	}
	selected, err := ui.selectedBackup(first.ID)
	if err != nil || selected != filepath.Join(backupDir, first.ID) {
		t.Fatalf("prepared backup selection=%q err=%v", selected, err)
	}
	state, err := ui.currentState()
	if err != nil {
		t.Fatal(err)
	}
	if len(state.Backups) != 1 || state.Backups[0].ID != first.ID {
		t.Fatalf("prepared Recovery UI backup list=%+v", state.Backups)
	}
}

func postRecoveryJSON(t *testing.T, client *http.Client, serverURL, csrf, backupID, confirmation string) *http.Response {
	t.Helper()
	body, err := json.Marshal(map[string]string{"backup_id": backupID, "confirmation": confirmation})
	if err != nil {
		t.Fatal(err)
	}
	request, err := http.NewRequest(http.MethodPost, serverURL+"/recovery/api/restore", strings.NewReader(string(body)))
	if err != nil {
		t.Fatal(err)
	}
	request.Header.Set("Content-Type", "application/json")
	if csrf != "" {
		request.Header.Set("X-CSRF-Token", csrf)
	}
	response, err := client.Do(request)
	if err != nil {
		t.Fatal(err)
	}
	return response
}

func TestPreparedRestoreRecoveryPlanAlwaysResumesOriginalJournal(t *testing.T) {
	root := t.TempDir()
	controlDir := filepath.Join(root, "control")
	if err := os.MkdirAll(controlDir, 0o700); err != nil {
		t.Fatal(err)
	}
	journalPath := filepath.Join(controlDir, "restore-operation-1.json")
	journal := `{"operation_id":"operation-1","manifest_path":"/backup/original/manifest.json","manifest_id":"original-backup","phase":"prepared","order":["database","assets"]}`
	if err := os.WriteFile(journalPath, []byte(journal), 0o600); err != nil {
		t.Fatal(err)
	}
	var resumedPath string
	plan, err := newPreparedRestoreRecoveryPlan(hostconfig.Options{DataDir: root}, journalPath, func(_ context.Context, path string) error {
		resumedPath = path
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	if plan.allowedBackupID != "original-backup" {
		t.Fatalf("allowed backup=%q", plan.allowedBackupID)
	}
	if err := plan.restore(t.Context(), "/backup/attacker-selected"); err != nil {
		t.Fatal(err)
	}
	if resumedPath != journalPath {
		t.Fatalf("resumed journal=%q want %q", resumedPath, journalPath)
	}
}

func TestRecoveryUIDiagnosticOnlyRefusesRestore(t *testing.T) {
	ui, token, err := newRecoveryUI(recoveryUIConfig{
		BackupDir: t.TempDir(), ControlDir: t.TempDir(), RestoreDisabled: true,
	})
	if err != nil {
		t.Fatal(err)
	}
	session, ok := ui.exchangeToken(token)
	if !ok {
		t.Fatal("could not claim diagnostic Recovery UI")
	}
	form := url.Values{
		"csrf": {ui.csrf}, "backup_id": {"any-backup"}, "confirmation": {"RESTORE"},
	}
	request := httptest.NewRequest(http.MethodPost, "/recovery/restore", strings.NewReader(form.Encode()))
	request.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	request.AddCookie(&http.Cookie{Name: recoverySessionCookie, Value: session})
	recorder := httptest.NewRecorder()
	ui.ServeHTTP(recorder, request)
	if recorder.Code != http.StatusUnprocessableEntity || !strings.Contains(recorder.Body.String(), "restore is disabled") {
		t.Fatalf("diagnostic-only restore status=%d body=%s", recorder.Code, recorder.Body.String())
	}
}

func TestRecoveryUISystemShellAndAssetsStayIndependentOfSiteDatabase(t *testing.T) {
	ui, token, err := newRecoveryUI(recoveryUIConfig{
		Diagnostic: "site database cannot be opened", BackupDir: t.TempDir(), ControlDir: t.TempDir(), RestoreDisabled: true,
	})
	if err != nil {
		t.Fatal(err)
	}
	server := httptest.NewServer(ui)
	t.Cleanup(server.Close)

	unauthorized, err := http.Get(server.URL + "/recovery/api/state")
	if err != nil {
		t.Fatal(err)
	}
	unauthorized.Body.Close()
	if unauthorized.StatusCode != http.StatusForbidden {
		t.Fatalf("unauthorized recovery state=%d", unauthorized.StatusCode)
	}

	jar, err := cookiejar.New(nil)
	if err != nil {
		t.Fatal(err)
	}
	client := &http.Client{Jar: jar}
	page, err := client.Get(server.URL + "/recovery?token=" + url.QueryEscape(token))
	if err != nil {
		t.Fatal(err)
	}
	pageBody, err := io.ReadAll(page.Body)
	page.Body.Close()
	if err != nil {
		t.Fatal(err)
	}
	body := string(pageBody)
	if page.StatusCode != http.StatusOK || !strings.Contains(body, `data-mode="recovery"`) ||
		!strings.Contains(body, "requires JavaScript") || strings.Contains(body, "/static/admin/") ||
		strings.Contains(body, "/static/public/") || strings.Contains(body, "/site.css") ||
		!strings.Contains(page.Header.Get("Content-Security-Policy"), "connect-src 'self'") {
		t.Fatalf("recovery shell status=%d headers=%v body=%s", page.StatusCode, page.Header, body)
	}
	for _, asset := range []string{"/static/system/system.css", "/static/system/system.js"} {
		response, err := http.Get(server.URL + asset)
		if err != nil {
			t.Fatal(err)
		}
		response.Body.Close()
		if response.StatusCode != http.StatusOK {
			t.Fatalf("Recovery system asset %s status=%d", asset, response.StatusCode)
		}
	}
	stateResponse, err := client.Get(server.URL + "/recovery/api/state")
	if err != nil {
		t.Fatal(err)
	}
	var state recoveryState
	if err := json.NewDecoder(stateResponse.Body).Decode(&state); err != nil {
		stateResponse.Body.Close()
		t.Fatal(err)
	}
	stateResponse.Body.Close()
	if stateResponse.StatusCode != http.StatusOK || state.Diagnostic != "site database cannot be opened" || !state.RestoreDisabled {
		t.Fatalf("database-independent recovery state status=%d state=%+v", stateResponse.StatusCode, state)
	}
}

func TestRecoveryUITokenExpiresAndReadinessRemainsFalse(t *testing.T) {
	now := time.Date(2026, 9, 14, 12, 0, 0, 0, time.UTC)
	ui, token, err := newRecoveryUI(recoveryUIConfig{
		BackupDir: t.TempDir(), ControlDir: t.TempDir(), SessionTTL: time.Minute,
		Now: func() time.Time { return now }, Restore: func(context.Context, string) error { return nil },
	})
	if err != nil {
		t.Fatal(err)
	}
	server := httptest.NewServer(ui)
	t.Cleanup(server.Close)
	ready, err := http.Get(server.URL + "/health/ready")
	if err != nil {
		t.Fatal(err)
	}
	ready.Body.Close()
	if ready.StatusCode != http.StatusServiceUnavailable {
		t.Fatalf("Recovery UI readiness=%d", ready.StatusCode)
	}
	now = now.Add(2 * time.Minute)
	response, err := http.Get(server.URL + "/recovery?token=" + token)
	if err != nil {
		t.Fatal(err)
	}
	response.Body.Close()
	if response.StatusCode != http.StatusForbidden {
		t.Fatalf("expired Recovery UI token=%d", response.StatusCode)
	}
}

func TestRecoveryAuditReconciliationImportsEachEventOnce(t *testing.T) {
	root := t.TempDir()
	store, _ := recoveryUITestStore(t, root)
	controlDir := filepath.Join(root, "control")
	requested, err := recovery.WriteAuditRecord(controlDir, "", "restore.requested", "backup-1", "started")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := recovery.WriteAuditRecord(controlDir, requested.OperationID, "restore.completed", "backup-1", "success"); err != nil {
		t.Fatal(err)
	}
	for attempt := 0; attempt < 2; attempt++ {
		if err := reconcileRecoveryAudit(t.Context(), store, controlDir); err != nil {
			t.Fatal(err)
		}
	}
	entries, err := store.ListAudit(t.Context(), 100)
	if err != nil {
		t.Fatal(err)
	}
	count := 0
	for _, entry := range entries {
		if strings.HasPrefix(entry.Action, "restore.") {
			count++
		}
	}
	if count != 2 {
		t.Fatalf("imported recovery audit entries=%d", count)
	}
}

func TestLocalRecoveryURLUsesLoopbackAndEscapesToken(t *testing.T) {
	if got := localRecoveryURL("[::]:9090", "token with spaces"); got != "http://127.0.0.1:9090/recovery?token=token+with+spaces" {
		t.Fatalf("Recovery URL=%q", got)
	}
}

func recoveryUITestStore(t *testing.T, root string) (*sqlite.Store, identity.User) {
	t.Helper()
	store, err := sqlite.Create(filepath.Join(root, "prods.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = store.Close() })
	passwordHash, err := identity.HashPassword("ownerpass1")
	if err != nil {
		t.Fatal(err)
	}
	owner, err := store.CompleteInstallation(t.Context(), sqlite.Installation{
		OwnerEmail: "owner@example.test", OwnerDisplayName: "Owner", PasswordHash: passwordHash,
		DefaultLocale: "en-US", SupportedLocales: []string{"en-US"}, TimeZone: "UTC",
	})
	if err != nil {
		t.Fatal(err)
	}
	return store, owner
}
