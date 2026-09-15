package webapp

import (
	"crypto/subtle"
	"encoding/json"
	"errors"
	"fmt"
	"html/template"
	"io/fs"
	"log/slog"
	"net/http"
	"net/mail"
	"net/url"
	"os"
	"strings"
	"sync"
	"time"

	"golang.org/x/text/language"

	"prods/internal/identity"
	"prods/internal/localization"
	"prods/internal/storage/sqlite"
)

type InstallerConfig struct {
	BootstrapToken  string
	DataDir         string
	BackupDir       string
	DefaultLocale   string
	DefaultTimeZone string
	OnComplete      func()
}

type InstallerServer struct {
	store          *sqlite.Store
	config         InstallerConfig
	templates      *template.Template
	mux            *http.ServeMux
	bootstrapToken string

	mu        sync.Mutex
	claimed   bool
	completed bool
	sessionID string
	csrfToken string
}

type installerPage struct {
	Title            string
	Lang             string
	Text             localization.InstallerMessages
	Token            string
	Error            string
	OwnerEmail       string
	OwnerDisplayName string
	DefaultLocale    string
	SupportedLocales string
	TimeZone         string
}

type installerState struct {
	Stage            string `json:"stage"`
	CSRFToken        string `json:"csrf_token,omitempty"`
	DefaultLocale    string `json:"default_locale"`
	SupportedLocales string `json:"supported_locales"`
	TimeZone         string `json:"time_zone"`
}

type installerFieldError struct {
	Field   string
	Message string
}

func (err *installerFieldError) Error() string { return err.Message }

func NewInstaller(store *sqlite.Store, config InstallerConfig) (*InstallerServer, string, error) {
	if config.DefaultLocale == "" {
		config.DefaultLocale = "en-US"
	}
	if config.DefaultTimeZone == "" {
		config.DefaultTimeZone = "UTC"
	}
	generated := ""
	if config.BootstrapToken == "" {
		token, err := identity.NewToken(24)
		if err != nil {
			return nil, "", err
		}
		config.BootstrapToken = token
		generated = token
	}
	tmpl, err := template.New("pages").Funcs(template.FuncMap{"urlquery": url.QueryEscape}).ParseFS(content, "templates/*.tmpl")
	if err != nil {
		return nil, "", fmt.Errorf("parse installer templates: %w", err)
	}
	static, err := fs.Sub(content, "static")
	if err != nil {
		return nil, "", err
	}
	server := &InstallerServer{
		store: store, config: config, templates: tmpl, mux: http.NewServeMux(),
		bootstrapToken: config.BootstrapToken,
	}
	server.mux.Handle("GET /static/system/", http.StripPrefix("/static/", http.FileServer(http.FS(static))))
	server.mux.HandleFunc("GET /install", server.install)
	server.mux.HandleFunc("GET /install/api/state", server.installState)
	server.mux.HandleFunc("POST /install/claim", server.claim)
	server.mux.HandleFunc("POST /install/complete", server.complete)
	server.mux.HandleFunc("GET /health/live", func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "text/plain; charset=utf-8")
		_, _ = w.Write([]byte("live\n"))
	})
	server.mux.HandleFunc("GET /health/ready", func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "text/plain; charset=utf-8")
		w.WriteHeader(http.StatusServiceUnavailable)
		_, _ = w.Write([]byte("not ready\n"))
	})
	server.mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/" {
			http.Redirect(w, r, "/install", http.StatusTemporaryRedirect)
			return
		}
		http.NotFound(w, r)
	})
	return server, generated, nil
}

func (s *InstallerServer) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Cache-Control", "no-store")
	w.Header().Set("Content-Security-Policy", "default-src 'none'; script-src 'self'; style-src 'self' 'unsafe-inline'; img-src 'self' data:; connect-src 'self'; form-action 'self'; frame-ancestors 'none'; base-uri 'none'")
	w.Header().Set("Referrer-Policy", "no-referrer")
	w.Header().Set("X-Content-Type-Options", "nosniff")
	s.mux.ServeHTTP(w, r)
}

func (s *InstallerServer) install(w http.ResponseWriter, r *http.Request) {
	s.render(w, "install-shell", localizedInstallerPage(r, "install-form", installerPage{}))
}

func (s *InstallerServer) installState(w http.ResponseWriter, r *http.Request) {
	writeInstallerJSON(w, http.StatusOK, s.currentInstallerState(r))
}

func (s *InstallerServer) currentInstallerState(r *http.Request) installerState {
	state := installerState{
		Stage: "claim", DefaultLocale: s.config.DefaultLocale,
		SupportedLocales: s.config.DefaultLocale, TimeZone: s.config.DefaultTimeZone,
	}
	s.mu.Lock()
	claimed, completed := s.claimed, s.completed
	s.mu.Unlock()
	if completed {
		state.Stage = "complete"
		return state
	}
	if csrf, ok := s.readSession(r); ok {
		state.Stage = "setup"
		state.CSRFToken = csrf
		return state
	}
	if claimed {
		state.Stage = "claimed"
	}
	return state
}

func (s *InstallerServer) claim(w http.ResponseWriter, r *http.Request) {
	provided := r.FormValue("token")
	if len(provided) != len(s.bootstrapToken) || subtle.ConstantTimeCompare([]byte(provided), []byte(s.bootstrapToken)) != 1 {
		if installerWantsJSON(r) {
			writeInstallerJSON(w, http.StatusUnauthorized, map[string]string{"error": localizedInstallerError(installerLocale(r), "Invalid bootstrap token."), "field": "token"})
			return
		}
		w.WriteHeader(http.StatusUnauthorized)
		s.render(w, "install-claim", localizedInstallerPage(r, "install-claim", installerPage{Error: "Invalid bootstrap token."}))
		return
	}
	sessionID, err := identity.NewToken(32)
	if err != nil {
		s.internalError(w, err)
		return
	}
	csrfToken, err := identity.NewToken(32)
	if err != nil {
		s.internalError(w, err)
		return
	}
	s.mu.Lock()
	if s.claimed {
		s.mu.Unlock()
		if installerWantsJSON(r) {
			writeInstallerJSON(w, http.StatusConflict, map[string]string{"error": localization.InstallerFor(installerLocale(r)).ClaimedInstructions})
			return
		}
		w.WriteHeader(http.StatusConflict)
		s.render(w, "install-claimed", localizedInstallerPage(r, "install-claimed", installerPage{}))
		return
	}
	s.claimed = true
	s.sessionID = sessionID
	s.csrfToken = csrfToken
	s.mu.Unlock()
	http.SetCookie(w, &http.Cookie{
		Name: "prods_install", Value: sessionID, Path: "/install", HttpOnly: true,
		SameSite: http.SameSiteStrictMode,
	})
	if installerWantsJSON(r) {
		writeInstallerJSON(w, http.StatusOK, installerState{
			Stage:            "setup",
			CSRFToken:        csrfToken,
			DefaultLocale:    s.config.DefaultLocale,
			SupportedLocales: s.config.DefaultLocale,
			TimeZone:         s.config.DefaultTimeZone,
		})
		return
	}
	// Remove the bootstrap token from the browser address bar and subsequent
	// requests. The server never places it in normal access or Admin logs.
	http.Redirect(w, r, "/install?lang="+url.QueryEscape(installerLocale(r)), http.StatusSeeOther)
}

func (s *InstallerServer) complete(w http.ResponseWriter, r *http.Request) {
	csrf, ok := s.readSession(r)
	if !ok {
		if installerWantsJSON(r) {
			writeInstallerJSON(w, http.StatusUnauthorized, map[string]string{"error": localization.InstallerFor(installerLocale(r)).InvalidInstallerSession})
			return
		}
		http.Error(w, localization.InstallerFor(installerLocale(r)).InvalidInstallerSession, http.StatusUnauthorized)
		return
	}
	providedCSRF := r.FormValue("csrf_token")
	if len(providedCSRF) != len(csrf) || subtle.ConstantTimeCompare([]byte(providedCSRF), []byte(csrf)) != 1 {
		if installerWantsJSON(r) {
			writeInstallerJSON(w, http.StatusForbidden, map[string]string{"error": localization.InstallerFor(installerLocale(r)).InvalidCSRF})
			return
		}
		http.Error(w, localization.InstallerFor(installerLocale(r)).InvalidCSRF, http.StatusForbidden)
		return
	}
	page := installerPage{
		Lang: installerLocale(r), Title: "Set up Prods", OwnerEmail: strings.TrimSpace(r.FormValue("owner_email")),
		OwnerDisplayName: strings.TrimSpace(r.FormValue("owner_display_name")),
		DefaultLocale:    strings.TrimSpace(r.FormValue("default_locale")),
		SupportedLocales: strings.TrimSpace(r.FormValue("supported_locales")),
		TimeZone:         strings.TrimSpace(r.FormValue("time_zone")),
	}
	locales, err := validateInstallationForm(page, r.FormValue("password"), r.FormValue("password_confirm"))
	if err != nil {
		if installerWantsJSON(r) {
			response := map[string]string{"error": localizedInstallerError(page.Lang, err.Error())}
			var fieldError *installerFieldError
			if errors.As(err, &fieldError) {
				response["field"] = fieldError.Field
			}
			writeInstallerJSON(w, http.StatusUnprocessableEntity, response)
			return
		}
		page.Error = err.Error()
		w.WriteHeader(http.StatusBadRequest)
		s.renderWithCSRF(w, "install-form", page, csrf)
		return
	}
	if err := checkWritableDirectory(s.config.DataDir); err != nil {
		if installerWantsJSON(r) {
			writeInstallerJSON(w, http.StatusUnprocessableEntity, map[string]string{"error": localizedInstallerError(page.Lang, "Data directory is not writable: "+err.Error())})
			return
		}
		page.Error = "Data directory is not writable: " + err.Error()
		w.WriteHeader(http.StatusBadRequest)
		s.renderWithCSRF(w, "install-form", page, csrf)
		return
	}
	if err := checkWritableDirectory(s.config.BackupDir); err != nil {
		if installerWantsJSON(r) {
			writeInstallerJSON(w, http.StatusUnprocessableEntity, map[string]string{"error": localizedInstallerError(page.Lang, "Backup directory is not writable: "+err.Error())})
			return
		}
		page.Error = "Backup directory is not writable: " + err.Error()
		w.WriteHeader(http.StatusBadRequest)
		s.renderWithCSRF(w, "install-form", page, csrf)
		return
	}
	passwordHash, err := identity.HashPassword(r.FormValue("password"))
	if err != nil {
		if installerWantsJSON(r) {
			writeInstallerJSON(w, http.StatusUnprocessableEntity, map[string]string{"error": localizedInstallerError(page.Lang, err.Error()), "field": "password"})
			return
		}
		page.Error = err.Error()
		w.WriteHeader(http.StatusBadRequest)
		s.renderWithCSRF(w, "install-form", page, csrf)
		return
	}
	_, err = s.store.CompleteInstallation(r.Context(), sqlite.Installation{
		OwnerEmail: page.OwnerEmail, OwnerDisplayName: page.OwnerDisplayName, PasswordHash: passwordHash,
		DefaultLocale: page.DefaultLocale, SupportedLocales: locales, TimeZone: page.TimeZone,
	})
	if err != nil {
		if errors.Is(err, sqlite.ErrInstallationState) || errors.Is(err, sqlite.ErrOwnerAlreadyExists) {
			if installerWantsJSON(r) {
				writeInstallerJSON(w, http.StatusConflict, map[string]string{"error": "installation state changed; restart Prods and follow the reported mode"})
				return
			}
			http.Error(w, "installation state changed; restart Prods and follow the reported mode", http.StatusConflict)
			return
		}
		s.internalError(w, err)
		return
	}
	s.mu.Lock()
	s.completed = true
	s.mu.Unlock()
	if installerWantsJSON(r) {
		writeInstallerJSON(w, http.StatusOK, s.currentInstallerState(r))
	} else {
		s.render(w, "install-complete", localizedInstallerPage(r, "install-complete", installerPage{}))
	}
	if s.config.OnComplete != nil {
		defer func() { go s.config.OnComplete() }()
	}
}

func (s *InstallerServer) readSession(r *http.Request) (string, bool) {
	cookie, err := r.Cookie("prods_install")
	if err != nil {
		return "", false
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if !s.claimed || s.completed || len(cookie.Value) != len(s.sessionID) ||
		subtle.ConstantTimeCompare([]byte(cookie.Value), []byte(s.sessionID)) != 1 {
		return "", false
	}
	return s.csrfToken, true
}

func validateInstallationForm(page installerPage, password, confirmation string) ([]string, error) {
	address, err := mail.ParseAddress(page.OwnerEmail)
	if err != nil || identity.NormalizeEmail(address.Address) != identity.NormalizeEmail(page.OwnerEmail) {
		return nil, &installerFieldError{Field: "owner_email", Message: "enter a valid Owner email address"}
	}
	if password != confirmation {
		return nil, &installerFieldError{Field: "password_confirm", Message: "password confirmation does not match"}
	}
	if err := identity.ValidatePassword(password); err != nil {
		return nil, &installerFieldError{Field: "password", Message: err.Error()}
	}
	defaultTag, err := language.Parse(page.DefaultLocale)
	if err != nil {
		return nil, &installerFieldError{Field: "default_locale", Message: "default locale is not valid"}
	}
	if !localization.IsAvailable(defaultTag.String()) {
		return nil, &installerFieldError{Field: "default_locale", Message: "default locale is not available in this binary"}
	}
	parts := strings.Split(page.SupportedLocales, ",")
	locales := make([]string, 0, len(parts))
	foundDefault := false
	for _, part := range parts {
		part = strings.TrimSpace(part)
		if part == "" {
			continue
		}
		tag, err := language.Parse(part)
		if err != nil {
			return nil, &installerFieldError{Field: "supported_locales", Message: fmt.Sprintf("supported locale %q is not valid", part)}
		}
		if !localization.IsAvailable(tag.String()) {
			return nil, &installerFieldError{Field: "supported_locales", Message: fmt.Sprintf("supported locale %q is not available in this binary", part)}
		}
		locales = append(locales, tag.String())
		foundDefault = foundDefault || tag.String() == defaultTag.String()
	}
	if len(locales) == 0 || !foundDefault {
		return nil, &installerFieldError{Field: "supported_locales", Message: "supported locales must include the default locale"}
	}
	if _, err := time.LoadLocation(page.TimeZone); err != nil {
		return nil, &installerFieldError{Field: "time_zone", Message: "time zone is not valid"}
	}
	return locales, nil
}

func checkWritableDirectory(path string) error {
	if path == "" {
		return errors.New("path is empty")
	}
	if err := os.MkdirAll(path, 0o700); err != nil {
		return err
	}
	file, err := os.CreateTemp(path, ".prods-install-check-*")
	if err != nil {
		return err
	}
	name := file.Name()
	if err := file.Sync(); err != nil {
		_ = file.Close()
		_ = os.Remove(name)
		return err
	}
	if err := file.Close(); err != nil {
		_ = os.Remove(name)
		return err
	}
	if err := os.Remove(name); err != nil {
		return err
	}
	return nil
}

func (s *InstallerServer) renderWithCSRF(w http.ResponseWriter, name string, page installerPage, csrf string) {
	page = prepareInstallerPage(name, page)
	type installerFormData struct {
		installerPage
		CSRF string
	}
	s.render(w, name, installerFormData{installerPage: page, CSRF: csrf})
}

func localizedInstallerPage(r *http.Request, name string, page installerPage) installerPage {
	page.Lang = installerLocale(r)
	return prepareInstallerPage(name, page)
}

func prepareInstallerPage(name string, page installerPage) installerPage {
	if page.Lang == "" {
		page.Lang = "en-US"
	}
	page.Text = localization.InstallerFor(page.Lang)
	switch name {
	case "install-claim":
		page.Title = page.Text.ClaimTitle
	case "install-claimed":
		page.Title = page.Text.ClaimedTitle
	case "install-form":
		page.Title = page.Text.SetupTitle
	case "install-complete":
		page.Title = page.Text.CompleteTitle
	}
	page.Error = localizedInstallerError(page.Lang, page.Error)
	return page
}

func installerLocale(r *http.Request) string {
	explicit := strings.TrimSpace(r.FormValue("interface_locale"))
	if explicit == "" {
		explicit = strings.TrimSpace(r.URL.Query().Get("lang"))
	}
	locale, err := localization.Resolve("en-US", []string{"en-US", "zh-TW"}, explicit, r.Header.Get("Accept-Language"))
	if err != nil {
		return "en-US"
	}
	return locale
}

func localizedInstallerError(locale, message string) string {
	if message == "" || !strings.HasPrefix(strings.ToLower(locale), "zh") {
		return message
	}
	text := localization.InstallerFor(locale)
	switch {
	case strings.Contains(message, "bootstrap token"):
		return text.InvalidBootstrapToken
	case strings.Contains(message, "valid Owner email"):
		return text.InvalidOwnerEmail
	case strings.Contains(message, "confirmation"):
		return text.PasswordMismatch
	case strings.Contains(strings.ToLower(message), "password"):
		return text.InvalidPassword
	case strings.Contains(message, "must include the default locale"):
		return text.DefaultMissingFromSupported
	case strings.Contains(message, "default locale"):
		return text.InvalidDefaultLocale
	case strings.Contains(message, "supported locale"):
		return text.InvalidSupportedLocale
	case strings.Contains(message, "time zone"):
		return text.InvalidTimeZone
	case strings.Contains(message, "Data directory"):
		return text.DataDirectoryNotWritable + strings.TrimPrefix(message, "Data directory is not writable")
	case strings.Contains(message, "Backup directory"):
		return text.BackupDirectoryNotWritable + strings.TrimPrefix(message, "Backup directory is not writable")
	default:
		return message
	}
}

func (s *InstallerServer) render(w http.ResponseWriter, name string, data any) {
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	if err := s.templates.ExecuteTemplate(w, name, data); err != nil {
		http.Error(w, "template error", http.StatusInternalServerError)
	}
}

func (s *InstallerServer) internalError(w http.ResponseWriter, err error) {
	slog.Error("installer request failed", "error", err)
	http.Error(w, "internal error", http.StatusInternalServerError)
}

func installerWantsJSON(r *http.Request) bool {
	return strings.Contains(strings.ToLower(r.Header.Get("Accept")), "application/json")
}

func writeInstallerJSON(w http.ResponseWriter, status int, value any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(status)
	if err := json.NewEncoder(w).Encode(value); err != nil {
		slog.Error("write installer JSON", "error", err)
	}
}
