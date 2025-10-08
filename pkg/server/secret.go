package server

import (
	"crypto/rand"
	"encoding/base64"
)

// GenerateOTP generates a cryptographically secure random 12-character OTP.
// Uses crypto/rand for secure randomness and base64 URL-safe encoding.
// Returns a 12-character string suitable for document protection.
func GenerateOTP() string {
	// Generate 9 random bytes
	// base64 encoding: 9 bytes → 12 chars
	b := make([]byte, 9)
	if _, err := rand.Read(b); err != nil {
		panic(err) // Should never fail
	}

	// URL-safe base64 (uses - and _ instead of + and /)
	// RawURLEncoding has no padding (=)
	return base64.RawURLEncoding.EncodeToString(b)
}
