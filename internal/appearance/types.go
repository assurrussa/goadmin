// Package appearance owns personal interface preferences and published branding.
package appearance

import (
	"bytes"
	"encoding/base64"
	"errors"
	"fmt"
	"image"
	_ "image/jpeg" // Register JPEG decoding for normalized branding images.
	"image/png"
	"regexp"
	"strings"
	"unicode"
	"unicode/utf8"
)

var (
	ErrInvalid = errors.New("invalid appearance request")
	ErrClosed  = errors.New("appearance service closed")
)

const (
	MaxLogoBytes      = 512 * 1024
	PersonalNamespace = "ui.appearance"
)

type PersonalPayload struct {
	SchemaVersion int    `json:"schemaVersion"`
	Theme         string `json:"theme"`
	Locale        string `json:"locale"`
	Density       string `json:"density"`
}
type PersonalDocument struct {
	Owner   string          `json:"owner"`
	Version int64           `json:"version"`
	Payload PersonalPayload `json:"payload"`
}
type PersonalRequest struct {
	Version int64           `json:"version"`
	Payload PersonalPayload `json:"payload"`
}

func DefaultPersonal() PersonalPayload { return PersonalPayload{1, "system", "ru", "default"} }
func (p PersonalPayload) Validate() error {
	if p.SchemaVersion != 1 ||
		(p.Theme != "system" && p.Theme != "light" && p.Theme != "dark") ||
		(p.Locale != "ru" && p.Locale != "en") ||
		(p.Density != "default" && p.Density != "compact" && p.Density != "comfortable") {
		return ErrInvalid
	}
	return nil
}

type PublicBranding struct {
	Version      int64  `json:"version"`
	Name         string `json:"name"`
	AccentColor  string `json:"accentColor"`
	LogoLightURL string `json:"logoLightUrl"`
	LogoDarkURL  string `json:"logoDarkUrl"`
}
type BrandingPayload struct {
	Name         string `json:"name"`
	AccentColor  string `json:"accentColor"`
	LogoLightURL string `json:"logoLightUrl"`
	LogoDarkURL  string `json:"logoDarkUrl"`
}
type BrandingDocument struct {
	Version int64           `json:"version"`
	Payload BrandingPayload `json:"payload"`
	Pending bool            `json:"pending,omitempty"`
}
type SaveBrandingRequest struct {
	Version     int64   `json:"version"`
	Name        string  `json:"name"`
	AccentColor string  `json:"accentColor"`
	LogoLight   *string `json:"logoLight,omitempty"`
	LogoDark    *string `json:"logoDark,omitempty"`
	Reset       bool    `json:"reset,omitempty"`
}

// Snapshot stores image bytes as immutable strings, safe across shallow RCU copies.
type Snapshot struct {
	Version     int64  `db:"version"`
	Name        string `db:"name"`
	AccentColor string `db:"accent_color"`
	LogoLight   string `db:"logo_light"`
	LogoDark    string `db:"logo_dark"`
}

func (s Snapshot) Public() PublicBranding {
	p := PublicBranding{Version: s.Version, Name: s.Name, AccentColor: s.AccentColor}
	if s.LogoLight != "" {
		p.LogoLightURL = fmt.Sprintf("/public/branding/light?v=%d", s.Version)
	}
	if s.LogoDark != "" {
		p.LogoDarkURL = fmt.Sprintf("/public/branding/dark?v=%d", s.Version)
	}
	return p
}

func (s Snapshot) Document() BrandingDocument {
	p := s.Public()
	return BrandingDocument{Version: p.Version, Payload: BrandingPayload{p.Name, p.AccentColor, p.LogoLightURL, p.LogoDarkURL}}
}

var accentPattern = regexp.MustCompile(`^#[0-9a-fA-F]{6}$`)

func (r SaveBrandingRequest) apply(old Snapshot) (Snapshot, error) {
	if r.Version < 0 {
		return Snapshot{}, ErrInvalid
	}
	old.Version = r.Version + 1
	if r.Reset {
		return Snapshot{Version: old.Version}, nil
	}
	if !utf8.ValidString(r.Name) ||
		utf8.RuneCountInString(r.Name) > 80 ||
		strings.IndexFunc(r.Name, unicode.IsControl) >= 0 ||
		(r.AccentColor != "" && !accentPattern.MatchString(r.AccentColor)) {
		return Snapshot{}, ErrInvalid
	}
	old.Name = strings.TrimSpace(r.Name)
	old.AccentColor = strings.ToUpper(r.AccentColor)
	for _, pair := range []struct {
		src *string
		dst *string
	}{{r.LogoLight, &old.LogoLight}, {r.LogoDark, &old.LogoDark}} {
		if pair.src == nil {
			continue
		}
		if *pair.src == "" {
			*pair.dst = ""
			continue
		}
		normalized, err := NormalizeLogo(*pair.src)
		if err != nil {
			return Snapshot{}, err
		}
		*pair.dst = string(normalized)
	}
	return old, nil
}

func NormalizeLogo(encoded string) ([]byte, error) {
	if len(encoded) > base64.StdEncoding.EncodedLen(MaxLogoBytes) {
		return nil, ErrInvalid
	}
	raw, err := base64.StdEncoding.Strict().DecodeString(encoded)
	if err != nil ||
		len(raw) > MaxLogoBytes {
		return nil, ErrInvalid
	}
	cfg, format, err := image.DecodeConfig(bytes.NewReader(raw))
	if err != nil ||
		(format != "png" && format != "jpeg") ||
		cfg.Width < 1 ||
		cfg.Height < 1 ||
		cfg.Width > 1024 ||
		cfg.Height > 1024 {
		return nil, ErrInvalid
	}
	img, _, err := image.Decode(bytes.NewReader(raw))
	if err != nil {
		return nil, ErrInvalid
	}
	var out bytes.Buffer
	if err = png.Encode(&out, img); err != nil {
		return nil, ErrInvalid
	}
	return out.Bytes(), nil
}
