package keycrypt

import (
	"strings"
	"testing"
)

func TestKeycryptRoundTrip(t *testing.T) {
	t.Setenv(EnvName, strings.Repeat("ab", 32))
	sealed, err := Seal("wiz_live_abc_secret")
	if err != nil {
		t.Fatal(err)
	}
	plain, err := Open(sealed)
	if err != nil {
		t.Fatal(err)
	}
	if plain != "wiz_live_abc_secret" {
		t.Fatalf("Open returned %q", plain)
	}

	tampered := append([]byte(nil), sealed...)
	tampered[len(tampered)-1] ^= 1
	if _, err := Open(tampered); err == nil {
		t.Fatal("tampered ciphertext opened, want an error")
	}

	t.Setenv(EnvName, strings.Repeat("cd", 32))
	if _, err := Open(sealed); err == nil {
		t.Fatal("opened with a different key, want an error")
	}
}

func TestValidateEncryptionKey(t *testing.T) {
	for name, key := range map[string]string{
		"empty":       "",
		"short":       strings.Repeat("a", 62),
		"odd":         strings.Repeat("a", 63),
		"long":        strings.Repeat("a", 66),
		"non-hex":     strings.Repeat("g", 64),
		"placeholder": placeholder,
	} {
		if err := ValidateKey(key); err == nil {
			t.Errorf("%s: ValidateKey returned nil, want an error", name)
		}
	}
	if err := ValidateKey(strings.Repeat("0f", 32)); err != nil {
		t.Fatalf("64 hex chars: %v", err)
	}
}
