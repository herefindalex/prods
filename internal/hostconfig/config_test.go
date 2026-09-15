package hostconfig

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestDefaultsResolveBesideConfirmedExecutableNotWorkingDirectory(t *testing.T) {
	root := t.TempDir()
	realDir := filepath.Join(root, "release")
	linkDir := filepath.Join(root, "bin")
	workingDir := filepath.Join(root, "elsewhere")
	for _, dir := range []string{realDir, linkDir, workingDir} {
		if err := os.MkdirAll(dir, 0o700); err != nil {
			t.Fatal(err)
		}
	}
	realExecutable := filepath.Join(realDir, "prods")
	if err := os.WriteFile(realExecutable, []byte("binary"), 0o700); err != nil {
		t.Fatal(err)
	}
	link := filepath.Join(linkDir, "prods")
	if err := os.Symlink(realExecutable, link); err != nil {
		t.Fatal(err)
	}
	options, err := (Resolver{
		Executable: link,
		Getwd:      func() (string, error) { return workingDir, nil },
		LookupEnv:  emptyEnv,
		FlagOutput: &bytes.Buffer{},
	}).Resolve()
	if err != nil {
		t.Fatal(err)
	}
	if options.DataDir != filepath.Join(realDir, "data") || options.BackupDir != filepath.Join(realDir, "backups") ||
		options.ConfigPath != filepath.Join(realDir, "prods.ini") || options.ConfigLoaded || options.ListenSource != "default" {
		t.Fatalf("defaults were not resolved from confirmed executable: %+v", options)
	}
}

func TestCLIOverridesEnvironmentWhichOverridesConfigAndConfigPathsUseConfigDirectory(t *testing.T) {
	root := t.TempDir()
	configDir := filepath.Join(root, "configuration")
	workingDir := filepath.Join(root, "working")
	if err := os.MkdirAll(configDir, 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(workingDir, 0o700); err != nil {
		t.Fatal(err)
	}
	configPath := filepath.Join(configDir, "site.ini")
	if err := os.WriteFile(configPath, []byte("\uFEFF[prods]\r\nlisten = :7000\r\ndata_dir = state\r\nbackup_dir = saved\r\nbase_url = https://config.example.test\r\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	environment := map[string]string{
		EnvListen:    ":7100",
		EnvBackupDir: "env-backups",
		EnvBaseURL:   "https://env.example.test",
	}
	options, err := (Resolver{
		Executable: filepath.Join(root, "prods"),
		Args: []string{
			"--config", configPath,
			"--listen", ":7200",
			"--base-url", "https://cli.example.test",
			"--data-dir", "cli-data",
		},
		LookupEnv:  func(name string) (string, bool) { value, ok := environment[name]; return value, ok },
		Getwd:      func() (string, error) { return workingDir, nil },
		FlagOutput: &bytes.Buffer{},
	}).Resolve()
	if err != nil {
		t.Fatal(err)
	}
	if options.Listen != ":7200" || options.BaseURL != "https://cli.example.test" ||
		options.DataDir != filepath.Join(workingDir, "cli-data") ||
		options.BackupDir != filepath.Join(workingDir, "env-backups") ||
		options.ListenSource != "cli" || options.BaseURLSource != "cli" || !options.ConfigLoaded {
		t.Fatalf("resolved precedence=%+v", options)
	}
	configOnly, err := (Resolver{
		Executable: filepath.Join(root, "prods"),
		Args:       []string{"--config", configPath},
		LookupEnv:  emptyEnv,
		Getwd:      func() (string, error) { return workingDir, nil },
		FlagOutput: &bytes.Buffer{},
	}).Resolve()
	if err != nil {
		t.Fatal(err)
	}
	if configOnly.DataDir != filepath.Join(configDir, "state") || configOnly.BackupDir != filepath.Join(configDir, "saved") {
		t.Fatalf("config-relative paths=%+v", configOnly)
	}
}

func TestSMTPUsesSecureConfigAndSecretPrecedenceWithoutPersistingPassword(t *testing.T) {
	root := t.TempDir()
	configDir := filepath.Join(root, "configuration")
	if err := os.MkdirAll(filepath.Join(configDir, "data", "secrets"), 0o700); err != nil {
		t.Fatal(err)
	}
	configPath := filepath.Join(configDir, "prods.ini")
	config := `[prods]
listen=:8080
data_dir=data
backup_dir=backups
base_url=https://catalog.example.test
smtp_host=config.smtp.example.test
smtp_port=465
smtp_username=config-user
smtp_password_file=data/secrets/smtp-password
smtp_from=rfq@example.test
smtp_tls_mode=tls
`
	if err := os.WriteFile(configPath, []byte(config), 0o600); err != nil {
		t.Fatal(err)
	}
	environment := map[string]string{
		EnvSMTPHost: "env.smtp.example.test", EnvSMTPPort: "587", EnvSMTPPassword: "env-secret",
		EnvSMTPTLSMode: "starttls",
	}
	options, err := (Resolver{
		Executable: filepath.Join(root, "prods"),
		Args:       []string{"--config", configPath, "--smtp-host", "cli.smtp.example.test", "--smtp-port", "2465"},
		LookupEnv:  func(name string) (string, bool) { value, ok := environment[name]; return value, ok },
		Getwd:      func() (string, error) { return root, nil },
		FlagOutput: &bytes.Buffer{},
	}).Resolve()
	if err != nil {
		t.Fatal(err)
	}
	if options.SMTPHost != "cli.smtp.example.test" || options.SMTPPort != 2465 || options.SMTPUsername != "config-user" ||
		options.SMTPPassword != "env-secret" || options.SMTPPasswordFile != filepath.Join(configDir, "data", "secrets", "smtp-password") ||
		options.SMTPFrom != "rfq@example.test" || options.SMTPTLSMode != "starttls" {
		t.Fatalf("resolved SMTP options = %+v", options)
	}

	newPath := filepath.Join(root, "written", "prods.ini")
	options.ConfigPath = newPath
	options.ConfigLoaded = false
	if err := WriteNew(options); err != nil {
		t.Fatal(err)
	}
	body, err := os.ReadFile(newPath)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(body), "env-secret") || !strings.Contains(string(body), "smtp_password_file=") ||
		!strings.Contains(string(body), "smtp_tls_mode=starttls") {
		t.Fatalf("written SMTP config leaked or omitted settings: %s", body)
	}
}

func TestSMTPRejectsIncompleteOrPlaintextConfigSecret(t *testing.T) {
	root := t.TempDir()
	for name, content := range map[string]string{
		"incomplete": `[prods]
listen=:8080
data_dir=data
backup_dir=backups
base_url=https://catalog.example.test
smtp_host=smtp.example.test
`,
		"plaintext": `[prods]
listen=:8080
data_dir=data
backup_dir=backups
base_url=https://catalog.example.test
smtp_host=smtp.example.test
smtp_from=rfq@example.test
smtp_password=must-not-be-accepted
`,
	} {
		path := filepath.Join(root, name+".ini")
		if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
			t.Fatal(err)
		}
		if _, err := (Resolver{Executable: filepath.Join(root, "prods"), Args: []string{"--config", path},
			LookupEnv: func(string) (string, bool) { return "", false }, Getwd: func() (string, error) { return root, nil },
			FlagOutput: &bytes.Buffer{}}).Resolve(); err == nil {
			t.Fatalf("%s SMTP config was accepted", name)
		}
	}
}

func TestGoogleSearchCredentialsUseConfigEnvironmentCLIAndStayInSecretsRoot(t *testing.T) {
	root := t.TempDir()
	configPath := filepath.Join(root, "prods.ini")
	dataDir := filepath.Join(root, "data")
	secretsDir := filepath.Join(dataDir, "secrets")
	if err := os.MkdirAll(secretsDir, 0o700); err != nil {
		t.Fatal(err)
	}
	configSecret := filepath.Join(secretsDir, "config-client-secret")
	configRefresh := filepath.Join(secretsDir, "config-refresh-token")
	cliSecret := filepath.Join(secretsDir, "cli-client-secret")
	cliRefresh := filepath.Join(secretsDir, "cli-refresh-token")
	for _, path := range []string{configSecret, configRefresh, cliSecret, cliRefresh} {
		if err := os.WriteFile(path, []byte("not-persisted\n"), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	config := "listen=:8080\ndata_dir=data\nbackup_dir=backups\nbase_url=https://catalog.example.test\n" +
		"google_search_client_id=config-client\ngoogle_search_client_secret_file=data/secrets/config-client-secret\n" +
		"google_search_refresh_token_file=data/secrets/config-refresh-token\n"
	if err := os.WriteFile(configPath, []byte(config), 0o600); err != nil {
		t.Fatal(err)
	}
	options, err := (Resolver{
		Executable: filepath.Join(root, "prods"),
		Args: []string{"--config", configPath, "--google-search-client-id", "cli-client",
			"--google-search-client-secret-file", cliSecret, "--google-search-refresh-token-file", cliRefresh},
		LookupEnv: func(name string) (string, bool) {
			if name == EnvGoogleSearchClientID {
				return "env-client", true
			}
			return "", false
		},
		Getwd: func() (string, error) { return root, nil },
	}).Resolve()
	if err != nil {
		t.Fatal(err)
	}
	if options.GoogleSearchClientID != "cli-client" || options.GoogleSearchClientSecretFile != cliSecret || options.GoogleSearchRefreshTokenFile != cliRefresh {
		t.Fatalf("Google Search precedence=%+v", options)
	}

	outside := filepath.Join(root, "outside-secret")
	_, err = (Resolver{
		Executable: filepath.Join(root, "prods"),
		Args:       []string{"--config", configPath, "--google-search-client-secret-file", outside},
		LookupEnv:  func(string) (string, bool) { return "", false },
		Getwd:      func() (string, error) { return root, nil },
	}).Resolve()
	if err == nil || !strings.Contains(err.Error(), "data_dir/secrets") {
		t.Fatalf("outside Google secret file error=%v", err)
	}
}

func TestWriteNewPersistsOnlyHostSettingsAndNeverOverwrites(t *testing.T) {
	root := t.TempDir()
	configPath := filepath.Join(root, "configuration", "prods.ini")
	options := Options{
		ConfigPath:  configPath,
		Listen:      "127.0.0.1:8088",
		DataDir:     filepath.Join(root, "data with spaces"),
		BackupDir:   filepath.Join(root, "backups"),
		BaseURL:     "http://127.0.0.1:8088",
		AdminToken:  "must-not-be-written",
		POCFixtures: true,
	}
	if err := WriteNew(options); err != nil {
		t.Fatal(err)
	}
	body, err := os.ReadFile(configPath)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(body), options.AdminToken) || strings.Contains(string(body), "poc") {
		t.Fatalf("config persisted secret or one-shot flags: %s", body)
	}
	if !strings.Contains(string(body), "data_dir="+options.DataDir+"\n") ||
		!strings.Contains(string(body), "backup_dir="+options.BackupDir+"\n") {
		t.Fatalf("external config paths did not remain absolute: %s", body)
	}
	resolved, err := (Resolver{
		Executable: filepath.Join(root, "prods"),
		Args:       []string{"--config", configPath},
		LookupEnv:  emptyEnv,
		FlagOutput: &bytes.Buffer{},
	}).Resolve()
	if err != nil {
		t.Fatal(err)
	}
	if resolved.Listen != options.Listen || resolved.DataDir != options.DataDir ||
		resolved.BackupDir != options.BackupDir || resolved.BaseURL != options.BaseURL {
		t.Fatalf("round-trip host config=%+v", resolved)
	}
	if err := WriteNew(options); err == nil || !strings.Contains(err.Error(), "file exists") {
		t.Fatalf("overwrite config error=%v", err)
	}
}

func TestWriteNewUsesRelativePathsForPortableInTreeData(t *testing.T) {
	root := t.TempDir()
	options := Options{
		ConfigPath: filepath.Join(root, "prods.ini"),
		Listen:     ":8080",
		DataDir:    filepath.Join(root, "data"),
		BackupDir:  filepath.Join(root, "backups"),
		BaseURL:    "http://127.0.0.1:8080",
	}
	if err := WriteNew(options); err != nil {
		t.Fatal(err)
	}
	body, err := os.ReadFile(options.ConfigPath)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(body), "data_dir=data\n") || !strings.Contains(string(body), "backup_dir=backups\n") {
		t.Fatalf("portable config paths were not relative: %s", body)
	}
}

func TestAssetGCGraceDaysUsesCLIEnvironmentConfigPrecedenceAndValidation(t *testing.T) {
	root := t.TempDir()
	configPath := filepath.Join(root, "prods.ini")
	config := "[prods]\nlisten=:8080\ndata_dir=data\nbackup_dir=backups\nbase_url=http://127.0.0.1:8080\nasset_gc_grace_days=10\n"
	if err := os.WriteFile(configPath, []byte(config), 0o600); err != nil {
		t.Fatal(err)
	}
	environment := map[string]string{EnvAssetGCGraceDays: "11"}
	options, err := (Resolver{
		Executable: filepath.Join(root, "prods"),
		Args:       []string{"--config", configPath, "--asset-gc-grace-days", "12"},
		LookupEnv: func(name string) (string, bool) {
			value, ok := environment[name]
			return value, ok
		},
		Getwd:      func() (string, error) { return root, nil },
		FlagOutput: &bytes.Buffer{},
	}).Resolve()
	if err != nil {
		t.Fatal(err)
	}
	if options.AssetGCGraceDays != 12 {
		t.Fatalf("asset GC grace days=%d", options.AssetGCGraceDays)
	}

	for _, value := range []string{"0", "3651", "not-a-number"} {
		_, err := (Resolver{
			Executable: filepath.Join(root, "prods"),
			Args:       []string{"--config", configPath},
			LookupEnv: func(name string) (string, bool) {
				if name == EnvAssetGCGraceDays {
					return value, true
				}
				return "", false
			},
			Getwd:      func() (string, error) { return root, nil },
			FlagOutput: &bytes.Buffer{},
		}).Resolve()
		if err == nil || !strings.Contains(err.Error(), "asset GC grace days") {
			t.Fatalf("invalid grace %q error=%v", value, err)
		}
	}
}

func TestTrustedProxyCIDRsUseExplicitPrecedenceCanonicalizationAndValidation(t *testing.T) {
	root := t.TempDir()
	configPath := filepath.Join(root, "prods.ini")
	config := "[prods]\nlisten=:8080\ndata_dir=data\nbackup_dir=backups\nbase_url=http://127.0.0.1:8080\ntrusted_proxies=10.0.0.0/8\n"
	if err := os.WriteFile(configPath, []byte(config), 0o600); err != nil {
		t.Fatal(err)
	}
	environment := map[string]string{EnvTrustedProxies: "192.0.2.1"}
	options, err := (Resolver{
		Executable: filepath.Join(root, "prods"),
		Args:       []string{"--config", configPath, "--trusted-proxies", "127.0.0.1,2001:db8::1"},
		LookupEnv: func(name string) (string, bool) {
			value, ok := environment[name]
			return value, ok
		},
		Getwd:      func() (string, error) { return root, nil },
		FlagOutput: &bytes.Buffer{},
	}).Resolve()
	if err != nil {
		t.Fatal(err)
	}
	want := []string{"127.0.0.1/32", "2001:db8::1/128"}
	if len(options.TrustedProxyCIDRs) != len(want) || options.TrustedProxyCIDRs[0] != want[0] || options.TrustedProxyCIDRs[1] != want[1] {
		t.Fatalf("trusted proxies=%v want=%v", options.TrustedProxyCIDRs, want)
	}

	for _, value := range []string{"not-an-address", "10.0.0.0/99", "10.0.0.1,,10.0.0.2"} {
		_, err := (Resolver{
			Executable: filepath.Join(root, "prods"),
			Args:       []string{"--config", configPath, "--trusted-proxies", value},
			LookupEnv:  emptyEnv,
			Getwd:      func() (string, error) { return root, nil },
			FlagOutput: &bytes.Buffer{},
		}).Resolve()
		if err == nil || !strings.Contains(err.Error(), "trusted") {
			t.Fatalf("invalid trusted proxies %q error=%v", value, err)
		}
	}
}

func TestExplicitMissingOrInvalidConfigFailsWithoutFallback(t *testing.T) {
	root := t.TempDir()
	base := Resolver{Executable: filepath.Join(root, "prods"), LookupEnv: emptyEnv, FlagOutput: &bytes.Buffer{}}
	base.Args = []string{"--config", filepath.Join(root, "missing.ini")}
	if _, err := base.Resolve(); err == nil || !strings.Contains(err.Error(), "missing.ini") {
		t.Fatalf("missing explicit config error=%v", err)
	}
	invalid := filepath.Join(root, "invalid.ini")
	if err := os.WriteFile(invalid, []byte("data_dir=data\nunknown=value\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	base.Args = []string{"--config", invalid}
	if _, err := base.Resolve(); err == nil || !strings.Contains(err.Error(), "unknown setting") {
		t.Fatalf("invalid config error=%v", err)
	}
}

func TestVersionDoesNotDependOnHostConfigAvailability(t *testing.T) {
	options, err := (Resolver{
		Executable: filepath.Join(t.TempDir(), "prods"),
		Args:       []string{"--version", "--config", "/definitely/missing/prods.ini"},
		LookupEnv:  emptyEnv,
		FlagOutput: &bytes.Buffer{},
	}).Resolve()
	if err != nil {
		t.Fatal(err)
	}
	if !options.Version {
		t.Fatalf("version options=%+v", options)
	}
}

func TestServiceFlagsAreExplicitAndMutuallyExclusive(t *testing.T) {
	root := t.TempDir()
	resolver := Resolver{
		Executable: filepath.Join(root, "prods"),
		Args:       []string{"--install-service", "--service-user", "prods-user"},
		LookupEnv:  emptyEnv,
		FlagOutput: &bytes.Buffer{},
	}
	options, err := resolver.Resolve()
	if err != nil {
		t.Fatal(err)
	}
	if !options.InstallService || options.UninstallService || options.ServiceUser != "prods-user" {
		t.Fatalf("service options=%+v", options)
	}
	resolver.Args = []string{"--install-service", "--uninstall-service"}
	if _, err := resolver.Resolve(); err == nil || !strings.Contains(err.Error(), "mutually exclusive") {
		t.Fatalf("mutually exclusive service flags error=%v", err)
	}
	resolver.Args = []string{"--install-service", "--backup-now"}
	if _, err := resolver.Resolve(); err == nil || !strings.Contains(err.Error(), "cannot be combined") {
		t.Fatalf("service operation combination error=%v", err)
	}
}

func TestServiceRemovalDoesNotDependOnReadableSiteConfig(t *testing.T) {
	options, err := (Resolver{
		Executable: filepath.Join(t.TempDir(), "prods"),
		Args:       []string{"--uninstall-service", "--config", "/missing/site.ini"},
		LookupEnv:  emptyEnv,
		FlagOutput: &bytes.Buffer{},
	}).Resolve()
	if err != nil {
		t.Fatal(err)
	}
	if !options.UninstallService {
		t.Fatalf("uninstall options=%+v", options)
	}
}

func TestRecoverOwnerIsCLIOnlyAndRejectsOtherOneShotOperations(t *testing.T) {
	root := t.TempDir()
	configPath := filepath.Join(root, "prods.ini")
	config := "[prods]\nlisten=:8080\ndata_dir=data\nbackup_dir=backups\nbase_url=https://catalog.example.test\n"
	if err := os.WriteFile(configPath, []byte(config), 0o600); err != nil {
		t.Fatal(err)
	}
	resolver := Resolver{
		Executable: filepath.Join(root, "prods"),
		Args:       []string{"--config", configPath, "--recover-owner", "  OWNER@Example.Test "},
		LookupEnv:  emptyEnv,
		Getwd:      func() (string, error) { return root, nil },
		FlagOutput: &bytes.Buffer{},
	}
	options, err := resolver.Resolve()
	if err != nil {
		t.Fatal(err)
	}
	if options.RecoverOwner != "OWNER@Example.Test" {
		t.Fatalf("recovery Owner=%q", options.RecoverOwner)
	}

	writtenPath := filepath.Join(root, "written", "prods.ini")
	options.ConfigPath = writtenPath
	if err := WriteNew(options); err != nil {
		t.Fatal(err)
	}
	body, err := os.ReadFile(writtenPath)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(strings.ToLower(string(body)), "recover_owner=") || strings.Contains(string(body), options.RecoverOwner) {
		t.Fatalf("one-shot recovery target was persisted: %s", body)
	}

	for _, extra := range [][]string{
		{"--backup-now"},
		{"--restore-backup", "backup-1"},
		{"--allow-restore-without-prebackup"},
		{"--poc-fixtures"},
		{"--admin-token", "temporary"},
		{"--install-service"},
		{"--uninstall-service"},
		{"--version"},
	} {
		resolver.Args = append([]string{"--config", configPath, "--recover-owner", "owner@example.test"}, extra...)
		if _, err := resolver.Resolve(); err == nil || !strings.Contains(err.Error(), "recover-owner") {
			t.Fatalf("recovery combination %v error=%v", extra, err)
		}
	}
	resolver.Args = []string{"--config", configPath, "--recover-owner", "   "}
	if _, err := resolver.Resolve(); err == nil || !strings.Contains(err.Error(), "empty") {
		t.Fatalf("empty recovery target error=%v", err)
	}
}

func emptyEnv(string) (string, bool) { return "", false }
