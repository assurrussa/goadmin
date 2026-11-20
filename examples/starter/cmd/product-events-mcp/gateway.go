package main

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"net"
	"net/http"
	"net/url"
	"strings"
	"sync"
	"time"
)

const agentPath = "/agent/product-events/"

type gateway struct {
	origin *url.URL
	client *http.Client

	// Only pairing/exchange are serialized. Reads may proceed concurrently.
	mu         sync.Mutex
	pending    pairing
	token      string
	validUntil time.Time
}

type pairing struct {
	Code            string `json:"code"`
	DeviceSecret    string `json:"device_secret"`
	VerificationURL string `json:"verification_url"`
	ExpiresIn       int    `json:"expires_in"`
}

func newGateway(origin string) (*gateway, error) {
	u, err := url.Parse(origin)
	if err != nil || u.Hostname() == "" || u.User != nil || u.Fragment != "" || u.RawQuery != "" ||
		u.ForceQuery || u.RawPath != "" || (u.Path != "" && u.Path != "/") {
		return nil, errors.New("provide the starter origin using -admin-url or GOADMIN_URL")
	}
	loopback := u.Hostname() == "localhost" || net.ParseIP(u.Hostname()).IsLoopback()
	if u.Scheme != "https" && !(u.Scheme == "http" && loopback) {
		return nil, errors.New("use HTTPS or a loopback HTTP origin")
	}
	u.Path = ""
	client := &http.Client{
		Timeout:       10 * time.Second,
		CheckRedirect: func(_ *http.Request, _ []*http.Request) error { return http.ErrUseLastResponse },
	}
	return &gateway{origin: u, client: client}, nil
}

// post contacts only a route selected by the tool implementation. Neither the
// upstream response body nor an HTTP error gets reflected in a tool error.
func (g *gateway) post(ctx context.Context, route, token string, in, out any) (int, error) {
	data, err := json.Marshal(in)
	if err != nil {
		return 0, err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, g.origin.String()+agentPath+route, bytes.NewReader(data))
	if err != nil {
		return 0, err
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "application/json")
	if token != "" {
		req.Header.Set("Authorization", "Bearer "+token)
	}
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
	return &toolProblem{Error: "unavailable", Message: "The starter could not complete the request."}
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
		return &toolProblem{Error: "authorization_expired", Message: "Access ended. Request a new connection to continue."}
	case http.StatusForbidden:
		return &toolProblem{Error: "denied", Message: "Access to the journal is denied."}
	case http.StatusNotFound:
		return &toolProblem{Error: "not_found", Message: "The event was not found."}
	case http.StatusUnprocessableEntity, http.StatusBadRequest:
		return &toolProblem{Error: "invalid_input", Message: "The starter rejected the input."}
	default:
		return gatewayUnavailable()
	}
}

func (g *gateway) credential(ctx context.Context) (string, *toolProblem) {
	g.mu.Lock()
	defer g.mu.Unlock()
	if !time.Now().Before(g.validUntil) {
		g.token, g.pending = "", pairing{}
	}
	if g.token != "" {
		return g.token, nil
	}
	if g.pending.DeviceSecret == "" {
		var p pairing
		status, err := g.post(ctx, "pair", "", struct{}{}, &p)
		if err != nil || (status != http.StatusOK && status != http.StatusCreated) {
			return "", gatewayUnavailable()
		}
		verify, err := g.origin.Parse(p.VerificationURL)
		if p.VerificationURL == "" || err != nil || verify.User != nil || verify.Scheme != g.origin.Scheme ||
			!strings.EqualFold(verify.Host, g.origin.Host) || verify.Fragment != "" ||
			p.Code == "" || p.DeviceSecret == "" || p.ExpiresIn <= 0 || p.ExpiresIn > 300 {
			return "", gatewayUnavailable()
		}
		p.VerificationURL = verify.String()
		g.pending, g.validUntil = p, time.Now().Add(time.Duration(p.ExpiresIn)*time.Second)
		return "", p.instructions("authorization_required")
	}

	var grant struct {
		Token     string `json:"access_token"`
		ExpiresIn int    `json:"expires_in"`
	}
	status, err := g.post(ctx, "token", "", map[string]string{"device_secret": g.pending.DeviceSecret}, &grant)
	if err != nil {
		return "", gatewayUnavailable()
	}
	switch status {
	case http.StatusAccepted:
		return "", g.pending.instructions("authorization_pending")
	case http.StatusOK:
		g.pending = pairing{}
		if grant.Token == "" || grant.ExpiresIn <= 0 || grant.ExpiresIn > 900 {
			return "", gatewayUnavailable()
		}
		g.token, g.validUntil = grant.Token, time.Now().Add(time.Duration(grant.ExpiresIn)*time.Second)
		return g.token, nil
	case http.StatusForbidden:
		g.pending = pairing{}
		return "", &toolProblem{Error: "denied", Message: "The operator denied this connection request."}
	case http.StatusGone:
		g.pending = pairing{}
		return "", &toolProblem{Error: "authorization_expired", Message: "This connection request has expired. Request a new connection to continue."}
	default:
		return "", gatewayUnavailable()
	}
}

func (p pairing) instructions(code string) *toolProblem {
	return &toolProblem{
		Error: code, VerificationURL: p.VerificationURL, Code: p.Code,
		Message: "Ask the operator to open the verification URL, match the code and approve journal access. Retry after approval.",
	}
}
