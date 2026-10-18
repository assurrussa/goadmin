// Package profile supplies optional SSO presentation snapshots, never identity,
// local credential changes, membership, roles or permission authority.
package profile

import (
	"errors"
	"net"
	"net/url"
	"slices"
	"strings"
	"time"
	"unicode"
	"unicode/utf8"

	"github.com/assurrussa/goadmin/internal/presentation"
)

var (
	ErrInvalid  = errors.New("invalid profile presentation")
	ErrConflict = errors.New("profile presentation changed")
	ErrDenied   = errors.New("profile presentation denied")
)

const displayNameClaim = "display_name"

type (
	Fields = presentation.Fields
	View   = presentation.View
)

// Snapshot is server input from a verified, freshly completed callback. Do not
// bind it to browser JSON. Login is an advisory canonical-login label only.
type Snapshot struct {
	Name      string    `json:"name"`
	Login     string    `json:"login"`
	AvatarURL string    `json:"avatarUrl"`
	IssuedAt  time.Time `json:"issuedAt"`
}

type avatarPolicy struct{ hosts []string }

// ValidateAvatarHosts is network-free. Empty policy disables remote avatars.
func ValidateAvatarHosts(hosts []string) error {
	if len(hosts) > 8 {
		return ErrInvalid
	}
	for i, host := range hosts {
		if !validAvatarHost(host) || slices.Contains(hosts[:i], host) {
			return ErrInvalid
		}
	}
	return nil
}

func validAvatarHost(host string) bool {
	if host != strings.ToLower(host) || len(host) > 253 || !strings.Contains(host, ".") ||
		strings.ContainsAny(host, "/:@?#%\\ \t\r\n") || net.ParseIP(host) != nil {
		return false
	}
	for _, suffix := range []string{".localhost", ".local", ".internal", ".lan", ".home.arpa"} {
		if strings.HasSuffix(host, suffix) {
			return false
		}
	}
	for _, label := range strings.Split(host, ".") {
		if !validAvatarLabel(label) {
			return false
		}
	}
	return true
}

func validAvatarLabel(label string) bool {
	if len(label) == 0 || len(label) > 63 || label[0] == '-' || label[len(label)-1] == '-' {
		return false
	}
	for _, b := range []byte(label) {
		valid := b >= 'a' && b <= 'z' || b >= '0' && b <= '9' || b == '-'
		if !valid {
			return false
		}
	}
	return true
}

func (p avatarPolicy) validate(raw string) bool {
	if raw == "" {
		return true
	}
	u, err := url.Parse(raw)
	return err == nil && len(raw) <= 256 && u.Scheme == "https" && u.User == nil && u.Host != "" &&
		u.Host == u.Hostname() && u.Fragment == "" && u.Opaque == "" && u.String() == raw &&
		!strings.ContainsFunc(u.Path, unicode.IsControl) &&
		!strings.ContainsAny(raw, "\\\r\n") && slices.Contains(p.hosts, u.Hostname())
}

func validLabel(value string) bool {
	return len(value) <= 256 && utf8.ValidString(value) && !strings.ContainsFunc(value, unicode.IsControl)
}

func (p avatarPolicy) snapshot(value Snapshot, now time.Time) (Snapshot, error) {
	if len(p.hosts) == 0 {
		value.AvatarURL = ""
	}
	if !validLabel(value.Name) || !validLabel(value.Login) || !p.validate(value.AvatarURL) ||
		value.IssuedAt.IsZero() || value.IssuedAt.After(now.Add(30*time.Second)) ||
		now.Sub(value.IssuedAt) > 30*time.Second {
		return Snapshot{}, ErrInvalid
	}
	return value, nil
}

// FromClaims extracts only the selected namespaced presentation convention.
// Unknown keys never become standard claims, roles, email or verification.
func FromClaims(attributes map[string]string, canonicalLogin string, issuedAt time.Time) Snapshot {
	return Snapshot{
		Name: attributes[displayNameClaim], Login: canonicalLogin,
		AvatarURL: attributes["avatar_url"], IssuedAt: issuedAt,
	}
}
