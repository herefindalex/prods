package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"net"
	"net/url"
	"os"
	"path/filepath"
	"testing"
	"time"

	"prods/internal/hostconfig"
	"prods/internal/identity"
	"prods/internal/storage/sqlite"
)

func TestNormalTerminationIncludesHelpButNotInvalidArguments(t *testing.T) {
	for _, err := range []error{nil, flag.ErrHelp, fmt.Errorf("resolve options: %w", flag.ErrHelp)} {
		if !normalTermination(err) {
			t.Errorf("normalTermination(%v) = false", err)
		}
	}
	if normalTermination(errors.New("invalid arguments")) {
		t.Fatal("invalid arguments treated as normal termination")
	}
}

func TestBaseURLAndInstallerURL(t *testing.T) {
	if secure, err := validateBaseURL("https://catalog.example.test"); err != nil || !secure {
		t.Fatalf("HTTPS base URL: secure=%v err=%v", secure, err)
	}
	if secure, err := validateBaseURL("http://127.0.0.1:8080"); err != nil || secure {
		t.Fatalf("HTTP base URL: secure=%v err=%v", secure, err)
	}
	for _, invalid := range []string{"catalog.example.test", "ftp://catalog.example.test", "https://user@example.test", "https://example.test?q=1"} {
		if _, err := validateBaseURL(invalid); err == nil {
			t.Errorf("validateBaseURL(%q) succeeded", invalid)
		}
	}
	if got := localInstallerURL(":9090", "abc123"); got != "http://127.0.0.1:9090/install?token=abc123" {
		t.Fatalf("installer URL = %q", got)
	}
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer listener.Close()
	location, err := url.Parse(localInstallerURL(listener.Addr().String(), "abc123"))
	if err != nil {
		t.Fatal(err)
	}
	_, port, err := net.SplitHostPort(location.Host)
	if err != nil || port == "0" {
		t.Fatalf("installer URL did not use allocated listener address: %s", location)
	}
}

func TestConfiguredMailSenderLoadsPrivateFileAndRejectsInvalidSecret(t *testing.T) {
	passwordPath := filepath.Join(t.TempDir(), "smtp-password")
	if err := os.WriteFile(passwordPath, []byte("secret-value\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	options := hostconfig.Options{
		SMTPHost: "smtp.example.test", SMTPPort: 587, SMTPUsername: "user",
		SMTPPasswordFile: passwordPath, SMTPFrom: "rfq@example.test", SMTPTLSMode: "starttls",
	}
	sender, err := configuredMailSender(options)
	if err != nil || sender == nil {
		t.Fatalf("configured sender = %T err=%v", sender, err)
	}
	options.SMTPPasswordFile = filepath.Join(t.TempDir(), "missing")
	if _, err := configuredMailSender(options); err == nil {
		t.Fatal("missing SMTP password file was accepted")
	}
	options.SMTPPassword = "environment-secret"
	if sender, err := configuredMailSender(options); err != nil || sender == nil {
		t.Fatalf("environment password did not override missing file: sender=%T err=%v", sender, err)
	}
	if sender, err := configuredMailSender(hostconfig.Options{}); err != nil || sender != nil {
		t.Fatalf("disabled SMTP = %T err=%v", sender, err)
	}
}

func TestConfiguredGoogleSearchSubmitterLoadsPrivateFiles(t *testing.T) {
	root := t.TempDir()
	clientSecret := filepath.Join(root, "client-secret")
	refreshToken := filepath.Join(root, "refresh-token")
	if err := os.WriteFile(clientSecret, []byte("client-value\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(refreshToken, []byte("refresh-value\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	submitter, err := configuredGoogleSearchSubmitter(hostconfig.Options{
		GoogleSearchClientID: "client-id", GoogleSearchClientSecretFile: clientSecret,
		GoogleSearchRefreshTokenFile: refreshToken,
	})
	if err != nil || submitter == nil {
		t.Fatalf("configured Google Search submitter=%T err=%v", submitter, err)
	}
	if submitter, err := configuredGoogleSearchSubmitter(hostconfig.Options{}); err != nil || submitter != nil {
		t.Fatalf("disabled Google Search submitter=%T err=%v", submitter, err)
	}
	if _, err := configuredGoogleSearchSubmitter(hostconfig.Options{
		GoogleSearchClientID: "client-id", GoogleSearchClientSecretFile: filepath.Join(root, "missing"),
		GoogleSearchRefreshTokenFile: refreshToken,
	}); err == nil {
		t.Fatal("missing Google OAuth secret file was accepted")
	}
}

func TestExternalSMTPPasswordRequirementIsDeclaredForBackups(t *testing.T) {
	options := hostconfig.Options{SMTPHost: "smtp.example.test", SMTPUsername: "user", SMTPPassword: "external"}
	requirements := externalBackupRequirements(options)
	if len(requirements) != 1 || requirements[0] != "environment:"+hostconfig.EnvSMTPPassword {
		t.Fatalf("external requirements = %#v", requirements)
	}
	options.SMTPPasswordFile = filepath.Join(t.TempDir(), "secrets", "smtp-password")
	if requirements := externalBackupRequirements(options); len(requirements) != 0 {
		t.Fatalf("local secret file was declared external = %#v", requirements)
	}
}

func TestRuntimeLogsAreNotIncludedInBackupRoots(t *testing.T) {
	root := t.TempDir()
	for _, name := range []string{"logs", "secrets"} {
		if err := os.MkdirAll(filepath.Join(root, name), 0o700); err != nil {
			t.Fatal(err)
		}
	}
	roots := existingBackupRoots(root)
	if _, included := roots["logs"]; included {
		t.Fatal("runtime logs were included in the backup root set")
	}
	if roots["secrets"] != filepath.Join(root, "secrets") {
		t.Fatalf("required secrets root missing: %+v", roots)
	}
}

func TestIssueOwnerRecoveryCreatesUsableRootSetPasswordLink(t *testing.T) {
	store, err := sqlite.Create(filepath.Join(t.TempDir(), "prods.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	passwordHash, err := identity.HashPassword("ownerpass1")
	if err != nil {
		t.Fatal(err)
	}
	owner, err := store.CompleteInstallation(context.Background(), sqlite.Installation{
		OwnerEmail: "owner@example.test", OwnerDisplayName: "Owner", PasswordHash: passwordHash,
		DefaultLocale: "en-US", SupportedLocales: []string{"en-US"}, TimeZone: "UTC",
	})
	if err != nil {
		t.Fatal(err)
	}

	before := time.Now().UTC()
	rawLink, expiresAt, err := issueOwnerRecovery(context.Background(), store, "https://catalog.example.test/base", owner.Email)
	if err != nil {
		t.Fatal(err)
	}
	location, err := url.Parse(rawLink)
	if err != nil {
		t.Fatal(err)
	}
	token := location.Query().Get("token")
	if location.Scheme != "https" || location.Host != "catalog.example.test" || location.Path != "/set-password" ||
		token == "" || expiresAt.Before(before.Add(29*time.Minute)) || expiresAt.After(before.Add(31*time.Minute)) {
		t.Fatalf("recovery link=%q expiry=%s", rawLink, expiresAt)
	}
	newHash, err := identity.HashPassword("recoveredpass2")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := store.SetUserPassword(context.Background(), token, newHash, time.Now().UTC()); err != nil {
		t.Fatalf("set password from console recovery link: %v", err)
	}
	if _, _, err := issueOwnerRecovery(context.Background(), store, "not-a-url", owner.Email); err == nil {
		t.Fatal("invalid recovery base URL was accepted")
	}
}
