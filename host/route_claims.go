package host

import (
	"fmt"
	"net/http"
	"strings"
)

type routeClaim struct {
	owner, label, method string
	segments             []string
	namespace            bool
}

func parseRouteClaim(route, owner string) (routeClaim, error) {
	fields := strings.Fields(route)
	if len(fields) != 2 || strings.Join(fields, " ") != route || !validClaimPath(fields[1]) {
		return routeClaim{}, fmt.Errorf("invalid route claim %q", route)
	}
	switch fields[0] {
	case http.MethodGet, http.MethodHead, http.MethodPost, http.MethodPut, http.MethodPatch,
		http.MethodDelete, http.MethodConnect, http.MethodOptions, http.MethodTrace, "*":
	default:
		return routeClaim{}, fmt.Errorf("invalid route claim %q", route)
	}
	return routeClaim{owner: owner, label: route, method: fields[0], segments: claimSegments(fields[1])}, nil
}

func validClaimPath(path string) bool {
	return strings.HasPrefix(path, "/") && !strings.ContainsAny(path, "?#\\ \t\r\n") && !strings.Contains(path, "//")
}

func claimSegments(path string) []string {
	path = strings.Trim(path, "/")
	if path == "" {
		return nil
	}
	return strings.Split(path, "/")
}

func moduleRouteClaims(d ModuleDescriptor) ([]routeClaim, error) {
	claims := make([]routeClaim, 0, len(d.Routes)+len(d.RouteNamespaces))
	for _, route := range d.Routes {
		claim, err := parseRouteClaim(route, d.Key)
		if err != nil {
			return nil, err
		}
		claims = append(claims, claim)
	}
	for _, path := range d.RouteNamespaces {
		if !validClaimPath(path) || strings.ContainsAny(path, ":*+") {
			return nil, fmt.Errorf("invalid route namespace %q", path)
		}
		claims = append(claims, routeClaim{
			owner: d.Key, label: path + "/**", method: "*", segments: claimSegments(path), namespace: true,
		})
	}
	return claims, nil
}

func routeClaimsOverlap(a, b routeClaim) bool {
	sameMethod := a.method == b.method || a.method == "*" || b.method == "*"
	getHead := a.method == http.MethodGet && b.method == http.MethodHead ||
		a.method == http.MethodHead && b.method == http.MethodGet
	if !sameMethod && !getHead {
		return false
	}
	for i := 0; ; i++ {
		aEnd, bEnd := i == len(a.segments), i == len(b.segments)
		if aEnd || bEnd {
			return aEnd && bEnd || aEnd && a.namespace || bEnd && b.namespace ||
				!aEnd && strings.Contains(a.segments[i], "*") || !bEnd && strings.Contains(b.segments[i], "*")
		}
		x, y := a.segments[i], b.segments[i]
		// Fiber catch-all parameters can consume multiple segments. Reject a
		// possible overlap even when their literal suffixes differ.
		if strings.ContainsAny(x, "*+") || strings.ContainsAny(y, "*+") {
			return true
		}
		if x != y && !strings.Contains(x, ":") && !strings.Contains(y, ":") {
			return false
		}
	}
}
