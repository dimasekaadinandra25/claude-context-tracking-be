package jwt

import (
	"strings"
	"testing"
	"time"

	"github.com/golang-jwt/jwt/v5"
)

func TestValidateSecretRejectsWeak(t *testing.T) {
	for name, secret := range map[string]string{
		"empty":       "",
		"short":       strings.Repeat("a", 31),
		"placeholder": "your-super-secret-key-change-in-production-min-32-characters",
	} {
		if err := ValidateSecret(secret); err == nil {
			t.Errorf("%s: ValidateSecret returned nil, want an error", name)
		}
	}
	if err := ValidateSecret("9f8c2b7e4d1a6c3f0b5e8d2a7c4f1b9e6d3a0c5f8b2e7d4a"); err != nil {
		t.Fatalf("strong secret: %v", err)
	}
}

func TestParseRejectsWrongIssuer(t *testing.T) {
	secret := "9f8c2b7e4d1a6c3f0b5e8d2a7c4f1b9e6d3a0c5f8b2e7d4a"
	t.Setenv("JWT_SECRET", secret)
	now := time.Now()
	token, err := jwt.NewWithClaims(jwt.SigningMethodHS256, &Claims{
		UserID: "u",
		RegisteredClaims: jwt.RegisteredClaims{
			ExpiresAt: jwt.NewNumericDate(now.Add(time.Hour)),
			IssuedAt:  jwt.NewNumericDate(now),
			Issuer:    "tuai-api",
		},
	}).SignedString([]byte(secret))
	if err != nil {
		t.Fatal(err)
	}
	if _, err := ParseToken(token); err == nil {
		t.Fatal("token with issuer tuai-api parsed, want an error")
	}

	good, err := GenerateToken("u", "u@x.test", "u", "u", false, nil)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := ParseToken(good); err != nil {
		t.Fatalf("own token: %v", err)
	}
}
