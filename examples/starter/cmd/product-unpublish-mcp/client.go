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
	"regexp"
	"sync"
	"time"
)

const actionPath = "/agent/product-unpublish"

var pairingCode = regexp.MustCompile("^[A-Z2-7]{5}-[A-Z2-7]{5}$")

type client struct {
	base    string
	http    *http.Client
	mu      sync.Mutex // protects the device exchange and in-memory credentials
	pending device
	token   string
	until   time.Time
}

type device struct {
	Code    string `json:"code"`
	Secret  string `json:"device_secret"`
	URL     string `json:"verification_url"`
	Seconds int    `json:"expires_in"`
}

func newClient(raw string) (*client, error) {
	u, err := url.Parse(raw)
	if err != nil || u.Hostname() == "" || u.User != nil || u.RawQuery != "" || u.ForceQuery || u.Fragment != "" || u.RawPath != "" || (u.Path != "/" && u.Path != "") {
		return nil, errors.New("provide the starter admin origin using -admin-url")
	}
	loopback := u.Hostname() == "localhost" || net.ParseIP(u.Hostname()).IsLoopback()
	if u.Scheme != "https" && !(u.Scheme == "http" && loopback) {
		return nil, errors.New("use HTTPS or a loopback HTTP origin")
	}
	u.Path = actionPath
	return &client{base: u.String(), http: &http.Client{
		Timeout: 10 * time.Second, CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse },
	}}, nil
}

func (g *client) post(ctx context.Context, route, bearer string, in, dest any) (int, error) {
	body, err := json.Marshal(in)
	if err != nil {
		return 0, err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, g.base+"/"+route, bytes.NewReader(body))
	if err != nil {
		return 0, err
	}
	req.Header.Set("Content-Type", "application/json")
	if bearer != "" {
		req.Header.Set("Authorization", "Bearer "+bearer)
	}
	resp, err := g.http.Do(req)
	if err != nil {
		return 0, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK && resp.StatusCode != http.StatusCreated {
		return resp.StatusCode, nil
	}
	const limit = 16 << 10
	data, err := io.ReadAll(io.LimitReader(resp.Body, limit+1))
	if err != nil || len(data) > limit {
		return 0, errors.New("invalid response")
	}
	return resp.StatusCode, json.Unmarshal(data, dest)
}

func (g *client) credential(ctx context.Context) (string, outcome) {
	g.mu.Lock()
	defer g.mu.Unlock()
	if !time.Now().Before(g.until) {
		g.token, g.pending = "", device{}
	}
	if g.token != "" {
		return g.token, outcome{}
	}
	if g.pending.Secret == "" {
		var d device
		status, err := g.post(ctx, "pair", "", struct{}{}, &d)
		if status == http.StatusTooManyRequests {
			return "", problem("rate_limited", "Too many pending connections. Retry later.")
		}
		if err != nil || (status != 200 && status != 201) || d.Secret == "" || d.URL != g.base+"/connect" || !pairingCode.MatchString(d.Code) || d.Seconds <= 0 || d.Seconds > 300 {
			return "", problem("unavailable", "Could not start the connection.")
		}
		g.pending, g.until = d, time.Now().Add(time.Duration(d.Seconds)*time.Second)
		return "", d.instructions("authorization_required")
	}
	var grant struct {
		Token   string `json:"access_token"`
		Seconds int    `json:"expires_in"`
	}
	status, err := g.post(ctx, "token", "", map[string]string{"device_secret": g.pending.Secret}, &grant)
	if err != nil {
		return "", problem("unavailable", "Could not check the connection.")
	}
	switch status {
	case http.StatusAccepted:
		return "", g.pending.instructions("authorization_pending")
	case http.StatusOK:
		g.pending = device{}
		if grant.Token != "" && grant.Seconds > 0 && grant.Seconds <= 900 {
			g.token, g.until = grant.Token, time.Now().Add(time.Duration(grant.Seconds)*time.Second)
			return g.token, outcome{}
		}
	case http.StatusForbidden, http.StatusGone:
		g.pending = device{}
		return "", problem("authorization_ended", "The operator declined the connection or it expired.")
	}
	return "", problem("unavailable", "Could not complete the connection.")
}

func (d device) instructions(code string) outcome {
	return outcome{Error: code, Message: "Ask the operator to enter this code on the verification page. Retry the same request after connecting.", VerificationURL: d.URL, Code: d.Code}
}

func (g *client) call(ctx context.Context, route, key string, in any) outcome {
	token, failure := g.credential(ctx)
	if token == "" {
		return failure
	}
	var receipt struct {
		RequestID string `json:"request_id"`
		Status    string `json:"status"`
		ReviewURL string `json:"review_url"`
		EventID   string `json:"event_id"`
	}
	status, err := g.post(ctx, route, token, in, &receipt)
	unknown := problem("outcome_unknown", "Check get_unpublish_request with the original request_id when the server is available. Do not generate a new key to retry.")
	if err != nil {
		return unknown
	}
	switch status {
	case http.StatusOK, http.StatusCreated:
		if receipt.RequestID != key {
			return unknown
		}
		out := outcome{RequestID: key, Status: receipt.Status}
		switch out.Status {
		case "pending":
			if receipt.ReviewURL != g.base+"/requests/"+key {
				return unknown
			}
			out.ReviewURL = receipt.ReviewURL
		case "succeeded":
			if !validID(receipt.EventID) {
				return unknown
			}
			out.EventID = receipt.EventID
		case "declined", "expired", "conflict":
		default:
			return unknown
		}
		return out
	case http.StatusUnauthorized:
		g.mu.Lock()
		if g.token == token {
			g.token = ""
		}
		g.mu.Unlock()
		return problem("authorization_expired", "Connect again to look up your prior request.")
	case http.StatusForbidden:
		return problem("denied", "The operator does not have access.")
	case http.StatusNotFound:
		return problem("not_found", "No matching product or request was found.")
	case http.StatusConflict:
		return problem("conflict", "The original proposal or product state conflicts with this request.")
	case http.StatusUnprocessableEntity, http.StatusBadRequest:
		return problem("invalid_input", "Check the product ID, request ID and reason.")
	case http.StatusTooManyRequests:
		return problem("rate_limited", "Retry later using the original request ID.")
	default:
		return unknown
	}
}
