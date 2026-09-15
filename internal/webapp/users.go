package webapp

import (
	"errors"
	"net/http"
	"net/url"
	"strings"
	"time"

	"prods/internal/identity"
	"prods/internal/localization"
	"prods/internal/storage/sqlite"
)

type setPasswordPage struct {
	Title     string
	Language  string
	ActionURL string
	LoginURL  string
	Text      localization.AuthText
	Token     string
	Email     string
	Error     string
	Success   bool
}

func (s *Server) setPassword(w http.ResponseWriter, r *http.Request) {
	if s.config.EnablePOCAdmin {
		http.NotFound(w, r)
		return
	}
	locale, text, err := s.authText(r)
	if err != nil {
		http.Error(w, "unsupported language", http.StatusBadRequest)
		return
	}
	newPage := func(token string) setPasswordPage {
		return setPasswordPage{
			Title: text.SetPasswordTitle, Language: locale,
			ActionURL: withLanguage("/set-password", locale), LoginURL: withLanguage("/admin/login", locale),
			Text: text, Token: token,
		}
	}
	if r.Method == http.MethodGet {
		token := r.URL.Query().Get("token")
		if token == "" {
			http.Error(w, "set-password token is required", http.StatusBadRequest)
			return
		}
		s.render(w, "set-password", newPage(token))
		return
	}
	token := r.FormValue("token")
	password := r.FormValue("password")
	confirmation := r.FormValue("password_confirmation")
	page := newPage(token)
	if password != confirmation {
		w.WriteHeader(http.StatusUnprocessableEntity)
		page.Error = text.PasswordMismatch
		s.render(w, "set-password", page)
		return
	}
	hash, err := identity.HashPassword(password)
	if err != nil {
		w.WriteHeader(http.StatusUnprocessableEntity)
		page.Error = text.InvalidPassword
		s.render(w, "set-password", page)
		return
	}
	user, err := s.store.SetUserPassword(r.Context(), token, hash, time.Now().UTC())
	if err != nil {
		w.WriteHeader(http.StatusConflict)
		page.Error = text.InvalidSetPasswordToken
		s.render(w, "set-password", page)
		return
	}
	page.Token = ""
	page.Email = user.Email
	page.Success = true
	s.render(w, "set-password", page)
}

func (s *Server) adminRoles(w http.ResponseWriter, r *http.Request) {
	if _, ok := s.requireCapability(w, r, identity.CapabilityUsersManage, false); !ok {
		return
	}
	roles, err := s.store.ListRoles(r.Context())
	if err != nil {
		s.internalError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, roles)
}

func (s *Server) adminCreateRole(w http.ResponseWriter, r *http.Request) {
	current, ok := s.requireCapability(w, r, identity.CapabilityUsersManage, true)
	if !ok {
		return
	}
	var role identity.Role
	if err := decodeJSON(r.Body, &role); err != nil {
		http.Error(w, "invalid JSON", http.StatusBadRequest)
		return
	}
	role, err := s.store.CreateRole(r.Context(), current.UserID, role)
	if err != nil {
		s.writeUserError(w, err)
		return
	}
	writeJSON(w, http.StatusCreated, role)
}

func (s *Server) adminUsers(w http.ResponseWriter, r *http.Request) {
	if _, ok := s.requireCapability(w, r, identity.CapabilityUsersManage, false); !ok {
		return
	}
	users, err := s.store.ListUsers(r.Context())
	if err != nil {
		s.internalError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, users)
}

func (s *Server) adminCreateUser(w http.ResponseWriter, r *http.Request) {
	current, ok := s.requireCapability(w, r, identity.CapabilityUsersManage, true)
	if !ok {
		return
	}
	var request struct {
		Email       string `json:"email"`
		DisplayName string `json:"display_name"`
		RoleID      string `json:"role_id"`
	}
	if err := decodeJSON(r.Body, &request); err != nil {
		http.Error(w, "invalid JSON", http.StatusBadRequest)
		return
	}
	grant, err := s.store.CreateUser(r.Context(), current.UserID, request.Email, request.DisplayName, request.RoleID, 24*time.Hour)
	if err != nil {
		s.writeUserError(w, err)
		return
	}
	base := strings.TrimRight(s.config.BaseURL, "/")
	writeJSON(w, http.StatusCreated, struct {
		User           identity.User `json:"user"`
		SetPasswordURL string        `json:"set_password_url"`
		ExpiresAt      time.Time     `json:"expires_at"`
	}{grant.User, base + "/set-password?token=" + url.QueryEscape(grant.Token), grant.ExpiresAt})
}

func (s *Server) adminDisableUser(w http.ResponseWriter, r *http.Request) {
	current, ok := s.requireCapability(w, r, identity.CapabilityUsersManage, true)
	if !ok {
		return
	}
	if err := s.store.DisableUser(r.Context(), current.UserID, r.PathValue("id")); err != nil {
		s.writeUserError(w, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (s *Server) writeUserError(w http.ResponseWriter, err error) {
	switch {
	case errors.Is(err, sqlite.ErrLastActiveOwner), errors.Is(err, sqlite.ErrInvalidGrant), sqlite.IsUniqueViolation(err):
		writeJSON(w, http.StatusConflict, map[string]string{"error": err.Error()})
	case errors.Is(err, sqlite.ErrInvalidCredentials):
		writeJSON(w, http.StatusUnprocessableEntity, map[string]string{"error": err.Error()})
	case errors.Is(err, sqlite.ErrPermissionDenied):
		http.Error(w, "forbidden", http.StatusForbidden)
	default:
		s.internalError(w, err)
	}
}
