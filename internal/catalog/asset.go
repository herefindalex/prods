package catalog

import (
	"encoding/hex"
	"errors"
	"path/filepath"
	"strings"
	"time"
)

var ErrInvalidAsset = errors.New("invalid asset")

type Asset struct {
	ID               string    `json:"id"`
	OwnerType        string    `json:"owner_type"`
	OwnerID          string    `json:"owner_id"`
	OriginalFilename string    `json:"original_filename"`
	StoragePath      string    `json:"-"`
	MIMEType         string    `json:"mime_type"`
	SizeBytes        int64     `json:"size_bytes"`
	Checksum         string    `json:"checksum"`
	CreatedAt        time.Time `json:"created_at"`
}

func (a *Asset) Prepare() error {
	a.ID = strings.TrimSpace(a.ID)
	a.OwnerType = strings.TrimSpace(a.OwnerType)
	a.OwnerID = strings.TrimSpace(a.OwnerID)
	a.OriginalFilename = strings.TrimSpace(filepath.Base(a.OriginalFilename))
	a.StoragePath = filepath.Clean(strings.TrimSpace(a.StoragePath))
	a.MIMEType = strings.TrimSpace(strings.ToLower(a.MIMEType))
	a.Checksum = strings.TrimSpace(strings.ToLower(a.Checksum))
	checksum, checksumErr := hex.DecodeString(a.Checksum)
	validOwnerAndType := (a.OwnerType == "product" && (a.MIMEType == "application/pdf" ||
		a.MIMEType == "image/jpeg" || a.MIMEType == "image/png" || a.MIMEType == "image/webp")) ||
		(a.OwnerType == "website" && a.OwnerID == "working" &&
			(a.MIMEType == "image/jpeg" || a.MIMEType == "image/png" || a.MIMEType == "image/webp" || a.MIMEType == "image/svg+xml"))
	if a.ID == "" || !validOwnerAndType || a.OwnerID == "" || a.OriginalFilename == "" ||
		a.StoragePath == "." || a.StoragePath == ".." || filepath.IsAbs(a.StoragePath) ||
		strings.HasPrefix(a.StoragePath, ".."+string(filepath.Separator)) ||
		a.SizeBytes <= 0 || checksumErr != nil || len(checksum) != 32 {
		return ErrInvalidAsset
	}
	return nil
}
