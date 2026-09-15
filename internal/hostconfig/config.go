package hostconfig

import (
	"bufio"
	"errors"
	"flag"
	"fmt"
	"io"
	"net/netip"
	"os"
	"path/filepath"
	"strconv"
	"strings"
)

const (
	EnvGoogleSearchClientID         = "PRODS_GOOGLE_SEARCH_CLIENT_ID"
	EnvGoogleSearchClientSecretFile = "PRODS_GOOGLE_SEARCH_CLIENT_SECRET_FILE"
	EnvGoogleSearchRefreshTokenFile = "PRODS_GOOGLE_SEARCH_REFRESH_TOKEN_FILE"
	EnvConfigPath                   = "PRODS_CONFIG"
	EnvListen                       = "PRODS_LISTEN"
	EnvDataDir                      = "PRODS_DATA_DIR"
	EnvBackupDir                    = "PRODS_BACKUP_DIR"
	EnvBaseURL                      = "PRODS_BASE_URL"
	EnvAdminToken                   = "PRODS_ADMIN_TOKEN"
	EnvSMTPHost                     = "PRODS_SMTP_HOST"
	EnvSMTPPort                     = "PRODS_SMTP_PORT"
	EnvSMTPUsername                 = "PRODS_SMTP_USERNAME"
	EnvSMTPPassword                 = "PRODS_SMTP_PASSWORD"
	EnvSMTPPasswordFile             = "PRODS_SMTP_PASSWORD_FILE"
	EnvSMTPFrom                     = "PRODS_SMTP_FROM"
	EnvSMTPTLSMode                  = "PRODS_SMTP_TLS_MODE"
	EnvAssetGCGraceDays             = "PRODS_ASSET_GC_GRACE_DAYS"
	EnvTrustedProxies               = "PRODS_TRUSTED_PROXIES"
)

type Options struct {
	GoogleSearchClientID         string
	GoogleSearchClientSecretFile string
	GoogleSearchRefreshTokenFile string
	ConfigPath                   string
	ConfigLoaded                 bool
	Listen                       string
	ListenSource                 string
	DataDir                      string
	BackupDir                    string
	BaseURL                      string
	BaseURLSource                string
	AdminToken                   string
	POCFixtures                  bool
	RestoreBackup                string
	AllowRestoreWithoutPrebackup bool
	BackupNow                    bool
	RecoverOwner                 string
	Version                      bool
	InstallService               bool
	UninstallService             bool
	ServiceUser                  string
	SMTPHost                     string
	SMTPPort                     int
	SMTPUsername                 string
	SMTPPassword                 string
	SMTPPasswordFile             string
	SMTPFrom                     string
	SMTPTLSMode                  string
	AssetGCGraceDays             int
	TrustedProxyCIDRs            []string
}

type Resolver struct {
	Executable string
	Args       []string
	LookupEnv  func(string) (string, bool)
	Getwd      func() (string, error)
	FlagOutput io.Writer
}

func (resolver Resolver) Resolve() (Options, error) {
	executable, err := confirmedExecutable(resolver.Executable)
	if err != nil {
		return Options{}, err
	}
	executableDir := filepath.Dir(executable)
	lookupEnv := resolver.LookupEnv
	if lookupEnv == nil {
		lookupEnv = os.LookupEnv
	}
	getwd := resolver.Getwd
	if getwd == nil {
		getwd = os.Getwd
	}
	workingDir, err := getwd()
	if err != nil {
		return Options{}, fmt.Errorf("resolve working directory: %w", err)
	}
	workingDir, err = filepath.Abs(workingDir)
	if err != nil {
		return Options{}, fmt.Errorf("resolve working directory: %w", err)
	}

	flags := flag.NewFlagSet("prods", flag.ContinueOnError)
	if resolver.FlagOutput != nil {
		flags.SetOutput(resolver.FlagOutput)
	} else {
		flags.SetOutput(os.Stderr)
	}
	configFlag := flags.String("config", "", "host configuration file (default: prods.ini beside the executable when present)")
	listenFlag := flags.String("listen", "", "HTTP listen address")
	dataDirFlag := flags.String("data-dir", "", "private data directory")
	backupDirFlag := flags.String("backup-dir", "", "backup directory")
	baseURLFlag := flags.String("base-url", "", "canonical public base URL")
	adminTokenFlag := flags.String("admin-token", "", "one-time Admin token for explicit PoC mode only")
	pocFixtures := flags.Bool("poc-fixtures", false, "explicitly create or reopen an isolated PoC database with synthetic fixtures")
	restoreBackup := flags.String("restore-backup", "", "offline restore from a backup ID or path; exits after verified roll-forward")
	allowRestoreWithoutPrebackup := flags.Bool("allow-restore-without-prebackup", false, "allow host-authorized recovery restore when current data cannot be backed up")
	backupNow := flags.Bool("backup-now", false, "create a verified backup and exit")
	recoverOwner := flags.String("recover-owner", "", "create a one-time password recovery link for an existing active Owner and exit")
	version := flags.Bool("version", false, "print the Prods and Go runtime versions and exit")
	installService := flags.Bool("install-service", false, "install and start the optional Prods system service")
	uninstallService := flags.Bool("uninstall-service", false, "stop and remove the optional Prods system service")
	serviceUser := flags.String("service-user", "", "Linux account used by the optional systemd service")
	smtpHostFlag := flags.String("smtp-host", "", "SMTP server host")
	smtpPortFlag := flags.Int("smtp-port", 0, "SMTP server port (default 587 when configured)")
	smtpUsernameFlag := flags.String("smtp-username", "", "SMTP authentication username")
	smtpPasswordFileFlag := flags.String("smtp-password-file", "", "file containing SMTP password; prefer a private file under data/secrets")
	smtpFromFlag := flags.String("smtp-from", "", "SMTP envelope and From address")
	smtpTLSModeFlag := flags.String("smtp-tls-mode", "", "secure SMTP mode: starttls or tls")
	googleSearchClientIDFlag := flags.String("google-search-client-id", "", "Google Search Console OAuth client ID")
	googleSearchClientSecretFileFlag := flags.String("google-search-client-secret-file", "", "private file containing the Google OAuth client secret")
	googleSearchRefreshTokenFileFlag := flags.String("google-search-refresh-token-file", "", "private file containing the Google OAuth refresh token")
	assetGCGraceDaysFlag := flags.Int("asset-gc-grace-days", 0, "days an unreferenced asset remains private before deletion (default 7)")
	trustedProxiesFlag := flags.String("trusted-proxies", "", "comma-separated proxy IPs or CIDRs allowed to supply forwarded client addresses")
	if err := flags.Parse(resolver.Args); err != nil {
		return Options{}, err
	}
	if flags.NArg() != 0 {
		return Options{}, fmt.Errorf("unexpected positional arguments: %s", strings.Join(flags.Args(), " "))
	}
	explicit := make(map[string]bool)
	flags.Visit(func(item *flag.Flag) { explicit[item.Name] = true })
	recoverOwnerEmail := strings.TrimSpace(*recoverOwner)
	if explicit["recover-owner"] && recoverOwnerEmail == "" {
		return Options{}, errors.New("--recover-owner email is empty")
	}
	if *version {
		if recoverOwnerEmail != "" {
			return Options{}, errors.New("--version and --recover-owner are mutually exclusive")
		}
		return Options{Version: true}, nil
	}
	if *installService && *uninstallService {
		return Options{}, errors.New("--install-service and --uninstall-service are mutually exclusive")
	}
	if *uninstallService {
		if recoverOwnerEmail != "" {
			return Options{}, errors.New("--uninstall-service and --recover-owner are mutually exclusive")
		}
		return Options{UninstallService: true}, nil
	}

	options := Options{
		Listen:           ":8080",
		ListenSource:     "default",
		DataDir:          filepath.Join(executableDir, "data"),
		BackupDir:        filepath.Join(executableDir, "backups"),
		BaseURL:          "http://127.0.0.1:8080",
		BaseURLSource:    "default",
		AssetGCGraceDays: 7,
	}
	configPath := strings.TrimSpace(*configFlag)
	configRequired := explicit["config"]
	if configRequired && configPath == "" {
		return Options{}, errors.New("--config path is empty")
	}
	if configPath == "" {
		if value, ok := lookupEnv(EnvConfigPath); ok && strings.TrimSpace(value) != "" {
			configPath = strings.TrimSpace(value)
			configRequired = true
		} else {
			configPath = filepath.Join(executableDir, "prods.ini")
		}
	}
	configPath, err = absoluteFrom(configPath, workingDir)
	if err != nil {
		return Options{}, fmt.Errorf("resolve config path: %w", err)
	}
	options.ConfigPath = configPath
	fileValues, err := readConfig(configPath)
	if err != nil {
		if !errors.Is(err, os.ErrNotExist) || configRequired {
			return Options{}, fmt.Errorf("read config %s: %w", configPath, err)
		}
	} else {
		options.ConfigLoaded = true
		configDir := filepath.Dir(configPath)
		if value := fileValues["listen"]; value != "" {
			options.Listen = value
			options.ListenSource = "config"
		}
		if value := fileValues["data_dir"]; value != "" {
			options.DataDir, err = absoluteFrom(value, configDir)
			if err != nil {
				return Options{}, fmt.Errorf("resolve config data_dir: %w", err)
			}
		}
		if value := fileValues["backup_dir"]; value != "" {
			options.BackupDir, err = absoluteFrom(value, configDir)
			if err != nil {
				return Options{}, fmt.Errorf("resolve config backup_dir: %w", err)
			}
		}
		if value := fileValues["base_url"]; value != "" {
			options.BaseURL = value
			options.BaseURLSource = "config"
		}
		options.SMTPHost = fileValues["smtp_host"]
		options.SMTPUsername = fileValues["smtp_username"]
		options.SMTPFrom = fileValues["smtp_from"]
		options.SMTPTLSMode = fileValues["smtp_tls_mode"]
		options.GoogleSearchClientID = fileValues["google_search_client_id"]
		if value := fileValues["asset_gc_grace_days"]; value != "" {
			options.AssetGCGraceDays, err = parseAssetGCGraceDays(value)
			if err != nil {
				return Options{}, fmt.Errorf("config asset_gc_grace_days: %w", err)
			}
		}
		if value := fileValues["trusted_proxies"]; value != "" {
			options.TrustedProxyCIDRs, err = parseTrustedProxies(value)
			if err != nil {
				return Options{}, fmt.Errorf("config trusted_proxies: %w", err)
			}
		}
		if value := fileValues["smtp_port"]; value != "" {
			options.SMTPPort, err = parsePort(value)
			if err != nil {
				return Options{}, fmt.Errorf("config smtp_port: %w", err)
			}
		}
		if value := fileValues["smtp_password_file"]; value != "" {
			options.SMTPPasswordFile, err = absoluteFrom(value, configDir)
			if err != nil {
				return Options{}, fmt.Errorf("resolve config smtp_password_file: %w", err)
			}
		}
		if value := fileValues["google_search_client_secret_file"]; value != "" {
			options.GoogleSearchClientSecretFile, err = absoluteFrom(value, configDir)
			if err != nil {
				return Options{}, fmt.Errorf("resolve config google_search_client_secret_file: %w", err)
			}
		}
		if value := fileValues["google_search_refresh_token_file"]; value != "" {
			options.GoogleSearchRefreshTokenFile, err = absoluteFrom(value, configDir)
			if err != nil {
				return Options{}, fmt.Errorf("resolve config google_search_refresh_token_file: %w", err)
			}
		}
	}

	if value, ok := nonEmptyEnv(lookupEnv, EnvListen); ok {
		options.Listen = value
		options.ListenSource = "environment"
	}
	if value, ok := nonEmptyEnv(lookupEnv, EnvDataDir); ok {
		options.DataDir, err = absoluteFrom(value, workingDir)
		if err != nil {
			return Options{}, fmt.Errorf("resolve %s: %w", EnvDataDir, err)
		}
	}
	if value, ok := nonEmptyEnv(lookupEnv, EnvBackupDir); ok {
		options.BackupDir, err = absoluteFrom(value, workingDir)
		if err != nil {
			return Options{}, fmt.Errorf("resolve %s: %w", EnvBackupDir, err)
		}
	}
	if value, ok := nonEmptyEnv(lookupEnv, EnvBaseURL); ok {
		options.BaseURL = value
		options.BaseURLSource = "environment"
	}
	if value, ok := nonEmptyEnv(lookupEnv, EnvAdminToken); ok {
		options.AdminToken = value
	}
	if value, ok := nonEmptyEnv(lookupEnv, EnvSMTPHost); ok {
		options.SMTPHost = value
	}
	if value, ok := nonEmptyEnv(lookupEnv, EnvSMTPPort); ok {
		options.SMTPPort, err = parsePort(value)
		if err != nil {
			return Options{}, fmt.Errorf("%s: %w", EnvSMTPPort, err)
		}
	}
	if value, ok := nonEmptyEnv(lookupEnv, EnvSMTPUsername); ok {
		options.SMTPUsername = value
	}
	if value, ok := nonEmptyEnv(lookupEnv, EnvSMTPPassword); ok {
		options.SMTPPassword = value
	}
	if value, ok := nonEmptyEnv(lookupEnv, EnvSMTPPasswordFile); ok {
		options.SMTPPasswordFile, err = absoluteFrom(value, workingDir)
		if err != nil {
			return Options{}, fmt.Errorf("resolve %s: %w", EnvSMTPPasswordFile, err)
		}
	}
	if value, ok := nonEmptyEnv(lookupEnv, EnvSMTPFrom); ok {
		options.SMTPFrom = value
	}
	if value, ok := nonEmptyEnv(lookupEnv, EnvSMTPTLSMode); ok {
		options.SMTPTLSMode = strings.ToLower(value)
	}
	if value, ok := nonEmptyEnv(lookupEnv, EnvGoogleSearchClientID); ok {
		options.GoogleSearchClientID = value
	}
	if value, ok := nonEmptyEnv(lookupEnv, EnvGoogleSearchClientSecretFile); ok {
		options.GoogleSearchClientSecretFile, err = absoluteFrom(value, workingDir)
		if err != nil {
			return Options{}, fmt.Errorf("resolve %s: %w", EnvGoogleSearchClientSecretFile, err)
		}
	}
	if value, ok := nonEmptyEnv(lookupEnv, EnvGoogleSearchRefreshTokenFile); ok {
		options.GoogleSearchRefreshTokenFile, err = absoluteFrom(value, workingDir)
		if err != nil {
			return Options{}, fmt.Errorf("resolve %s: %w", EnvGoogleSearchRefreshTokenFile, err)
		}
	}
	if value, ok := nonEmptyEnv(lookupEnv, EnvAssetGCGraceDays); ok {
		options.AssetGCGraceDays, err = parseAssetGCGraceDays(value)
		if err != nil {
			return Options{}, fmt.Errorf("%s: %w", EnvAssetGCGraceDays, err)
		}
	}
	if value, ok := nonEmptyEnv(lookupEnv, EnvTrustedProxies); ok {
		options.TrustedProxyCIDRs, err = parseTrustedProxies(value)
		if err != nil {
			return Options{}, fmt.Errorf("%s: %w", EnvTrustedProxies, err)
		}
	}

	if explicit["listen"] {
		options.Listen = strings.TrimSpace(*listenFlag)
		options.ListenSource = "cli"
	}
	if explicit["data-dir"] {
		options.DataDir, err = absoluteFrom(strings.TrimSpace(*dataDirFlag), workingDir)
		if err != nil {
			return Options{}, fmt.Errorf("resolve --data-dir: %w", err)
		}
	}
	if explicit["backup-dir"] {
		options.BackupDir, err = absoluteFrom(strings.TrimSpace(*backupDirFlag), workingDir)
		if err != nil {
			return Options{}, fmt.Errorf("resolve --backup-dir: %w", err)
		}
	}
	if explicit["base-url"] {
		options.BaseURL = strings.TrimSpace(*baseURLFlag)
		options.BaseURLSource = "cli"
	}
	if explicit["admin-token"] {
		options.AdminToken = strings.TrimSpace(*adminTokenFlag)
	}
	if explicit["smtp-host"] {
		options.SMTPHost = strings.TrimSpace(*smtpHostFlag)
	}
	if explicit["smtp-port"] {
		options.SMTPPort = *smtpPortFlag
	}
	if explicit["smtp-username"] {
		options.SMTPUsername = strings.TrimSpace(*smtpUsernameFlag)
	}
	if explicit["smtp-password-file"] {
		options.SMTPPasswordFile, err = absoluteFrom(strings.TrimSpace(*smtpPasswordFileFlag), workingDir)
		if err != nil {
			return Options{}, fmt.Errorf("resolve --smtp-password-file: %w", err)
		}
	}
	if explicit["smtp-from"] {
		options.SMTPFrom = strings.TrimSpace(*smtpFromFlag)
	}
	if explicit["smtp-tls-mode"] {
		options.SMTPTLSMode = strings.ToLower(strings.TrimSpace(*smtpTLSModeFlag))
	}
	if explicit["google-search-client-id"] {
		options.GoogleSearchClientID = strings.TrimSpace(*googleSearchClientIDFlag)
	}
	if explicit["google-search-client-secret-file"] {
		options.GoogleSearchClientSecretFile, err = absoluteFrom(strings.TrimSpace(*googleSearchClientSecretFileFlag), workingDir)
		if err != nil {
			return Options{}, fmt.Errorf("resolve --google-search-client-secret-file: %w", err)
		}
	}
	if explicit["google-search-refresh-token-file"] {
		options.GoogleSearchRefreshTokenFile, err = absoluteFrom(strings.TrimSpace(*googleSearchRefreshTokenFileFlag), workingDir)
		if err != nil {
			return Options{}, fmt.Errorf("resolve --google-search-refresh-token-file: %w", err)
		}
	}
	if explicit["asset-gc-grace-days"] {
		options.AssetGCGraceDays, err = parseAssetGCGraceDays(strconv.Itoa(*assetGCGraceDaysFlag))
		if err != nil {
			return Options{}, fmt.Errorf("--asset-gc-grace-days: %w", err)
		}
	}
	if explicit["trusted-proxies"] {
		options.TrustedProxyCIDRs, err = parseTrustedProxies(*trustedProxiesFlag)
		if err != nil {
			return Options{}, fmt.Errorf("--trusted-proxies: %w", err)
		}
	}
	if strings.TrimSpace(options.Listen) == "" || strings.TrimSpace(options.DataDir) == "" ||
		strings.TrimSpace(options.BackupDir) == "" || strings.TrimSpace(options.BaseURL) == "" {
		return Options{}, errors.New("listen, data_dir, backup_dir, and base_url must not be empty")
	}
	options.POCFixtures = *pocFixtures
	options.RestoreBackup = strings.TrimSpace(*restoreBackup)
	options.AllowRestoreWithoutPrebackup = *allowRestoreWithoutPrebackup
	options.BackupNow = *backupNow
	options.RecoverOwner = recoverOwnerEmail
	options.Version = *version
	options.InstallService = *installService
	options.UninstallService = *uninstallService
	options.ServiceUser = strings.TrimSpace(*serviceUser)
	if options.SMTPHost != "" {
		if options.SMTPPort == 0 {
			options.SMTPPort = 587
		}
		if options.SMTPTLSMode == "" {
			options.SMTPTLSMode = "starttls"
		}
		if options.SMTPFrom == "" || (options.SMTPTLSMode != "starttls" && options.SMTPTLSMode != "tls") ||
			(options.SMTPUsername == "") != (options.SMTPPassword == "" && options.SMTPPasswordFile == "") {
			return Options{}, errors.New("SMTP requires smtp_from, secure smtp_tls_mode, and matching username/password configuration")
		}
		if options.SMTPPasswordFile != "" && !pathWithin(filepath.Join(options.DataDir, "secrets"), options.SMTPPasswordFile) {
			return Options{}, errors.New("smtp_password_file must be inside data_dir/secrets so backup and restore include it")
		}
	} else if options.SMTPPort != 0 || options.SMTPUsername != "" || options.SMTPPassword != "" ||
		options.SMTPPasswordFile != "" || options.SMTPFrom != "" || options.SMTPTLSMode != "" {
		return Options{}, errors.New("smtp_host is required when any SMTP setting is configured")
	}
	googleSearchConfigured := options.GoogleSearchClientID != "" || options.GoogleSearchClientSecretFile != "" || options.GoogleSearchRefreshTokenFile != ""
	if googleSearchConfigured {
		if options.GoogleSearchClientID == "" || options.GoogleSearchClientSecretFile == "" || options.GoogleSearchRefreshTokenFile == "" {
			return Options{}, errors.New("Google Search Console requires client ID, client secret file, and refresh token file together")
		}
		secretsRoot := filepath.Join(options.DataDir, "secrets")
		if !pathWithin(secretsRoot, options.GoogleSearchClientSecretFile) || !pathWithin(secretsRoot, options.GoogleSearchRefreshTokenFile) {
			return Options{}, errors.New("Google Search Console secret files must be inside data_dir/secrets so backup and restore include them")
		}
	}
	if options.InstallService && (options.POCFixtures || options.RestoreBackup != "" ||
		options.AllowRestoreWithoutPrebackup || options.BackupNow || options.AdminToken != "" || options.RecoverOwner != "") {
		return Options{}, errors.New("--install-service cannot be combined with PoC, backup, restore, --recover-owner, or temporary Admin operations")
	}
	if options.RecoverOwner != "" && (options.POCFixtures || options.RestoreBackup != "" ||
		options.AllowRestoreWithoutPrebackup || options.BackupNow || options.AdminToken != "") {
		return Options{}, errors.New("--recover-owner cannot be combined with PoC, backup, restore, or temporary Admin operations")
	}
	if !options.InstallService && options.ServiceUser != "" {
		return Options{}, errors.New("--service-user requires --install-service")
	}
	return options, nil
}

func pathWithin(root, target string) bool {
	relative, err := filepath.Rel(filepath.Clean(root), filepath.Clean(target))
	return err == nil && relative != ".." && !strings.HasPrefix(relative, ".."+string(filepath.Separator))
}

// WriteNew creates the single host configuration selected by Resolve. It does
// not persist secrets or one-shot operational flags and never overwrites an
// existing file.
func WriteNew(options Options) error {
	if strings.TrimSpace(options.ConfigPath) == "" {
		return errors.New("config path is empty")
	}
	if options.AssetGCGraceDays == 0 {
		options.AssetGCGraceDays = 7
	}
	if _, err := parseAssetGCGraceDays(strconv.Itoa(options.AssetGCGraceDays)); err != nil {
		return err
	}
	values := []string{options.Listen, options.DataDir, options.BackupDir, options.BaseURL, options.SMTPHost,
		options.SMTPUsername, options.SMTPPasswordFile, options.SMTPFrom, options.SMTPTLSMode,
		options.GoogleSearchClientID, options.GoogleSearchClientSecretFile, options.GoogleSearchRefreshTokenFile,
		strings.Join(options.TrustedProxyCIDRs, ",")}
	for _, value := range values {
		if strings.ContainsAny(value, "\r\n") {
			return errors.New("host configuration values must be single-line")
		}
	}
	if err := os.MkdirAll(filepath.Dir(options.ConfigPath), 0o700); err != nil {
		return fmt.Errorf("create config directory %s: %w", filepath.Dir(options.ConfigPath), err)
	}
	file, err := os.OpenFile(options.ConfigPath, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0o600)
	if err != nil {
		return fmt.Errorf("create config %s: %w", options.ConfigPath, err)
	}
	complete := false
	defer func() {
		_ = file.Close()
		if !complete {
			_ = os.Remove(options.ConfigPath)
		}
	}()
	configDir := filepath.Dir(options.ConfigPath)
	content := fmt.Sprintf(
		"[prods]\nlisten=%s\ndata_dir=%s\nbackup_dir=%s\nbase_url=%s\nasset_gc_grace_days=%d\n",
		options.Listen,
		portableConfigPath(configDir, options.DataDir),
		portableConfigPath(configDir, options.BackupDir),
		options.BaseURL,
		options.AssetGCGraceDays,
	)
	if len(options.TrustedProxyCIDRs) != 0 {
		trusted, err := parseTrustedProxies(strings.Join(options.TrustedProxyCIDRs, ","))
		if err != nil {
			return err
		}
		content += "trusted_proxies=" + strings.Join(trusted, ",") + "\n"
	}
	if options.SMTPHost != "" {
		content += fmt.Sprintf("smtp_host=%s\nsmtp_port=%d\nsmtp_from=%s\nsmtp_tls_mode=%s\n",
			options.SMTPHost, options.SMTPPort, options.SMTPFrom, options.SMTPTLSMode)
		if options.SMTPUsername != "" {
			content += "smtp_username=" + options.SMTPUsername + "\n"
		}
		if options.SMTPPasswordFile != "" {
			content += "smtp_password_file=" + portableConfigPath(configDir, options.SMTPPasswordFile) + "\n"
		}
	}
	if options.GoogleSearchClientID != "" {
		content += "google_search_client_id=" + options.GoogleSearchClientID + "\n"
		content += "google_search_client_secret_file=" + portableConfigPath(configDir, options.GoogleSearchClientSecretFile) + "\n"
		content += "google_search_refresh_token_file=" + portableConfigPath(configDir, options.GoogleSearchRefreshTokenFile) + "\n"
	}
	if _, err := io.WriteString(file, content); err != nil {
		return fmt.Errorf("write config %s: %w", options.ConfigPath, err)
	}
	if err := file.Sync(); err != nil {
		return fmt.Errorf("sync config %s: %w", options.ConfigPath, err)
	}
	if err := file.Close(); err != nil {
		return fmt.Errorf("close config %s: %w", options.ConfigPath, err)
	}
	complete = true
	return nil
}

func portableConfigPath(configDir, target string) string {
	relative, err := filepath.Rel(configDir, target)
	if err != nil || relative == ".." || strings.HasPrefix(relative, ".."+string(filepath.Separator)) {
		return target
	}
	return relative
}

func confirmedExecutable(raw string) (string, error) {
	if strings.TrimSpace(raw) == "" {
		return "", errors.New("executable path is empty")
	}
	absolute, err := filepath.Abs(raw)
	if err != nil {
		return "", fmt.Errorf("resolve executable: %w", err)
	}
	confirmed, err := filepath.EvalSymlinks(absolute)
	if err == nil {
		return confirmed, nil
	}
	if errors.Is(err, os.ErrNotExist) {
		return filepath.Clean(absolute), nil
	}
	return "", fmt.Errorf("confirm executable path %s: %w", absolute, err)
}

func readConfig(path string) (map[string]string, error) {
	file, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer file.Close()
	values := make(map[string]string)
	scanner := bufio.NewScanner(file)
	lineNumber := 0
	section := "prods"
	for scanner.Scan() {
		lineNumber++
		line := strings.TrimSpace(scanner.Text())
		if lineNumber == 1 {
			line = strings.TrimPrefix(line, "\uFEFF")
		}
		if line == "" || strings.HasPrefix(line, "#") || strings.HasPrefix(line, ";") {
			continue
		}
		if strings.HasPrefix(line, "[") && strings.HasSuffix(line, "]") {
			section = strings.ToLower(strings.TrimSpace(strings.TrimSuffix(strings.TrimPrefix(line, "["), "]")))
			if section != "prods" {
				return nil, fmt.Errorf("line %d: unsupported section %q", lineNumber, section)
			}
			continue
		}
		if section != "prods" {
			return nil, fmt.Errorf("line %d: setting outside [prods]", lineNumber)
		}
		key, value, ok := strings.Cut(line, "=")
		if !ok {
			return nil, fmt.Errorf("line %d: expected key=value", lineNumber)
		}
		key = strings.ReplaceAll(strings.ToLower(strings.TrimSpace(key)), "-", "_")
		value = strings.TrimSpace(value)
		switch key {
		case "listen", "data_dir", "backup_dir", "base_url", "smtp_host", "smtp_port", "smtp_username",
			"smtp_password_file", "smtp_from", "smtp_tls_mode", "google_search_client_id",
			"google_search_client_secret_file", "google_search_refresh_token_file", "asset_gc_grace_days", "trusted_proxies":
		default:
			return nil, fmt.Errorf("line %d: unknown setting %q", lineNumber, key)
		}
		if _, exists := values[key]; exists {
			return nil, fmt.Errorf("line %d: duplicate setting %q", lineNumber, key)
		}
		if value == "" {
			return nil, fmt.Errorf("line %d: setting %q is empty", lineNumber, key)
		}
		values[key] = value
	}
	if err := scanner.Err(); err != nil {
		return nil, err
	}
	return values, nil
}

func parsePort(value string) (int, error) {
	port, err := strconv.Atoi(strings.TrimSpace(value))
	if err != nil || port < 1 || port > 65535 {
		return 0, fmt.Errorf("invalid port %q", value)
	}
	return port, nil
}

func parseAssetGCGraceDays(value string) (int, error) {
	days, err := strconv.Atoi(strings.TrimSpace(value))
	if err != nil || days < 1 || days > 3650 {
		return 0, fmt.Errorf("invalid asset GC grace days %q (want 1..3650)", value)
	}
	return days, nil
}

func parseTrustedProxies(value string) ([]string, error) {
	parts := strings.Split(value, ",")
	seen := make(map[string]struct{}, len(parts))
	result := make([]string, 0, len(parts))
	for _, raw := range parts {
		entry := strings.TrimSpace(raw)
		if entry == "" {
			return nil, errors.New("trusted proxy list contains an empty entry")
		}
		prefix, err := netip.ParsePrefix(entry)
		if err != nil {
			address, addressErr := netip.ParseAddr(entry)
			if addressErr != nil {
				return nil, fmt.Errorf("invalid trusted proxy %q", entry)
			}
			address = address.Unmap()
			prefix = netip.PrefixFrom(address, address.BitLen())
		} else {
			prefix = prefix.Masked()
		}
		canonical := prefix.String()
		if _, exists := seen[canonical]; exists {
			continue
		}
		seen[canonical] = struct{}{}
		result = append(result, canonical)
	}
	return result, nil
}

func nonEmptyEnv(lookup func(string) (string, bool), name string) (string, bool) {
	value, ok := lookup(name)
	value = strings.TrimSpace(value)
	return value, ok && value != ""
}

func absoluteFrom(value, base string) (string, error) {
	if strings.TrimSpace(value) == "" {
		return "", errors.New("path is empty")
	}
	if !filepath.IsAbs(value) {
		value = filepath.Join(base, value)
	}
	absolute, err := filepath.Abs(value)
	if err != nil {
		return "", err
	}
	return filepath.Clean(absolute), nil
}
