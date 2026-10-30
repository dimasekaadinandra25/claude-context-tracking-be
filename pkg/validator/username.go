package validator

import (
	"errors"
	"regexp"
)

var (
	ErrUsernameTooShort      = errors.New("username must be at least 3 characters long")
	ErrUsernameTooLong       = errors.New("username must not exceed 20 characters")
	ErrInvalidUsernameFormat = errors.New("username can only contain letters, numbers, underscores, and hyphens")
	ErrUsernameStartsInvalid = errors.New("username must start with a letter or number")
)

// Username validation regex: alphanumeric, underscore, hyphen (3-20 chars)
var usernameRegex = regexp.MustCompile(`^[a-zA-Z0-9][a-zA-Z0-9_-]{2,19}$`)

// ValidateUsername validates username format
func ValidateUsername(username string) error {
	// Check length
	if len(username) < 3 {
		return ErrUsernameTooShort
	}

	if len(username) > 20 {
		return ErrUsernameTooLong
	}

	// Check format
	if !usernameRegex.MatchString(username) {
		// Check if it starts with invalid character
		if username[0] == '_' || username[0] == '-' {
			return ErrUsernameStartsInvalid
		}
		return ErrInvalidUsernameFormat
	}

	return nil
}
