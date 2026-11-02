package keycrypt

import (
	"crypto/aes"
	"crypto/cipher"
	"encoding/hex"
	"errors"
	"fmt"
	"os"
)

const (
	EnvName     = "API_KEY_ENCRYPTION_KEY"
	keyBytes    = 32
	placeholder = "generate-with-openssl-rand-hex-32"
)

var ErrInvalidKey = errors.New("API_KEY_ENCRYPTION_KEY must be 64 hex characters")

func ValidateKey(hexKey string) error {
	if hexKey == placeholder {
		return fmt.Errorf("%w: still the .env.example placeholder", ErrInvalidKey)
	}
	key, err := hex.DecodeString(hexKey)
	if err != nil || len(key) != keyBytes {
		return ErrInvalidKey
	}
	return nil
}

func Seal(plain string) ([]byte, error) {
	gcm, err := newGCM()
	if err != nil {
		return nil, err
	}
	return gcm.Seal(nil, nil, []byte(plain), nil), nil
}

func Open(sealed []byte) (string, error) {
	gcm, err := newGCM()
	if err != nil {
		return "", err
	}
	plain, err := gcm.Open(nil, nil, sealed, nil)
	if err != nil {
		return "", err
	}
	return string(plain), nil
}

func newGCM() (cipher.AEAD, error) {
	hexKey := os.Getenv(EnvName)
	if err := ValidateKey(hexKey); err != nil {
		return nil, err
	}
	key, _ := hex.DecodeString(hexKey)
	block, err := aes.NewCipher(key)
	if err != nil {
		return nil, err
	}
	return cipher.NewGCMWithRandomNonce(block)
}
