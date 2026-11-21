package mcp

import (
	"errors"
	"fmt"
	"slices"
	"strings"

	"github.com/gofiber/fiber/v3/extractors"

	"github.com/assurrussa/goadmin/host"
	integrationroles "github.com/assurrussa/goadmin/internal/auth"
)

type actionDescription struct {
	Propose    string      `json:"propose"`
	Status     string      `json:"status"`
	Parameters []Parameter `json:"parameters"`
	Fields     []Field     `json:"fields"`
}

func (a actionDescription) validate() error {
	if !namePattern.MatchString(a.Propose) || !namePattern.MatchString(a.Status) || a.Status == a.Propose {
		return errors.New("mcp: distinct proposal and status tool names are required")
	}
	seen := map[string]bool{fieldRequestID: true}
	for _, p := range a.Parameters {
		if seen[p.Name] || !p.valid() {
			return fmt.Errorf("mcp: invalid action parameter %q", p.Name)
		}
		seen[p.Name] = true
	}
	seen = map[string]bool{
		fieldRequestID: true, fieldStatus: true, fieldReviewURL: true, fieldExpiresAt: true,
		fieldError: true, "message": true, fieldCode: true, fieldVerificationURL: true,
	}
	for _, f := range a.Fields {
		if seen[f.Key] || !namePattern.MatchString(f.Key) || !hasDescription(f.Description) ||
			(f.Kind != Text && f.Kind != PositiveID) {
			return fmt.Errorf("mcp: invalid action result field %q", f.Key)
		}
		seen[f.Key] = true
	}
	return nil
}

type actionFeature struct {
	cfg    Config
	action Action
	key    string
}

func (a Action) describe() description {
	return description{Version: 2, Name: a.ProposeTool, Description: a.Description, Action: &actionDescription{
		Propose: a.ProposeTool, Status: a.StatusTool, Parameters: a.Parameters, Fields: a.ResultFields,
	}}
}

// ConfirmedModule registers one separately paired proposal and receipt tool.
// All actions in a host namespace share the operator-scoped request UUID ledger.
func ConfirmedModule(cfg Config, action Action) (host.Module, error) {
	origin, err := adminOrigin(cfg.Origin)
	if err != nil {
		return host.Module{}, err
	}
	cfg.Origin = origin.String()
	if cfg.TrustedProxy.IsValid() {
		proxy, ok := pairingIP(cfg.TrustedProxy.String())
		if !ok {
			return host.Module{}, errors.New("mcp: configure one trusted proxy IP address")
		}
		cfg.TrustedProxy = proxy
	}
	if !pathPattern.MatchString(action.Path) || !namePattern.MatchString(action.Namespace) || !namePattern.MatchString(action.Key) ||
		len(action.Namespace) > 100 || len(action.Key) > 100 || len(action.Version) > 100 ||
		!namePattern.MatchString(action.Version) || !hasDescription(action.Title) || !hasDescription(action.ConfirmLabel) ||
		len(action.Permissions) == 0 || action.Normalize == nil || action.Inspect == nil || action.Apply == nil {
		return host.Module{}, errors.New("mcp: incomplete confirmed action declaration")
	}
	seen := map[host.PermissionKey]bool{}
	for _, p := range action.Permissions {
		if p.IsZero() || integrationroles.ParsePermissionKey(p.String()) != p || seen[p] {
			return host.Module{}, errors.New("mcp: invalid or duplicate action permission")
		}
		seen[p] = true
	}
	if err := action.describe().validate(); err != nil {
		return host.Module{}, err
	}
	action.Parameters = slices.Clone(action.Parameters)
	for i := range action.Parameters {
		action.Parameters[i].Enum = slices.Clone(action.Parameters[i].Enum)
	}
	action.ResultFields = slices.Clone(action.ResultFields)
	action.Permissions = slices.Clone(action.Permissions)
	key := "mcp-action." + action.Namespace + "." + action.Key
	f := actionFeature{cfg: cfg, action: action, key: key}
	descriptor := host.ModuleDescriptor{Key: key, Requires: slices.Clone(action.Requires), RouteNamespaces: []string{action.Path}}
	return host.DefinedFeatureModule(descriptor, f), nil
}

func (f actionFeature) Descriptor() host.FeatureDescriptor { return host.FeatureDescriptor{Key: f.key} }

func (f actionFeature) Build(_ host.Menu) host.ExtensionBuilder {
	return func(app *host.App) (*host.Extension, error) {
		store := app.Unwrap().Session()
		if store == nil || store.Extractor.Source != extractors.SourceCookie || strings.TrimSpace(store.Extractor.Key) == "" {
			return nil, errors.New("mcp: confirmed actions require a browser session cookie")
		}
		origin, _ := adminOrigin(f.cfg.Origin)
		h := &actionHandler{
			app: app, action: f.action, origin: origin, sessionName: store.Extractor.Key, grants: newActionPairings(),
			store:  actionStore{tx: app.TxManager(), action: f.action},
			device: resourceHandler{originHost: origin.Host, trustedProxy: f.cfg.TrustedProxy},
		}
		return host.NewExtension().WithMiddlewares(h.nativeTransitions).
			WithPublicRegister(h.registerDevice).WithRegister(h.registerBrowser), nil
	}
}
