package auth

import (
	"errors"
)

var (
	// ErrInvalidToken is returned when the token is invalid
	ErrInvalidToken = errors.New("invalid or missing API key")
	// ErrMissingAPIKey is returned when the X-API-Key header is missing
	ErrMissingAPIKey = errors.New("missing X-API-Key header")
)

// Validator validates API keys
type Validator struct {
	apiKey string
}

// NewValidator creates a new API key validator
func NewValidator(apiKey string) *Validator {
	return &Validator{
		apiKey: apiKey,
	}
}

// ValidateAPIKey validates an API key from X-API-Key header
func (v *Validator) ValidateAPIKey(apiKey string) error {
	if apiKey == "" {
		return ErrMissingAPIKey
	}

	if apiKey != v.apiKey {
		return ErrInvalidToken
	}

	return nil
}

// ValidateToken is kept for backward compatibility (used by MCP middleware)
// It validates bearer token from Authorization header
func (v *Validator) ValidateToken(authHeader string) error {
	if authHeader == "" {
		return errors.New("missing Authorization header")
	}

	// For MCP: Extract token from "Bearer <token>"
	token := authHeader
	if len(authHeader) > 7 && authHeader[:7] == "Bearer " {
		token = authHeader[7:]
	}

	if token != v.apiKey {
		return ErrInvalidToken
	}

	return nil
}
