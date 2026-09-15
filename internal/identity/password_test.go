package identity

import (
	"errors"
	"strings"
	"testing"
)

func TestPasswordPolicyAndHash(t *testing.T) {
	for _, invalid := range []string{"short1", "abcdefgh", "12345678"} {
		if err := ValidatePassword(invalid); !errors.Is(err, ErrInvalidPassword) {
			t.Fatalf("ValidatePassword(%q) = %v", invalid, err)
		}
	}
	for _, valid := range []string{"password1", "密碼測試abc1", strings.Repeat("long", 40) + "9"} {
		if err := ValidatePassword(valid); err != nil {
			t.Fatalf("ValidatePassword(%q) = %v", valid, err)
		}
	}
	hash, err := HashPassword("correct horse 7")
	if err != nil {
		t.Fatal(err)
	}
	if hash == "correct horse 7" || strings.Contains(hash, "correct horse 7") {
		t.Fatal("password hash contains the password")
	}
	if ok, err := VerifyPassword(hash, "correct horse 7"); err != nil || !ok {
		t.Fatalf("verify correct password: ok=%v err=%v", ok, err)
	}
	if ok, err := VerifyPassword(hash, "incorrect 7"); err != nil || ok {
		t.Fatalf("verify incorrect password: ok=%v err=%v", ok, err)
	}
}

func TestPasswordRejectsResourceAbuseAndMalformedHash(t *testing.T) {
	if err := ValidatePassword(strings.Repeat("a", maxPasswordBytes) + "1"); !errors.Is(err, ErrPasswordTooLong) {
		t.Fatalf("long password = %v", err)
	}
	if ok, err := VerifyPassword("not-a-hash", "password1"); ok || !errors.Is(err, ErrInvalidPasswordHash) {
		t.Fatalf("malformed hash: ok=%v err=%v", ok, err)
	}
}
