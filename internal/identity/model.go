package identity

import (
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"strings"
	"time"
)

const (
	RoleOwner    = "role_owner"
	UserActive   = "active"
	UserDisabled = "disabled"
)

type Capability string

const (
	CapabilityAdminAccess    Capability = "admin.access"
	CapabilityCatalogView    Capability = "catalog.view"
	CapabilityCatalogEdit    Capability = "catalog.edit"
	CapabilityCatalogPublish Capability = "catalog.publish"
	CapabilityCatalogImport  Capability = "catalog.import"
	CapabilityCatalogExport  Capability = "catalog.export"
	CapabilityRFQView        Capability = "rfq.view"
	CapabilityRFQManage      Capability = "rfq.manage"
	CapabilityRFQExport      Capability = "rfq.export"
	CapabilityUsersManage    Capability = "users.manage"
	CapabilitySystemManage   Capability = "system.manage"
	CapabilityAuditView      Capability = "audit.view"
)

var OwnerCapabilities = []Capability{
	CapabilityAdminAccess,
	CapabilityCatalogView,
	CapabilityCatalogEdit,
	CapabilityCatalogPublish,
	CapabilityCatalogImport,
	CapabilityCatalogExport,
	CapabilityRFQView,
	CapabilityRFQManage,
	CapabilityRFQExport,
	CapabilityUsersManage,
	CapabilitySystemManage,
	CapabilityAuditView,
}

func ValidCapability(capability Capability) bool {
	for _, candidate := range OwnerCapabilities {
		if capability == candidate {
			return true
		}
	}
	return false
}

type Role struct {
	ID           string       `json:"id"`
	Name         string       `json:"name"`
	Status       string       `json:"status"`
	Revision     int64        `json:"revision"`
	Capabilities []Capability `json:"capabilities"`
}

type User struct {
	ID           string              `json:"id"`
	Email        string              `json:"email"`
	DisplayName  string              `json:"display_name"`
	Role         string              `json:"role_id"`
	Status       string              `json:"status"`
	PasswordHash string              `json:"-"`
	AuthRevision int                 `json:"auth_revision"`
	Capabilities map[Capability]bool `json:"capabilities,omitempty"`
}

func (u User) Can(capability Capability) bool {
	return u.Capabilities[capability]
}

type AdminSession struct {
	User      User
	CSRFToken string
	ExpiresAt time.Time
}

type SetPasswordGrant struct {
	User      User      `json:"user"`
	Token     string    `json:"token"`
	ExpiresAt time.Time `json:"expires_at"`
}

func NormalizeEmail(value string) string {
	return strings.ToLower(strings.TrimSpace(value))
}

func NewToken(byteCount int) (string, error) {
	raw := make([]byte, byteCount)
	if _, err := rand.Read(raw); err != nil {
		return "", err
	}
	return hex.EncodeToString(raw), nil
}

func TokenDigest(token string) string {
	sum := sha256.Sum256([]byte(token))
	return hex.EncodeToString(sum[:])
}
