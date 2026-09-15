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
		s.internalAPIError(w, r, err)
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
		s.writeAPIError(w, r, http.StatusBadRequest, apiCodeInvalidJSON)
		return
	}
	role, err := s.store.CreateRole(r.Context(), current.UserID, role)
	if err != nil {
		s.writeUserError(w, r, err)
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
		s.internalAPIError(w, r, err)
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
		s.writeAPIError(w, r, http.StatusBadRequest, apiCodeInvalidJSON)
		return
	}
	grant, err := s.store.CreateUser(r.Context(), current.UserID, request.Email, request.DisplayName, request.RoleID, 24*time.Hour)
	if err != nil {
		s.writeUserError(w, r, err)
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
		s.writeUserError(w, r, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (s *Server) adminChangeUserRole(w http.ResponseWriter, r *http.Request) {
	current, ok := s.requireCapability(w, r, identity.CapabilityUsersManage, true)
	if !ok {
		return
	}
	var request struct {
		RoleID string `json:"role_id"`
	}
	if err := decodeJSON(r.Body, &request); err != nil {
		s.writeAPIError(w, r, http.StatusBadRequest, apiCodeInvalidJSON)
		return
	}
	user, err := s.store.ChangeUserRole(r.Context(), current.UserID, r.PathValue("id"), request.RoleID)
	if err != nil {
		s.writeUserError(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, user)
}

func (s *Server) adminIssueSetPasswordGrant(w http.ResponseWriter, r *http.Request) {
	current, ok := s.requireCapability(w, r, identity.CapabilityUsersManage, true)
	if !ok {
		return
	}
	grant, err := s.store.IssueSetPasswordGrant(r.Context(), current.UserID, r.PathValue("id"), 24*time.Hour)
	if err != nil {
		s.writeUserError(w, r, err)
		return
	}
	s.writeSetPasswordGrant(w, http.StatusCreated, grant)
}

func (s *Server) adminReactivateUser(w http.ResponseWriter, r *http.Request) {
	current, ok := s.requireCapability(w, r, identity.CapabilityUsersManage, true)
	if !ok {
		return
	}
	grant, err := s.store.ReactivateUser(r.Context(), current.UserID, r.PathValue("id"), 24*time.Hour)
	if err != nil {
		s.writeUserError(w, r, err)
		return
	}
	s.writeSetPasswordGrant(w, http.StatusOK, grant)
}

func (s *Server) writeSetPasswordGrant(w http.ResponseWriter, status int, grant identity.SetPasswordGrant) {
	base := strings.TrimRight(s.config.BaseURL, "/")
	writeJSON(w, status, struct {
		User           identity.User `json:"user"`
		SetPasswordURL string        `json:"set_password_url"`
		ExpiresAt      time.Time     `json:"expires_at"`
	}{grant.User, base + "/set-password?token=" + url.QueryEscape(grant.Token), grant.ExpiresAt})
}

func (s *Server) writeUserError(w http.ResponseWriter, r *http.Request, err error) {
	switch {
	case errors.Is(err, sqlite.ErrLastActiveOwner):
		s.writeAPIError(w, r, http.StatusConflict, apiCodeLastActiveOwner)
	case errors.Is(err, sqlite.ErrInvalidGrant), errors.Is(err, sqlite.ErrUserState), sqlite.IsUniqueViolation(err):
		s.writeAPIError(w, r, http.StatusConflict, apiCodeConflict)
	case errors.Is(err, sqlite.ErrInvalidCredentials):
		s.writeAPIError(w, r, http.StatusUnprocessableEntity, apiCodeValidationFailed)
	case errors.Is(err, sqlite.ErrPermissionDenied):
		s.writeAPIError(w, r, http.StatusForbidden, apiCodeForbidden)
	default:
		s.internalAPIError(w, r, err)
	}
}
