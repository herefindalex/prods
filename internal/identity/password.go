package identity

import (
	"crypto/pbkdf2"
	"crypto/rand"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/base64"
	"errors"
	"fmt"
	"strconv"
	"strings"
	"unicode"
	"unicode/utf8"
)

const (
	passwordHashVersion    = 1
	passwordHashIterations = 600_000
	passwordSaltBytes      = 16
	passwordKeyBytes       = 32
	maxPasswordBytes       = 1024
)

var (
	ErrInvalidPassword         = errors.New("password must contain at least 8 characters, including a letter and a digit")
	ErrPasswordTooLong         = errors.New("password exceeds the supported length")
	ErrInvalidPasswordHash     = errors.New("invalid password hash")
	ErrUnsupportedPasswordHash = errors.New("unsupported password hash")
)

// ValidatePassword applies the product-level password contract. Character
// counting and the letter/digit checks use Unicode code points; the byte cap
// is a technical resource bound and still permits password-manager output.
func ValidatePassword(password string) error {
	if len(password) > maxPasswordBytes {
		return ErrPasswordTooLong
	}
	if !utf8.ValidString(password) {
		return ErrInvalidPassword
	}
	hasLetter := false
	hasDigit := false
	for _, r := range password {
		hasLetter = hasLetter || unicode.IsLetter(r)
		hasDigit = hasDigit || unicode.IsDigit(r)
	}
	if utf8.RuneCountInString(password) < 8 || !hasLetter || !hasDigit {
		return ErrInvalidPassword
	}
	return nil
}

func HashPassword(password string) (string, error) {
	if err := ValidatePassword(password); err != nil {
		return "", err
	}
	salt := make([]byte, passwordSaltBytes)
	if _, err := rand.Read(salt); err != nil {
		return "", fmt.Errorf("generate password salt: %w", err)
	}
	key, err := pbkdf2.Key(sha256.New, password, salt, passwordHashIterations, passwordKeyBytes)
	if err != nil {
		return "", fmt.Errorf("derive password hash: %w", err)
	}
	return fmt.Sprintf("$pbkdf2-sha256$v=%d$i=%d$%s$%s",
		passwordHashVersion,
		passwordHashIterations,
		base64.RawStdEncoding.EncodeToString(salt),
		base64.RawStdEncoding.EncodeToString(key),
	), nil
}

func VerifyPassword(encoded, password string) (bool, error) {
	if len(password) > maxPasswordBytes {
		return false, ErrPasswordTooLong
	}
	parts := strings.Split(encoded, "$")
	if len(parts) != 6 || parts[0] != "" || parts[1] != "pbkdf2-sha256" {
		return false, ErrInvalidPasswordHash
	}
	version, err := parseParameter(parts[2], "v")
	if err != nil {
		return false, err
	}
	if version != passwordHashVersion {
		return false, ErrUnsupportedPasswordHash
	}
	iterations, err := parseParameter(parts[3], "i")
	if err != nil || iterations < 1 || iterations > passwordHashIterations*2 {
		return false, ErrInvalidPasswordHash
	}
	salt, err := base64.RawStdEncoding.DecodeString(parts[4])
	if err != nil || len(salt) != passwordSaltBytes {
		return false, ErrInvalidPasswordHash
	}
	want, err := base64.RawStdEncoding.DecodeString(parts[5])
	if err != nil || len(want) != passwordKeyBytes {
		return false, ErrInvalidPasswordHash
	}
	got, err := pbkdf2.Key(sha256.New, password, salt, iterations, len(want))
	if err != nil {
		return false, err
	}
	return subtle.ConstantTimeCompare(got, want) == 1, nil
}

func parseParameter(value, name string) (int, error) {
	prefix := name + "="
	if !strings.HasPrefix(value, prefix) {
		return 0, ErrInvalidPasswordHash
	}
	parsed, err := strconv.Atoi(strings.TrimPrefix(value, prefix))
	if err != nil {
		return 0, ErrInvalidPasswordHash
	}
	return parsed, nil
}
