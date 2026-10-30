package validator

import (
	"errors"
	"regexp"
	"strings"
)

var (
	ErrInvalidEmailFormat = errors.New("invalid email format")
	ErrEmailTooLong       = errors.New("email address is too long")
)

// Email validation regex pattern (RFC 5322 compliant)
var emailRegex = regexp.MustCompile(`^[a-zA-Z0-9.!#$%&'*+/=?^_` + "`" + `{|}~-]+@[a-zA-Z0-9](?:[a-zA-Z0-9-]{0,61}[a-zA-Z0-9])?(?:\.[a-zA-Z0-9](?:[a-zA-Z0-9-]{0,61}[a-zA-Z0-9])?)*$`)

// ValidateEmail validates email format
func ValidateEmail(email string) error {
	// Check length
	if len(email) > 254 {
		return ErrEmailTooLong
	}

	// Check format
	if !emailRegex.MatchString(email) {
		return ErrInvalidEmailFormat
	}

	return nil
}

// NormalizeEmail normalizes email address (lowercase, trim spaces)
func NormalizeEmail(email string) string {
	email = strings.TrimSpace(email)
	email = strings.ToLower(email)
	return email
}

// IsDisposableEmail checks if email is from a disposable email provider
// This is a basic check - you may want to use a more comprehensive list or service
func IsDisposableEmail(email string) bool {
	disposableDomains := []string{
		"tempmail.com", "guerrillamail.com", "10minutemail.com",
		"mailinator.com", "throwaway.email", "temp-mail.org",
	}

	parts := strings.Split(email, "@")
	if len(parts) != 2 {
		return false
	}

	domain := strings.ToLower(parts[1])
	for _, disposable := range disposableDomains {
		if domain == disposable {
			return true
		}
	}

	return false
}
