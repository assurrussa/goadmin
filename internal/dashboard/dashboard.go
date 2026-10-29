// Package dashboard owns dashboard registration, authorization snapshots and bounded loading.
package dashboard

import (
	"context"
	"errors"
	"fmt"
	"net/url"
	"regexp"
	"slices"
	"strconv"
	"strings"
	"sync"
	"time"
	"unicode"

	auth "github.com/assurrussa/goadmin/internal/auth"
)

const (
	MaxParallel     = 4
	ProviderTimeout = 2 * time.Second
)

// Text is a literal fallback with optional Russian and English translations.
type Text struct {
	Text string `json:"text"`
	RU   string `json:"ru,omitempty"`
	EN   string `json:"en,omitempty"`
}

type Actor struct {
	AdminID   int64
	SubjectID string
}
type Metric struct {
	Key   string `json:"key"`
	Label Text   `json:"label"`
	Value string `json:"value"`
}
type Status struct {
	Text Text   `json:"text"`
	Tone string `json:"tone"`
}
type Content struct {
	Metrics []Metric
	Status  *Status
}
type Link struct {
	Label         Text               `json:"label"`
	Href          string             `json:"href"`
	PermissionKey auth.PermissionKey `json:"-"`
}
type Card struct {
	Key           string
	Title         Text
	PermissionKey auth.PermissionKey
	Links         []Link
	Load          func(context.Context, Actor) (Content, error)
}
type View struct {
	Key       string   `json:"key"`
	Title     Text     `json:"title"`
	Links     []Link   `json:"links"`
	Metrics   []Metric `json:"metrics"`
	Status    *Status  `json:"status,omitempty"`
	State     string   `json:"state"`
	UpdatedAt string   `json:"updatedAt,omitempty"`
}
type Snapshot struct {
	Owner     string `json:"owner"`
	Cards     []View `json:"cards"`
	UpdatedAt string `json:"updatedAt,omitempty"`
}

// Registry is immutable after assembly. Provider callbacks remain borrowed.
type Registry struct{ cards []Card }

var stableKey = regexp.MustCompile(`^[a-zA-Z0-9][a-zA-Z0-9._-]{0,127}$`)

// New validates registrations. Only the built-in notification card has no module permission:
// its provider is always scoped to the authenticated admin.
func New(builtins, custom []Card) (*Registry, error) {
	cards := make([]Card, 0, len(builtins)+len(custom))
	seen := make(map[string]bool)
	for group, input := range [][]Card{builtins, custom} {
		for _, card := range input {
			if !stableKey.MatchString(card.Key) || (group == 1 && strings.HasPrefix(card.Key, "goadmin.")) {
				return nil, fmt.Errorf("invalid dashboard card key %q", card.Key)
			}
			if seen[card.Key] {
				return nil, fmt.Errorf("duplicate dashboard card key %q", card.Key)
			}
			seen[card.Key] = true
			if err := validateCard(card, group == 0 && card.Key == "goadmin.notifications"); err != nil {
				return nil, err
			}
			card.Links = slices.Clone(card.Links)
			cards = append(cards, card)
		}
	}
	return &Registry{cards: cards}, nil
}

func validateCard(card Card, withoutPermission bool) error {
	if card.Load == nil || strings.TrimSpace(card.Title.Text) == "" {
		return fmt.Errorf("dashboard card %q requires a title and loader", card.Key)
	}
	if !withoutPermission && !validPermission(card.PermissionKey) {
		return fmt.Errorf("dashboard card %q requires a valid permission", card.Key)
	}
	for _, link := range card.Links {
		if strings.TrimSpace(link.Label.Text) == "" || !SafeHref(link.Href) {
			return fmt.Errorf("dashboard card %q has an invalid link", card.Key)
		}
		if link.PermissionKey != (auth.PermissionKey{}) && !validPermission(link.PermissionKey) {
			return fmt.Errorf("dashboard card %q has an invalid link permission", card.Key)
		}
	}
	return nil
}

func validPermission(key auth.PermissionKey) bool {
	return !key.IsZero() && auth.ParsePermissionKey(key.String()) == key
}

// SafeHref permits only internal root-relative URLs, including safe query/fragment suffixes.
func SafeHref(href string) bool {
	if !strings.HasPrefix(href, "/") || strings.HasPrefix(href, "//") {
		return false
	}
	decoded, err := url.PathUnescape(href)
	if err != nil {
		return false
	}
	if strings.HasPrefix(decoded, "//") || strings.Contains(decoded, "\\") || strings.ContainsFunc(decoded, unicode.IsControl) {
		return false
	}
	parsed, err := url.Parse(href)
	return err == nil && parsed.Host == "" && parsed.Scheme == "" && parsed.User == nil
}

// Request contains only copied identity and already authorized descriptors.
// Authorize must run sequentially on the request goroutine, before Load.
type Request struct {
	actor Actor
	cards []Card
}

func (r *Registry) Authorize(actor Actor, allowed func(auth.PermissionKey) bool) Request {
	request := Request{actor: actor}
	if r == nil || actor.AdminID <= 0 || actor.SubjectID == "" {
		return request
	}
	for _, registered := range r.cards {
		if !registered.PermissionKey.IsZero() && (allowed == nil || !allowed(registered.PermissionKey)) {
			continue
		}
		card := registered
		card.Links = make([]Link, 0, len(registered.Links))
		for _, link := range registered.Links {
			// The card gate was already checked: a link's permission can only narrow it.
			if link.PermissionKey == (auth.PermissionKey{}) || (allowed != nil && allowed(link.PermissionKey)) {
				card.Links = append(card.Links, link)
			}
		}
		request.cards = append(request.cards, card)
	}
	return request
}

func (r Request) Descriptors() Snapshot {
	snapshot := Snapshot{Owner: strconv.FormatInt(r.actor.AdminID, 10), Cards: make([]View, len(r.cards))}
	for i, card := range r.cards {
		snapshot.Cards[i] = View{
			Key: card.Key, Title: card.Title, Links: append([]Link{}, card.Links...),
			Metrics: []Metric{}, State: "loading",
		}
	}
	return snapshot
}

// Load joins a fixed-size worker pool. Callbacks must honor their standard context;
// no Fiber request context is retained or shared with workers.
func (r Request) Load(ctx context.Context, onError func(context.Context, string, error)) Snapshot {
	return r.load(ctx, ProviderTimeout, onError)
}

func (r Request) load(ctx context.Context, timeout time.Duration, onError func(context.Context, string, error)) Snapshot {
	snapshot := r.Descriptors()
	var workers sync.WaitGroup
	jobs := make(chan int)
	for range min(MaxParallel, len(r.cards)) {
		workers.Go(func() {
			for i := range jobs {
				card := r.cards[i]
				providerCtx, cancel := context.WithTimeout(ctx, timeout)
				var content Content
				err := providerCtx.Err()
				if err == nil {
					content, err = invoke(providerCtx, card.Load, r.actor)
				}
				if ctxErr := providerCtx.Err(); ctxErr != nil {
					err = ctxErr
				}
				if err == nil {
					err = validateContent(content)
				}
				cancel()
				view := &snapshot.Cards[i]
				if err != nil {
					view.State = "unavailable"
					if onError != nil {
						onError(ctx, card.Key, err)
					}
					continue
				}
				view.State = "ready"
				view.Metrics = append([]Metric{}, content.Metrics...)
				if content.Status != nil {
					status := *content.Status
					view.Status = &status
				}
				view.UpdatedAt = time.Now().UTC().Format(time.RFC3339)
			}
		})
	}
	for i := range r.cards {
		jobs <- i
	}
	close(jobs)
	workers.Wait()
	snapshot.UpdatedAt = time.Now().UTC().Format(time.RFC3339)
	return snapshot
}

func validateContent(content Content) error {
	seen := make(map[string]bool, len(content.Metrics))
	for _, metric := range content.Metrics {
		if !stableKey.MatchString(metric.Key) || seen[metric.Key] || strings.TrimSpace(metric.Label.Text) == "" {
			return errors.New("invalid dashboard metric")
		}
		seen[metric.Key] = true
	}
	if content.Status != nil {
		if strings.TrimSpace(content.Status.Text.Text) == "" {
			return errors.New("invalid dashboard status")
		}
		switch content.Status.Tone {
		case "neutral", "info", "success", "warning", "error":
		default:
			return errors.New("invalid dashboard status tone")
		}
	}
	return nil
}

// invoke isolates host callbacks from the HTTP process. Panic payloads may contain
// secrets and are deliberately discarded rather than logged or returned.
func invoke(ctx context.Context, load func(context.Context, Actor) (Content, error), actor Actor) (content Content, err error) {
	defer func() {
		if recover() != nil {
			content = Content{}
			err = errors.New("dashboard provider panicked")
		}
	}()
	return load(ctx, actor)
}
