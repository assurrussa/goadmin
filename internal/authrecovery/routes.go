package authrecovery

import (
	"context"
	"errors"
	"net/url"
	"path"
	"strings"
)

type Routes struct {
	ForgotPath string
	ResetPaths []string
	LoginURL   string
	Title      string
	Context    func(context.Context) (context.Context, error)
}

// Normalize snapshots configuration. Existing aliases must share one trusted
// context when custom routes are supplied; a browser can never select scope.
func Normalize(input []Routes) ([]Routes, error) {
	if len(input) == 0 {
		return []Routes{{
			ForgotPath: "/auth/forgot-password", ResetPaths: []string{"/auth/reset-password", "/reset-password"},
			LoginURL: "/auth/login", Context: func(ctx context.Context) (context.Context, error) { return ctx, nil },
		}}, nil
	}
	out := make([]Routes, 0, len(input))
	seen := map[string]bool{}
	for _, r := range input {
		if r.Context == nil || len(r.ResetPaths) == 0 || !validPath(r.ForgotPath) || !validLoginURL(r.LoginURL) {
			return nil, errors.New("invalid password recovery routes")
		}
		r.ResetPaths = append([]string(nil), r.ResetPaths...)
		for _, p := range append([]string{r.ForgotPath}, r.ResetPaths...) {
			if !validPath(p) || seen[p] {
				return nil, errors.New("duplicate or invalid password recovery path")
			}
			seen[p] = true
		}
		out = append(out, r)
	}
	found := false
	for _, r := range out {
		if r.ForgotPath == "/auth/forgot-password" {
			aliases := map[string]bool{}
			for _, p := range r.ResetPaths {
				aliases[p] = true
			}
			found = aliases["/auth/reset-password"] && aliases["/reset-password"]
		}
	}
	if !found {
		return nil, errors.New("password recovery must explicitly scope all legacy aliases")
	}
	return out, nil
}

func validPath(s string) bool {
	u, err := url.Parse(s)
	return err == nil && strings.HasPrefix(s, "/") && !strings.HasPrefix(s, "//") &&
		!strings.ContainsAny(s, "\\%:*?#") && u.Path == s && path.Clean(s) == s && s != "/"
}

func Paths(routes []Routes) []string {
	var out []string
	for _, r := range routes {
		out = append(out, r.ForgotPath)
		out = append(out, r.ResetPaths...)
	}
	return out
}

func validLoginURL(s string) bool {
	if s == "" || validPath(s) {
		return true
	}
	u, err := url.Parse(s)
	return err == nil && (u.Scheme == "https" || u.Scheme == "http") && u.Host != "" && u.User == nil && u.Fragment == ""
}
