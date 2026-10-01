package host

import (
	"crypto/sha256"
	"errors"
	"strings"
)

const minimumFirstAdminSetupTokenLength = 32

// FirstAdminSetupToken is an opaque host configuration value for the static
// first-admin bootstrap token. It retains only a SHA-256 hash of the raw token.
type FirstAdminSetupToken struct {
	hash       [sha256.Size]byte
	configured bool
}

// NewFirstAdminSetupToken validates and hashes a host-supplied bootstrap token.
// An empty value disables static first-admin bootstrap.
func NewFirstAdminSetupToken(raw string) (FirstAdminSetupToken, error) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return FirstAdminSetupToken{}, nil
	}
	if len(raw) < minimumFirstAdminSetupTokenLength {
		return FirstAdminSetupToken{}, errors.New("first admin setup token must contain at least 32 bytes")
	}

	return FirstAdminSetupToken{
		hash:       sha256.Sum256([]byte(raw)),
		configured: true,
	}, nil
}
