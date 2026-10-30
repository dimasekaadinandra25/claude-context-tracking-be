package token

import (
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"fmt"
	"math/big"

	"golang.org/x/crypto/bcrypt"
)

// GenerateSecureToken generates a cryptographically secure random token
func GenerateSecureToken(length int) (string, error) {
	bytes := make([]byte, length)
	if _, err := rand.Read(bytes); err != nil {
		return "", fmt.Errorf("failed to generate secure token: %w", err)
	}
	return hex.EncodeToString(bytes), nil
}

// GenerateBase64Token generates a base64 encoded random token
func GenerateBase64Token(length int) (string, error) {
	bytes := make([]byte, length)
	if _, err := rand.Read(bytes); err != nil {
		return "", fmt.Errorf("failed to generate base64 token: %w", err)
	}
	return base64.URLEncoding.EncodeToString(bytes), nil
}

// GenerateEmailVerificationToken generates a token for email verification
func GenerateEmailVerificationToken() (string, error) {
	return GenerateSecureToken(32) // 64 character hex string
}

// GeneratePasswordResetToken generates a token for password reset
func GeneratePasswordResetToken() (string, error) {
	return GenerateSecureToken(32) // 64 character hex string
}

// GenerateRefreshToken generates a token for refresh tokens
func GenerateRefreshToken() (string, error) {
	return GenerateBase64Token(64) // Base64 encoded 64-byte token
}

// HashRefreshToken creates a SHA256 hash of the refresh token for storage
func HashRefreshToken(token string) string {
	hash := sha256.Sum256([]byte(token))
	return hex.EncodeToString(hash[:])
}

// GenerateOTP generates a cryptographically secure 6-digit OTP code
func GenerateOTP() (string, error) {
	// Generate a random number between 100000 and 999999
	max := big.NewInt(900000) // 999999 - 100000
	n, err := rand.Int(rand.Reader, max)
	if err != nil {
		return "", fmt.Errorf("failed to generate OTP: %w", err)
	}

	// Add 100000 to ensure 6 digits
	otp := n.Int64() + 100000
	return fmt.Sprintf("%06d", otp), nil
}

// HashOTP creates a bcrypt hash of the OTP for secure storage
// Uses bcrypt instead of SHA256 for better security against rainbow table attacks
func HashOTP(otp string) (string, error) {
	hash, err := bcrypt.GenerateFromPassword([]byte(otp), bcrypt.DefaultCost)
	if err != nil {
		return "", fmt.Errorf("failed to hash OTP: %w", err)
	}
	return string(hash), nil
}

// VerifyOTP verifies if the provided OTP matches the hashed OTP
func VerifyOTP(otp, hashedOTP string) bool {
	err := bcrypt.CompareHashAndPassword([]byte(hashedOTP), []byte(otp))
	return err == nil
}
