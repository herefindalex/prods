package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"io"
	"log/slog"
	"net"
	"net/http"
	"net/url"
	"os"
	"os/signal"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"syscall"
	"time"

	"prods/internal/consoleui"
	"prods/internal/distribution"
	"prods/internal/hostconfig"
	"prods/internal/maildelivery"
	"prods/internal/platform"
	"prods/internal/recovery"
	"prods/internal/searchnotify"
	"prods/internal/storage/sqlite"
	"prods/internal/webapp"
)

var (
	applicationVersion = "dev"
	sourceRevision     = "unknown"
)

func main() {
	serviceProcess, detectionErr := platform.IsServiceProcess()
	if detectionErr != nil {
		slog.Error("detect service process", "error", detectionErr)
		os.Exit(1)
	}
	var err error
	if serviceProcess {
		err = platform.RunService(platform.DefaultServiceName, func(stop <-chan struct{}) error {
			return runWithStop(stop)
		})
	} else {
		err = run()
	}
	if normalTermination(err) {
		return
	}
	slog.Error("prods stopped", "error", err)
	os.Exit(1)
}

func normalTermination(err error) bool {
	return err == nil || errors.Is(err, flag.ErrHelp)
}

func run() error {
	return runWithStop(nil)
}

func runWithStop(stop <-chan struct{}) (returnErr error) {
	executable, err := os.Executable()
	if err != nil {
		return fmt.Errorf("resolve executable: %w", err)
	}
	options, err := (hostconfig.Resolver{Executable: executable, Args: os.Args[1:]}).Resolve()
	if err != nil {
		return err
	}
	if options.Version {
		fmt.Printf("Prods %s\nSource %s\nGo %s\n", applicationVersion, sourceRevision, runtime.Version())
		return nil
	}
	if options.UninstallService {
		return platform.UninstallService(context.Background(), platform.DefaultServiceName)
	}
	runtimeLog, err := platform.OpenRuntimeLog(filepath.Join(options.DataDir, "logs"), 0, 0)
	if err != nil {
		return err
	}
	hostConsole := consoleui.New(os.Stdin, os.Stderr)
	previousLogger := slog.Default()
	slog.SetDefault(slog.New(slog.NewJSONHandler(io.MultiWriter(runtimeLog, hostConsole), nil)))
	defer func() {
		if returnErr != nil {
			slog.Error("Prods runtime stopped", "error", returnErr)
		}
		if consoleErr := hostConsole.Close(); consoleErr != nil && returnErr == nil {
			returnErr = fmt.Errorf("close console UI: %w", consoleErr)
		}
		slog.SetDefault(previousLogger)
		if closeErr := runtimeLog.Close(); closeErr != nil && returnErr == nil {
			returnErr = fmt.Errorf("close runtime log: %w", closeErr)
		}
	}()
	runtimeCtx, cancelRuntime := context.WithCancel(context.Background())
	defer cancelRuntime()
	if stop != nil {
		go func() {
			select {
			case <-stop:
				cancelRuntime()
			case <-runtimeCtx.Done():
			}
		}()
	}
	listen := &options.Listen
	dataDir := &options.DataDir
	backupDir := &options.BackupDir
	adminToken := &options.AdminToken
	baseURL := &options.BaseURL
	pocFixtures := &options.POCFixtures
	restoreBackup := &options.RestoreBackup
	allowRestoreWithoutPrebackup := &options.AllowRestoreWithoutPrebackup
	backupNow := &options.BackupNow
	recoverOwner := &options.RecoverOwner
	resourceGate := platform.NewResourceGate(nil)

	if err := os.MkdirAll(*dataDir, 0o700); err != nil {
		return fmt.Errorf("create data directory: %w", err)
	}
	dbPath := filepath.Join(*dataDir, "prods.db")
	ownership, err := platform.AcquireDatabase(dbPath)
	if err != nil {
		return err
	}
	defer ownership.Close()
	controlDir := filepath.Join(*dataDir, "control")
	pendingRestores, err := recovery.PendingRestoreJournals(controlDir)
	if err != nil {
		return fmt.Errorf("inspect restore journal: %w", err)
	}
	if len(pendingRestores) > 1 {
		return runRecoveryDiagnosticUI(runtimeCtx, options,
			fmt.Sprintf("Multiple prepared restore journals were found in %s. Prods will not choose, resume, or replace either operation automatically.", controlDir), hostConsole, stop)
	}
	if len(pendingRestores) == 1 {
		if err := recovery.ResumeRestore(runtimeCtx, pendingRestores[0]); err != nil {
			return runPreparedRestoreRecoveryUI(runtimeCtx, options,
				fmt.Sprintf("Prepared restore could not complete automatically: %v", err), pendingRestores[0], hostConsole, stop)
		}
		return errors.New("prepared restore completed and verified; restart Prods to run normal startup checks")
	}
	if strings.TrimSpace(*restoreBackup) != "" {
		if err := runOfflineRestore(runtimeCtx, dbPath, *dataDir, *backupDir, options.ConfigPath, *restoreBackup,
			*allowRestoreWithoutPrebackup, resourceGate, externalBackupRequirements(options)); err != nil {
			return err
		}
		slog.Info("restore completed and verified", "restart_required", true)
		fmt.Fprintln(os.Stdout, "Restore completed and verified. Restart Prods to run normal startup checks.")
		return nil
	}
	pendingMigrations, err := recovery.PendingMigrationJournals(controlDir)
	if err != nil {
		return fmt.Errorf("inspect migration journal: %w", err)
	}
	if len(pendingMigrations) > 1 {
		return runRecoveryUI(runtimeCtx, options,
			fmt.Sprintf("Multiple pending migration journals were found in %s; select a verified restore point.", controlDir), hostConsole, stop)
	}
	if len(pendingMigrations) == 1 {
		if err := resumeMigration(runtimeCtx, dbPath, pendingMigrations[0]); err != nil {
			return runRecoveryUI(runtimeCtx, options, fmt.Sprintf("Schema migration reconciliation failed: %v", err), hostConsole, stop)
		}
		return errors.New("pending schema migration reconciled; restart Prods to run normal startup checks")
	}
	inspection := sqlite.Inspect(dbPath)
	slog.Info("resolved Prods runtime",
		"version", applicationVersion,
		"go", runtime.Version(),
		"config", options.ConfigPath,
		"listen", options.Listen,
		"base_url", options.BaseURL,
		"data_dir", *dataDir,
		"backup_dir", *backupDir,
		"database", ownership.ResourcePath(),
		"database_state", inspection.State,
		"instance_kind", inspection.Kind,
	)
	if *recoverOwner != "" {
		if inspection.State != sqlite.DatabaseReady || inspection.Kind != sqlite.DatabaseKindSite {
			return fmt.Errorf("owner recovery requires a completed site database; current state=%s kind=%s", inspection.State, inspection.Kind)
		}
		store, err := sqlite.OpenReady(dbPath)
		if err != nil {
			return err
		}
		recoveryURL, expiresAt, recoveryErr := issueOwnerRecovery(runtimeCtx, store, *baseURL, *recoverOwner)
		closeErr := store.Close()
		if recoveryErr != nil {
			return recoveryErr
		}
		if closeErr != nil {
			return closeErr
		}
		fmt.Printf("Owner recovery link (expires %s):\n%s\nRestart Prods normally, then open this link before it expires.\n",
			expiresAt.Format(time.RFC3339), recoveryURL)
		return nil
	}
	if options.InstallService {
		if inspection.State != sqlite.DatabaseReady || inspection.Kind != sqlite.DatabaseKindSite {
			return fmt.Errorf("service installation requires a completed site database; current state=%s kind=%s", inspection.State, inspection.Kind)
		}
		if !options.ConfigLoaded {
			if err := persistInitialHostConfig(&options); err != nil {
				return err
			}
		}
		if err := ownership.Close(); err != nil {
			return fmt.Errorf("release foreground database ownership before service start: %w", err)
		}
		confirmedExecutable, err := filepath.EvalSymlinks(executable)
		if err != nil {
			confirmedExecutable, err = filepath.Abs(executable)
			if err != nil {
				return fmt.Errorf("resolve service executable: %w", err)
			}
		}
		return platform.InstallService(runtimeCtx, platform.ServiceSpec{
			Name:        platform.DefaultServiceName,
			DisplayName: "Prods",
			Description: "Prods catalog and RFQ service",
			Executable:  confirmedExecutable,
			ConfigPath:  options.ConfigPath,
			DataDir:     options.DataDir,
			BackupDir:   options.BackupDir,
			User:        options.ServiceUser,
		})
	}
	if *backupNow {
		if inspection.State != sqlite.DatabaseReady {
			return fmt.Errorf("backup requires a ready database; current state is %s", inspection.State)
		}
		store, err := sqlite.OpenReady(dbPath)
		if err != nil {
			return err
		}
		manifest, backupErr := createBackup(runtimeCtx, store, dbPath, resourceGate, recovery.BackupConfig{
			BackupDir: *backupDir, AssetDir: filepath.Join(*dataDir, "assets"),
			ApplicationVersion: applicationVersion, Kind: "manual", Roots: existingBackupRoots(*dataDir), Files: existingBackupFiles(options.ConfigPath),
			ExternalRequirements: externalBackupRequirements(options),
		})
		closeErr := store.Close()
		if backupErr != nil {
			return backupErr
		}
		if closeErr != nil {
			return closeErr
		}
		slog.Info("backup created", "backup_id", manifest.ID, "created_utc", manifest.CreatedUTC,
			"content_verified", manifest.ContentVerified, "read_only_applied", manifest.ReadOnlyApplied)
		return nil
	}

	if inspection.State == sqlite.DatabaseUpgrade {
		store, err := sqlite.OpenForUpgrade(dbPath)
		if err != nil {
			return err
		}
		manifest, backupErr := createBackup(runtimeCtx, store, dbPath, resourceGate, recovery.BackupConfig{
			BackupDir:          *backupDir,
			AssetDir:           filepath.Join(*dataDir, "assets"),
			ApplicationVersion: applicationVersion, Kind: "pre-upgrade",
			ExternalRequirements: externalBackupRequirements(options),
			Roots:                existingBackupRoots(*dataDir), Files: existingBackupFiles(options.ConfigPath),
		})
		if backupErr != nil {
			_ = store.Close()
			return fmt.Errorf("pre-upgrade backup failed; schema was not changed: %w", backupErr)
		}
		plan, err := sqlite.PendingMigrations(inspection.SchemaVersion)
		if err != nil {
			_ = store.Close()
			return err
		}
		journalPath, err := recovery.PrepareMigrationJournal(
			controlDir,
			filepath.Join(*backupDir, manifest.ID),
			inspection.SchemaVersion,
			sqlite.CurrentSchemaVersion,
			migrationSteps(plan),
		)
		if err != nil {
			_ = store.Close()
			return fmt.Errorf("prepare migration journal: %w", err)
		}
		if err := store.Upgrade(runtimeCtx, manifest.ID); err != nil {
			_ = store.Close()
			if journalErr := recovery.MarkMigrationFailed(journalPath, err); journalErr != nil {
				return fmt.Errorf("recovery required: schema upgrade failed after backup %s: %v; record failure: %w", manifest.ID, err, journalErr)
			}
			return fmt.Errorf("recovery required: schema upgrade failed after verified backup %s: %w", manifest.ID, err)
		}
		if err := store.Close(); err != nil {
			return err
		}
		if err := recovery.CompleteMigrationJournal(journalPath, sqlite.CurrentSchemaVersion); err != nil {
			return fmt.Errorf("recovery required: migration committed but completion evidence failed: %w", err)
		}
		slog.Info("schema upgrade completed", "from", inspection.SchemaVersion, "to", sqlite.CurrentSchemaVersion,
			"backup_id", manifest.ID, "backup_content_verified", manifest.ContentVerified, "backup_read_only_applied", manifest.ReadOnlyApplied)
		return errors.New("schema upgrade completed; restart Prods to run normal startup checks")
	}
	if inspection.State == sqlite.DatabaseRecovery {
		return runRecoveryUI(runtimeCtx, options, inspection.Reason, hostConsole, stop)
	}
	if inspection.State == sqlite.DatabaseInstalling {
		if *pocFixtures || *adminToken != "" {
			return errors.New("an incomplete real installation cannot be opened with PoC credentials")
		}
		store, err := sqlite.OpenInstalling(dbPath)
		if err != nil {
			return err
		}
		defer store.Close()
		listener, err := prepareInstallerListener(&options)
		if err != nil {
			return err
		}
		defer listener.Close()
		return runInstaller(runtimeCtx, store, listener, options, dbPath, hostConsole, stop)
	}
	if inspection.State == sqlite.DatabaseFresh && !*pocFixtures {
		if *adminToken != "" {
			return errors.New("temporary Admin token requires -poc-fixtures")
		}
		listener, err := prepareInstallerListener(&options)
		if err != nil {
			return err
		}
		defer listener.Close()
		store, err := sqlite.Create(dbPath)
		if err != nil {
			return err
		}
		defer store.Close()
		return runInstaller(runtimeCtx, store, listener, options, dbPath, hostConsole, stop)
	}

	var store *sqlite.Store
	if inspection.State == sqlite.DatabaseFresh {
		store, err = sqlite.CreatePOC(dbPath)
		inspection.Kind = sqlite.DatabaseKindPOC
	} else {
		store, err = sqlite.OpenReady(dbPath)
	}
	if err != nil {
		return err
	}
	defer store.Close()
	if inspection.Kind == sqlite.DatabaseKindSite && (*pocFixtures || *adminToken != "") {
		return errors.New("PoC credentials cannot be enabled for a normal installed site")
	}
	if inspection.Kind == sqlite.DatabaseKindSite {
		if err := reconcileRecoveryAudit(runtimeCtx, store, controlDir); err != nil {
			return fmt.Errorf("reconcile recovery audit: %w", err)
		}
	}
	secureCookies, err := validateBaseURL(*baseURL)
	if err != nil {
		return err
	}
	pocMode := inspection.Kind == sqlite.DatabaseKindPOC
	var mailSender maildelivery.Sender
	var googleSearchSubmitter searchnotify.GoogleSubmitter
	if !pocMode {
		mailSender, err = configuredMailSender(options)
		if err != nil {
			return err
		}
		googleSearchSubmitter, err = configuredGoogleSearchSubmitter(options)
		if err != nil {
			return err
		}
	}
	app, generatedToken, err := webapp.New(store, webapp.Config{
		BaseURL: *baseURL, AdminToken: *adminToken, EnablePOCAdmin: pocMode,
		SecureCookies: secureCookies, EnforceHost: true, WorkDir: filepath.Join(*dataDir, "work"),
		PublicDir: filepath.Join(*dataDir, "generated", "public"),
		AssetDir:  filepath.Join(*dataDir, "assets"), DatabasePath: dbPath, BackupDir: *backupDir, ResourceGate: resourceGate,
		EnableBackupScheduler: !pocMode, EnableAssetGC: !pocMode, AssetGCGrace: time.Duration(options.AssetGCGraceDays) * 24 * time.Hour, TrustedProxyCIDRs: options.TrustedProxyCIDRs, BackupRoots: existingBackupRoots(*dataDir), BackupFiles: existingBackupFiles(options.ConfigPath), ApplicationVersion: applicationVersion, DistributionClient: distribution.NewClient(nil),
		MailSender:                 mailSender,
		IndexNowSubmitter:          searchnotify.HTTPIndexNowClient{},
		GoogleSearchSubmitter:      googleSearchSubmitter,
		BackupExternalRequirements: externalBackupRequirements(options),
		RuntimeLogPath:             filepath.Join(*dataDir, "logs", "prods.log"),
		RuntimeLogFiles:            platform.DefaultRuntimeLogFiles,
	})
	if err != nil {
		return err
	}
	if generatedToken != "" {
		if !hostConsole.Interactive() {
			fmt.Printf("Temporary PoC Admin login token (console only):\n%s\n", generatedToken)
		}
	}
	defer app.Close()
	listener, err := net.Listen("tcp", *listen)
	if err != nil {
		return fmt.Errorf("listen on %s: %w; choose another --listen address or stop the conflicting process", *listen, err)
	}
	defer listener.Close()
	guide := runtimeConsoleGuide(options, "正常執行", "服務已啟動，請在瀏覽器開啟服務網址。", *baseURL, listener.Addr().String(), ownership.ResourcePath())
	if generatedToken != "" {
		guide.Action = "服務已啟動。請在瀏覽器開啟服務網址，並使用下列臨時 Admin token 登入。"
		guide.Details = []string{"臨時 Admin token：" + generatedToken}
	}
	if err := hostConsole.Start(runtimeCtx, guide); err != nil {
		return fmt.Errorf("start console UI: %w", err)
	}
	slog.Info("Prods listening", "address", listener.Addr().String(), "mode", "normal", "instance_kind", inspection.Kind, "base_url", *baseURL)
	return serveListenerWithStop(listener, app, nil, 10*time.Second, consoleStop(stop, hostConsole))
}

func configuredMailSender(options hostconfig.Options) (maildelivery.Sender, error) {
	if strings.TrimSpace(options.SMTPHost) == "" {
		return nil, nil
	}
	password := options.SMTPPassword
	if password == "" && options.SMTPPasswordFile != "" {
		info, err := os.Stat(options.SMTPPasswordFile)
		if err != nil {
			return nil, fmt.Errorf("read SMTP password file metadata: %w", err)
		}
		if !info.Mode().IsRegular() || info.Size() < 1 || info.Size() > 64<<10 {
			return nil, errors.New("SMTP password file must be a non-empty regular file no larger than 64 KiB")
		}
		body, err := os.ReadFile(options.SMTPPasswordFile)
		if err != nil {
			return nil, fmt.Errorf("read SMTP password file: %w", err)
		}
		password = strings.TrimRight(string(body), "\r\n")
		if password == "" {
			return nil, errors.New("SMTP password file is empty")
		}
	}
	sender, err := maildelivery.NewSMTPSender(maildelivery.SMTPConfig{
		Host: options.SMTPHost, Port: options.SMTPPort, Username: options.SMTPUsername,
		Password: password, From: options.SMTPFrom, TLSMode: options.SMTPTLSMode,
	})
	if err != nil {
		return nil, fmt.Errorf("configure SMTP: %w", err)
	}
	return sender, nil
}

func configuredGoogleSearchSubmitter(options hostconfig.Options) (searchnotify.GoogleSubmitter, error) {
	if strings.TrimSpace(options.GoogleSearchClientID) == "" {
		return nil, nil
	}
	clientSecret, err := readPrivateCredential(options.GoogleSearchClientSecretFile, "Google OAuth client secret")
	if err != nil {
		return nil, err
	}
	refreshToken, err := readPrivateCredential(options.GoogleSearchRefreshTokenFile, "Google OAuth refresh token")
	if err != nil {
		return nil, err
	}
	client, err := searchnotify.NewGoogleSearchConsoleClient(searchnotify.GoogleOAuthConfig{
		ClientID: options.GoogleSearchClientID, ClientSecret: clientSecret, RefreshToken: refreshToken,
	})
	if err != nil {
		return nil, fmt.Errorf("configure Google Search Console: %w", err)
	}
	return client, nil
}

func readPrivateCredential(path, label string) (string, error) {
	info, err := os.Stat(path)
	if err != nil {
		return "", fmt.Errorf("read %s file metadata: %w", label, err)
	}
	if !info.Mode().IsRegular() || info.Size() < 1 || info.Size() > 64<<10 {
		return "", fmt.Errorf("%s file must be a non-empty regular file no larger than 64 KiB", label)
	}
	body, err := os.ReadFile(path)
	if err != nil {
		return "", fmt.Errorf("read %s file: %w", label, err)
	}
	value := strings.TrimSpace(string(body))
	if value == "" {
		return "", fmt.Errorf("%s file is empty", label)
	}
	return value, nil
}

func externalBackupRequirements(options hostconfig.Options) []string {
	if options.SMTPHost != "" && options.SMTPUsername != "" && options.SMTPPasswordFile == "" {
		return []string{"environment:" + hostconfig.EnvSMTPPassword}
	}
	return nil
}

func runOfflineRestore(ctx context.Context, dbPath, dataDir, backupDir, configPath, selected string, allowWithoutPrebackup bool,
	resourceGate *platform.ResourceGate, externalRequirements []string) error {
	selected = strings.TrimSpace(selected)
	if !filepath.IsAbs(selected) {
		selected = filepath.Join(backupDir, selected)
	}
	manifest, err := recovery.LoadManifest(selected)
	if err != nil {
		return fmt.Errorf("load selected restore backup: %w", err)
	}
	if !sqlite.SupportedSchemaVersion(manifest.SchemaVersion) {
		return fmt.Errorf("selected backup schema version %d is not supported by this binary", manifest.SchemaVersion)
	}
	assetDir := filepath.Join(dataDir, "assets")
	if err := os.MkdirAll(assetDir, 0o700); err != nil {
		return err
	}
	inspection := sqlite.Inspect(dbPath)
	if inspection.State == sqlite.DatabaseReady {
		store, err := sqlite.OpenReady(dbPath)
		if err != nil {
			return fmt.Errorf("open current database for pre-restore backup: %w", err)
		}
		roots := existingBackupRoots(dataDir)
		_, backupErr := createBackup(ctx, store, dbPath, resourceGate, recovery.BackupConfig{
			BackupDir: backupDir, AssetDir: assetDir, ApplicationVersion: applicationVersion, Kind: "pre-restore", Roots: roots,
			ExternalRequirements: externalRequirements,
			Files:                existingBackupFiles(configPath),
		})
		closeErr := store.Close()
		if backupErr != nil {
			if !allowWithoutPrebackup {
				return fmt.Errorf("pre-restore backup failed; use the explicit host-recovery override only after preserving what is possible: %w", backupErr)
			}
			slog.Warn("continuing restore without a complete pre-restore backup", "error", backupErr)
		}
		if closeErr != nil {
			return closeErr
		}
	} else if inspection.State != sqlite.DatabaseFresh && !allowWithoutPrebackup {
		return fmt.Errorf("current database is %s; preserve diagnostics and pass -allow-restore-without-prebackup only for an explicit host-authorized recovery", inspection.State)
	}

	targets := map[string]recovery.RestoreTarget{
		"database": {Path: dbPath, Kind: "file"},
		"assets":   {Path: assetDir, Kind: "directory"},
	}
	for name := range manifest.Roots {
		if name == "" || filepath.Base(name) != name || strings.ContainsAny(name, `/\\`) {
			return fmt.Errorf("unsafe restore root name %q", name)
		}
		targets[name] = recovery.RestoreTarget{Path: filepath.Join(dataDir, name), Kind: "directory"}
	}
	for name := range manifest.Files {
		if name != "host-config" {
			return fmt.Errorf("unsupported restore file root %q", name)
		}
		if strings.TrimSpace(configPath) == "" {
			return errors.New("selected backup includes host-config but no host config path is available")
		}
		targets[name] = recovery.RestoreTarget{Path: configPath, Kind: "file"}
	}
	journalPath, err := recovery.PrepareRestore(ctx, recovery.RestoreConfig{
		BackupPath: selected, ControlDir: filepath.Join(dataDir, "control"), Targets: targets,
		ResourceGate: resourceGate, ByteHeadroom: 64 << 20, InodeHeadroom: 128, MinFreePercent: 1,
	})
	if err != nil {
		return fmt.Errorf("prepare restore: %w", err)
	}
	if err := recovery.ResumeRestore(ctx, journalPath); err != nil {
		return fmt.Errorf("restore prepared; recovery must roll forward using %s: %w", journalPath, err)
	}
	completed, err := recovery.LoadRestoreJournal(journalPath)
	if err != nil {
		return fmt.Errorf("restore completed but journal verification failed: %w", err)
	}
	if err := recovery.SupersedeMigrationJournals(filepath.Join(dataDir, "control"), completed.OperationID); err != nil {
		return fmt.Errorf("restore completed but prior migration journal cleanup failed: %w", err)
	}
	return nil
}

func migrationSteps(plan []sqlite.MigrationInfo) []recovery.MigrationStep {
	steps := make([]recovery.MigrationStep, 0, len(plan))
	for _, item := range plan {
		steps = append(steps, recovery.MigrationStep{
			Version: item.Version, Name: item.Name, Checksum: item.Checksum, Transactional: item.Transactional,
		})
	}
	return steps
}

func resumeMigration(ctx context.Context, dbPath, journalPath string) error {
	journal, err := recovery.LoadMigrationJournal(journalPath)
	if err != nil {
		return fmt.Errorf("recovery required: read migration journal: %w", err)
	}
	if journal.Phase == recovery.MigrationFailed {
		return fmt.Errorf("recovery required: migration %s previously failed: %s", journal.OperationID, journal.Failure)
	}
	if journal.Phase != recovery.MigrationPrepared {
		return fmt.Errorf("recovery required: migration %s has unsupported phase %q", journal.OperationID, journal.Phase)
	}
	if journal.ToVersion != sqlite.CurrentSchemaVersion {
		return fmt.Errorf("recovery required: migration %s targets schema %d but this binary expects %d", journal.OperationID, journal.ToVersion, sqlite.CurrentSchemaVersion)
	}
	if err := recovery.ValidateMigrationJournalSource(journal); err != nil {
		return fmt.Errorf("recovery required: migration backup validation failed: %w", err)
	}
	plan, err := sqlite.PendingMigrations(journal.FromVersion)
	if err != nil {
		return fmt.Errorf("recovery required: migration plan unavailable: %w", err)
	}
	expected := migrationSteps(plan)
	if len(expected) != len(journal.Steps) {
		return fmt.Errorf("recovery required: migration plan changed after preparation")
	}
	for index := range expected {
		if expected[index] != journal.Steps[index] {
			return fmt.Errorf("recovery required: migration step %d changed after preparation", expected[index].Version)
		}
	}

	inspection := sqlite.Inspect(dbPath)
	if inspection.State == sqlite.DatabaseReady && inspection.SchemaVersion == journal.ToVersion {
		return recovery.CompleteMigrationJournal(journalPath, inspection.SchemaVersion)
	}
	if inspection.State != sqlite.DatabaseUpgrade || inspection.SchemaVersion != journal.FromVersion {
		return fmt.Errorf("recovery required: migration journal expects schema %d or %d; database is state=%s schema=%d", journal.FromVersion, journal.ToVersion, inspection.State, inspection.SchemaVersion)
	}
	store, err := sqlite.OpenForUpgrade(dbPath)
	if err != nil {
		return fmt.Errorf("recovery required: reopen migration database: %w", err)
	}
	if err := store.Upgrade(ctx, journal.BackupID); err != nil {
		_ = store.Close()
		if journalErr := recovery.MarkMigrationFailed(journalPath, err); journalErr != nil {
			return fmt.Errorf("recovery required: resume migration failed: %v; record failure: %w", err, journalErr)
		}
		return fmt.Errorf("recovery required: resume migration failed: %w", err)
	}
	if err := store.Close(); err != nil {
		return fmt.Errorf("migration committed; restart required after database close failed: %w", err)
	}
	return recovery.CompleteMigrationJournal(journalPath, journal.ToVersion)
}

func createBackup(ctx context.Context, store *sqlite.Store, databasePath string, gate *platform.ResourceGate, config recovery.BackupConfig) (recovery.Manifest, error) {
	requiredBytes, requiredInodes, err := recovery.EstimateBackupResourcesWithFiles(ctx, databasePath, config.AssetDir, config.Roots, config.Files)
	if err != nil {
		return recovery.Manifest{}, fmt.Errorf("estimate backup resources: %w", err)
	}
	config.ResourceGate = gate
	config.RequiredBytes = requiredBytes
	config.RequiredInodes = requiredInodes
	config.ByteHeadroom = 64 << 20
	config.InodeHeadroom = 128
	config.MinFreePercent = 1
	config.ApplyReadOnly = true
	return recovery.CreateBackup(ctx, store, config)
}

func existingBackupRoots(dataDir string) map[string]string {
	roots := make(map[string]string)
	for _, name := range []string{"secrets", "config", "public-state"} {
		path := filepath.Join(dataDir, name)
		if info, err := os.Stat(path); err == nil && info.IsDir() {
			roots[name] = path
		}
	}
	return roots
}

func existingBackupFiles(configPath string) map[string]string {
	files := make(map[string]string)
	if info, err := os.Stat(configPath); err == nil && info.Mode().IsRegular() {
		files["host-config"] = configPath
	}
	return files
}

func runInstaller(ctx context.Context, store *sqlite.Store, listener net.Listener, options hostconfig.Options, databasePath string, hostConsole *consoleui.Session, stop <-chan struct{}) error {
	completed := make(chan struct{}, 1)
	app, bootstrapToken, err := webapp.NewInstaller(store, webapp.InstallerConfig{
		DataDir: options.DataDir, BackupDir: options.BackupDir, DefaultTimeZone: time.Local.String(),
		ApplicationVersion: applicationVersion,
		OnComplete: func() {
			select {
			case completed <- struct{}{}:
			default:
			}
		},
	})
	if err != nil {
		return err
	}
	listen := listener.Addr().String()
	installerURL := localInstallerURL(listen, bootstrapToken)
	if !hostConsole.Interactive() {
		fmt.Printf("Prods installation requires ownership claim. Open this one-time URL (console only):\n%s\n", installerURL)
	}
	guide := runtimeConsoleGuide(options, "等待安裝", "請在瀏覽器開啟以下網址以進行安裝。", installerURL, listen, databasePath)
	if err := hostConsole.Start(ctx, guide); err != nil {
		return fmt.Errorf("start console UI: %w", err)
	}
	slog.Info("Prods listening", "address", listen, "mode", "installer", "data_dir", options.DataDir, "backup_dir", options.BackupDir)
	return serveListenerWithStop(listener, app, completed, 10*time.Second, consoleStop(stop, hostConsole))
}

func runtimeConsoleGuide(options hostconfig.Options, status, action, actionURL, listen, databasePath string) consoleui.Guide {
	return consoleui.Guide{
		Status: status, Action: action, URL: actionURL,
		Version: applicationVersion, Host: consoleui.HostLabel(), Listen: listen, BaseURL: options.BaseURL,
		AdminURL:   strings.TrimRight(options.BaseURL, "/") + "/admin",
		ConfigPath: options.ConfigPath, DataDir: options.DataDir, BackupDir: options.BackupDir, DatabasePath: databasePath,
	}
}

func consoleStop(serviceStop <-chan struct{}, hostConsole *consoleui.Session) <-chan struct{} {
	if hostConsole == nil || !hostConsole.Interactive() {
		return serviceStop
	}
	consoleInterrupt := hostConsole.Interrupt()
	if serviceStop == nil {
		return consoleInterrupt
	}
	combined := make(chan struct{})
	go func() {
		select {
		case <-serviceStop:
		case <-consoleInterrupt:
		}
		close(combined)
	}()
	return combined
}

func prepareInstallerListener(options *hostconfig.Options) (net.Listener, error) {
	listener, err := net.Listen("tcp", options.Listen)
	if err == nil {
		if writeErr := persistInitialHostConfig(options); writeErr != nil {
			_ = listener.Close()
			return nil, writeErr
		}
		return listener, nil
	}
	if options.ListenSource != "default" || options.ConfigLoaded {
		return nil, fmt.Errorf("listen on %s: %w; update the effective listen setting or stop the conflicting process", options.Listen, err)
	}
	host, portText, splitErr := net.SplitHostPort(options.Listen)
	if splitErr != nil {
		return nil, fmt.Errorf("listen on %s: %w", options.Listen, err)
	}
	port, parseErr := strconv.Atoi(portText)
	if parseErr != nil || port < 1 || port > 65535 {
		return nil, fmt.Errorf("listen on %s: %w", options.Listen, err)
	}
	lastPort := port + 19
	if lastPort > 65535 {
		lastPort = 65535
	}
	for candidate := port + 1; candidate <= lastPort; candidate++ {
		address := net.JoinHostPort(host, strconv.Itoa(candidate))
		listener, candidateErr := net.Listen("tcp", address)
		if candidateErr != nil {
			continue
		}
		options.Listen = address
		if options.BaseURLSource == "default" {
			options.BaseURL = "http://127.0.0.1:" + strconv.Itoa(candidate)
		}
		if writeErr := persistInitialHostConfig(options); writeErr != nil {
			_ = listener.Close()
			return nil, writeErr
		}
		return listener, nil
	}
	return nil, fmt.Errorf("listen on %s failed and no free installer port was found through %d: %w", options.Listen, lastPort, err)
}

func persistInitialHostConfig(options *hostconfig.Options) error {
	if options.ConfigLoaded {
		return nil
	}
	if err := hostconfig.WriteNew(*options); err != nil {
		return fmt.Errorf("persist initial host configuration %s: %w; create a writable config explicitly and restart", options.ConfigPath, err)
	}
	options.ConfigLoaded = true
	return nil
}

func localInstallerURL(listen, token string) string {
	host, port, err := net.SplitHostPort(listen)
	if err != nil {
		host, port = "127.0.0.1", "8080"
	}
	if host == "" || host == "0.0.0.0" || host == "::" {
		host = "127.0.0.1"
	}
	location := &url.URL{Scheme: "http", Host: net.JoinHostPort(host, port), Path: "/install"}
	query := location.Query()
	query.Set("token", token)
	location.RawQuery = query.Encode()
	return location.String()
}

func issueOwnerRecovery(ctx context.Context, store *sqlite.Store, baseURL, email string) (string, time.Time, error) {
	if _, err := validateBaseURL(baseURL); err != nil {
		return "", time.Time{}, err
	}
	grant, err := store.CreateOwnerRecoveryGrant(ctx, email, 30*time.Minute)
	if err != nil {
		return "", time.Time{}, fmt.Errorf("create Owner recovery grant: %w", err)
	}
	location, err := url.Parse(baseURL)
	if err != nil {
		return "", time.Time{}, fmt.Errorf("parse Owner recovery base URL: %w", err)
	}
	location.Path = "/set-password"
	location.RawPath = ""
	query := location.Query()
	query.Set("token", grant.Token)
	location.RawQuery = query.Encode()
	return location.String(), grant.ExpiresAt, nil
}

func validateBaseURL(raw string) (bool, error) {
	location, err := url.Parse(raw)
	if err != nil || location.Host == "" || (location.Scheme != "http" && location.Scheme != "https") ||
		location.User != nil || location.RawQuery != "" || location.Fragment != "" {
		return false, fmt.Errorf("invalid canonical public base URL %q", raw)
	}
	return location.Scheme == "https", nil
}

func serveListener(listener net.Listener, handler http.Handler, completed <-chan struct{}, shutdownTimeout time.Duration) error {
	return serveListenerWithStop(listener, handler, completed, shutdownTimeout, nil)
}

func serveListenerWithStop(listener net.Listener, handler http.Handler, completed <-chan struct{}, shutdownTimeout time.Duration, serviceStop <-chan struct{}) error {
	server := &http.Server{
		Addr: listener.Addr().String(), Handler: handler, ReadHeaderTimeout: 5 * time.Second,
		IdleTimeout: 30 * time.Second,
	}
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	errCh := make(chan error, 1)
	go func() {
		errCh <- server.Serve(listener)
	}()
	if completed == nil {
		select {
		case <-ctx.Done():
		case <-serviceStop:
		case err := <-errCh:
			if !errors.Is(err, http.ErrServerClosed) {
				return err
			}
			return nil
		}
	} else {
		select {
		case <-ctx.Done():
		case <-serviceStop:
		case <-completed:
		case err := <-errCh:
			if !errors.Is(err, http.ErrServerClosed) {
				return err
			}
			return nil
		}
	}
	if drainer, ok := handler.(interface{ BeginDrain() }); ok {
		drainer.BeginDrain()
	}
	shutdownCtx, cancel := context.WithTimeout(context.Background(), shutdownTimeout)
	defer cancel()
	shutdownErr := server.Shutdown(shutdownCtx)
	if shutdownErr != nil {
		closeErr := server.Close()
		serveErr := <-errCh
		result := fmt.Errorf("graceful shutdown exceeded %s: %w", shutdownTimeout, shutdownErr)
		if closeErr != nil {
			result = errors.Join(result, fmt.Errorf("force close HTTP server: %w", closeErr))
		}
		if serveErr != nil && !errors.Is(serveErr, http.ErrServerClosed) {
			result = errors.Join(result, fmt.Errorf("HTTP server after force close: %w", serveErr))
		}
		return result
	}
	err := <-errCh
	if errors.Is(err, http.ErrServerClosed) {
		return nil
	}
	return err
}
