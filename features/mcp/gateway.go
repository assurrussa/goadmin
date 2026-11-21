package mcp

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/url"
	"regexp"
	"sync"
	"time"
)

var codePattern = regexp.MustCompile(`^[A-Z2-7]{5}-[A-Z2-7]{5}$`)

type gateway struct {
	origin *url.URL
	path   string
	client *http.Client

	// Only pairing/exchange are serialized. Reads may proceed concurrently.
	mu         sync.Mutex
	pending    gatewayPairing
	token      string
	validUntil time.Time
}

type gatewayPairing struct {
	Code            string `json:"code"`
	DeviceSecret    string `json:"device_secret"`    //nolint:tagliatelle // Existing device protocol.
	VerificationURL string `json:"verification_url"` //nolint:tagliatelle // Existing device protocol.
	ExpiresIn       int    `json:"expires_in"`       //nolint:tagliatelle // Existing device protocol.
}

func newGateway(origin, path string) (*gateway, error) {
	u, err := adminOrigin(origin)
	if err != nil {
		return nil, err
	}
	if !pathPattern.MatchString(path) {
		return nil, errors.New("mcp: provide the configured resource path using -resource-path or GOADMIN_MCP_RESOURCE_PATH")
	}
	client := &http.Client{
		Timeout:       10 * time.Second,
		CheckRedirect: func(_ *http.Request, _ []*http.Request) error { return http.ErrUseLastResponse },
	}
	return &gateway{origin: u, path: path, client: client}, nil
}

// post contacts only a route selected by the tool implementation. Neither the
// upstream response body nor an HTTP error gets reflected in a tool error.
func (g *gateway) post(ctx context.Context, route, token string, in, out any) (int, error) {
	data, err := json.Marshal(in)
	if err != nil {
		return 0, err
	}
	//nolint:gosec // Origin and path are validated operator configuration, and the tool selects the endpoint.
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, g.origin.String()+g.path+"/"+route, bytes.NewReader(data))
	if err != nil {
		return 0, err
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "application/json")
	if token != "" {
		req.Header.Set("Authorization", "Bearer "+token)
	}
	//nolint:gosec // The client refuses redirects and uses the configured origin.
	resp, err := g.client.Do(req)
	if err != nil {
		return 0, err
	}
	defer resp.Body.Close()
	if resp.StatusCode == http.StatusOK || resp.StatusCode == http.StatusCreated {
		const limit = 1 << 20
		body, err := io.ReadAll(io.LimitReader(resp.Body, limit+1))
		if err != nil || len(body) > limit {
			return resp.StatusCode, errors.New("invalid response size")
		}
		return resp.StatusCode, json.Unmarshal(body, out)
	}
	return resp.StatusCode, nil
}

func gatewayUnavailable() *toolProblem {
	return &toolProblem{Error: "unavailable", Message: "The admin server could not complete the request."}
}

func (g *gateway) read(ctx context.Context, route string, in, out any) *toolProblem {
	token, problem := g.credential(ctx)
	if problem != nil {
		return problem
	}
	status, err := g.post(ctx, route, token, in, out)
	if err != nil {
		return gatewayUnavailable()
	}
	switch status {
	case http.StatusOK:
		return nil
	case http.StatusUnauthorized:
		g.mu.Lock()
		if g.token == token {
			g.token = ""
		}
		g.mu.Unlock()
		return &toolProblem{Error: problemExpired, Message: "Access ended. Request a new connection to continue."}
	case http.StatusConflict:
		return &toolProblem{
			Error: "browser_refresh_required", VerificationURL: g.connectURL(),
			Message: "Ask the operator to open or refresh the admin page in the same browser session, then retry.",
		}
	case http.StatusForbidden:
		return &toolProblem{Error: problemDenied, Message: "Access to the resource is denied."}
	case http.StatusNotFound:
		return &toolProblem{Error: "not_found", Message: "The record was not found."}
	case http.StatusUnprocessableEntity, http.StatusBadRequest:
		return &toolProblem{Error: problemInvalidInput, Message: "The admin server rejected the input."}
	default:
		return gatewayUnavailable()
	}
}

func (g *gateway) credential(ctx context.Context) (string, *toolProblem) {
	g.mu.Lock()
	defer g.mu.Unlock()
	if !time.Now().Before(g.validUntil) {
		g.token, g.pending = "", gatewayPairing{}
	}
	if g.token != "" {
		return g.token, nil
	}
	if g.pending.DeviceSecret == "" {
		var p gatewayPairing
		status, err := g.post(ctx, "pair", "", struct{}{}, &p)
		if err != nil || (status != http.StatusOK && status != http.StatusCreated) {
			return "", gatewayUnavailable()
		}
		if p.VerificationURL != g.connectURL() || !codePattern.MatchString(p.Code) ||
			p.DeviceSecret == "" || p.ExpiresIn <= 0 || p.ExpiresIn > int(pairLifetime.Seconds()) {
			return "", gatewayUnavailable()
		}
		g.pending, g.validUntil = p, time.Now().Add(time.Duration(p.ExpiresIn)*time.Second)
		return "", p.instructions("authorization_required")
	}

	var grant struct {
		Token     string `json:"access_token"` //nolint:tagliatelle // Existing device protocol.
		ExpiresIn int    `json:"expires_in"`   //nolint:tagliatelle // Existing device protocol.
	}
	status, err := g.post(ctx, "token", "", map[string]string{fieldDeviceSecret: g.pending.DeviceSecret}, &grant)
	if err != nil {
		return "", gatewayUnavailable()
	}
	switch status {
	case http.StatusAccepted:
		return "", g.pending.instructions("authorization_pending")
	case http.StatusOK:
		g.pending = gatewayPairing{}
		if grant.Token == "" || grant.ExpiresIn <= 0 || grant.ExpiresIn > int(grantLifetime.Seconds()) {
			return "", gatewayUnavailable()
		}
		g.token, g.validUntil = grant.Token, time.Now().Add(time.Duration(grant.ExpiresIn)*time.Second)
		return g.token, nil
	case http.StatusForbidden:
		g.pending = gatewayPairing{}
		return "", &toolProblem{Error: problemDenied, Message: "The operator denied this connection request."}
	case http.StatusGone:
		g.pending = gatewayPairing{}
		return "", &toolProblem{
			Error: problemExpired, Message: "This connection request has expired. Request a new connection to continue.",
		}
	default:
		return "", gatewayUnavailable()
	}
}

func (g *gateway) connectURL() string { return g.origin.String() + g.path + "/connect" }

func (p gatewayPairing) instructions(code string) *toolProblem {
	return &toolProblem{
		Error: code, VerificationURL: p.VerificationURL, Code: p.Code,
		Message: "Ask the operator to open the verification URL, match the code and approve read access. Retry after approval.",
	}
}
