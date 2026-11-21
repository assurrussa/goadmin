// Package mcp exposes explicitly configured read-only admin resources to local MCP clients.
package mcp

import (
	"context"
	"errors"
	"fmt"
	"net"
	"net/netip"
	"net/url"
	"regexp"
	"slices"
	"strings"

	"github.com/assurrussa/goadmin/features/crud"
	"github.com/assurrussa/goadmin/host"
	"github.com/assurrussa/goadmin/toolkit/datagrid"
)

const approvalPage = "agent/ReadAccess"

// Config fixes the admin origin and the optional pairing proxy. The proxy must
// be the direct socket peer and must overwrite X-Real-IP with one address.
type Config struct {
	Origin       string
	TrustedProxy netip.Addr
}

// Resource declares the exact web namespace, operator permission, tools and
// fields exposed to a local MCP client. A nil operation is disabled.
type Resource struct {
	Path                string
	Permission          host.PermissionKey
	Name                string
	Description         string
	RecordIDDescription string
	Search              *Search
	Detail              *Detail
	Consent             Consent

	// AllowExternalSessions permits a supported AuthHub browser session to
	// approve a grant. Native browser sessions work without this option.
	AllowExternalSessions bool
}

// Consent supplies the browser page copy and optional link to the read UI.
type Consent struct {
	Title, Description, BackHref, BackText string
}

// Search exposes a paginated actor-scoped list. Query maps to the DataGrid
// search box. Filters map to DataGrid field filters by their FilterKey.
type Search struct {
	ToolName    string     `json:"toolName"`
	Description string     `json:"description"`
	Query       *Parameter `json:"query,omitempty"`
	Filters     []Filter   `json:"filters,omitempty"`
	Fields      []Field    `json:"fields"`
}

// Detail exposes an actor-scoped lookup by record ID.
type Detail struct {
	ToolName    string    `json:"toolName"`
	Description string    `json:"description"`
	ID          Parameter `json:"id"`
	Fields      []Field   `json:"fields"`
}

// Kind describes a scalar JSON value. PositiveID is a canonical base-10
// positive int64 encoded as a string.
type Kind string

const (
	Text       Kind = "string"
	Integer    Kind = "integer"
	Number     Kind = "number"
	Boolean    Kind = "boolean"
	PositiveID Kind = "positive_id"
)

// Parameter documents and validates an input field.
type Parameter struct {
	Name        string   `json:"name"`
	Description string   `json:"description"`
	Kind        Kind     `json:"kind"`
	Required    bool     `json:"required,omitempty"`
	Enum        []string `json:"enum,omitempty"`
}

// Filter maps a tool parameter to a supported field filter.
type Filter struct {
	Parameter
	FilterKey string `json:"filterKey"`
}

// Field is an allowlisted property of the reader's Data map. Nullable fields
// emit JSON null if absent. The reader's separate ID is always exposed as id.
type Field struct {
	Key         string `json:"key"`
	Description string `json:"description"`
	Kind        Kind   `json:"kind"`
	Nullable    bool   `json:"nullable,omitempty"`
}

// Reader uses the same scoped read contract as a declarative CRUD resource.
type Reader = crud.ReadOnlyReader

// Record contains a reader's ID and projected field values.
type Record = crud.ReadOnlyRecord[map[string]any]

// ReadFuncs adapts custom actor-scoped reads. List must scope both rows and the
// total. Get must scope the lookup and enforce any record-level policy.
type ReadFuncs struct {
	ListFunc func(context.Context, host.Actor, datagrid.Filters) ([]Record, int, error)
	GetFunc  func(context.Context, host.Actor, string) (Record, error)
}

func (r ReadFuncs) List(ctx context.Context, actor host.Actor, filters datagrid.Filters) ([]Record, int, error) {
	return r.ListFunc(ctx, actor, filters)
}

func (r ReadFuncs) Get(ctx context.Context, actor host.Actor, id string) (Record, error) {
	return r.GetFunc(ctx, actor, id)
}

// Adapter mounts MCP on the same actor-scoped reader used by CRUD's web UI.
func Adapter(cfg Config, resource Resource) (crud.ReadOnlyAdapter, error) {
	cfg, resource, err := validate(cfg, resource)
	if err != nil {
		return crud.ReadOnlyAdapter{}, err
	}
	return crud.ReadOnlyAdapter{
		BasePath: resource.Path,
		Build: func(app *host.App, reader crud.ReadOnlyReader) (*host.Extension, error) {
			return newHandler(app, cfg, resource, reader).extension(), nil
		},
	}, nil
}

// Module mounts MCP for a custom reader, including a host's DataGrid. The
// permission is shared with the existing resource and must be declared there.
func Module(cfg Config, resource Resource, reader Reader) (host.Module, error) {
	cfg, resource, err := validate(cfg, resource)
	if err != nil {
		return host.Module{}, err
	}
	if callbacks, ok := reader.(*ReadFuncs); ok {
		if callbacks == nil {
			return host.Module{}, errors.New("mcp: a scoped reader is required")
		}
		reader = *callbacks
	}
	if reader == nil {
		return host.Module{}, errors.New("mcp: a scoped reader is required")
	}
	if funcs, ok := reader.(ReadFuncs); ok &&
		(resource.Search != nil && funcs.ListFunc == nil || resource.Detail != nil && funcs.GetFunc == nil) {
		return host.Module{}, errors.New("mcp: each enabled operation requires its scoped callback")
	}
	key := "mcp" + strings.ReplaceAll(resource.Path, "/", ".")
	feature := resourceFeature{key: key, cfg: cfg, resource: resource, reader: reader}
	return host.DefinedFeatureModule(host.ModuleDescriptor{Key: key, RouteNamespaces: []string{resource.Path}}, feature), nil
}

type resourceFeature struct {
	key      string
	cfg      Config
	resource Resource
	reader   Reader
}

func (f resourceFeature) Descriptor() host.FeatureDescriptor {
	return host.FeatureDescriptor{Key: f.key}
}

func (f resourceFeature) Build(_ host.Menu) host.ExtensionBuilder {
	return func(app *host.App) (*host.Extension, error) {
		return newHandler(app, f.cfg, f.resource, f.reader).extension(), nil
	}
}

var (
	pathPattern = regexp.MustCompile(`^(/[a-z][a-z0-9_-]*)+$`)
	namePattern = regexp.MustCompile(`^[A-Za-z0-9_-]{1,128}$`)
)

func adminOrigin(raw string) (*url.URL, error) {
	u, err := url.Parse(raw)
	if err != nil || u.Hostname() == "" || u.User != nil || u.Fragment != "" || u.RawQuery != "" ||
		u.ForceQuery || u.RawPath != "" || (u.Path != "" && u.Path != "/") {
		return nil, errors.New("mcp: provide an absolute admin origin")
	}
	loopback := u.Hostname() == "localhost" || net.ParseIP(u.Hostname()).IsLoopback()
	if u.Scheme != "https" && (u.Scheme != "http" || !loopback) {
		return nil, errors.New("mcp: use HTTPS or a loopback HTTP origin")
	}
	u.Path = ""
	return u, nil
}

func validate(cfg Config, resource Resource) (Config, Resource, error) {
	origin, err := adminOrigin(cfg.Origin)
	if err != nil {
		return cfg, resource, err
	}
	cfg.Origin = origin.String()
	if cfg.TrustedProxy.IsValid() {
		proxy, ok := pairingIP(cfg.TrustedProxy.String())
		if !ok {
			return cfg, resource, errors.New("mcp: configure one trusted proxy IP address")
		}
		cfg.TrustedProxy = proxy
	}
	if !pathPattern.MatchString(resource.Path) || resource.Permission.IsZero() {
		return cfg, resource, errors.New("mcp: a literal route namespace and operator permission are required")
	}
	if consent := resource.Consent; (consent.BackHref == "") != (consent.BackText == "") ||
		consent.BackHref != "" && !pathPattern.MatchString(consent.BackHref) {
		return cfg, resource, errors.New("mcp: the consent link requires a local admin path and label")
	}
	resource = resource.snapshot()
	return cfg, resource, resource.description().validate()
}

func (r Resource) snapshot() Resource {
	if r.Search != nil {
		search := *r.Search
		search.Filters = slices.Clone(search.Filters)
		for i := range search.Filters {
			search.Filters[i].Enum = slices.Clone(search.Filters[i].Enum)
		}
		search.Fields = slices.Clone(search.Fields)
		if search.Query != nil {
			query := *search.Query
			query.Enum = slices.Clone(query.Enum)
			search.Query = &query
		}
		r.Search = &search
	}
	if r.Detail != nil {
		detail := *r.Detail
		detail.Fields = slices.Clone(detail.Fields)
		detail.ID.Required = true
		detail.ID.Enum = slices.Clone(detail.ID.Enum)
		r.Detail = &detail
	}
	return r
}

type description struct {
	Version     int     `json:"version"`
	Name        string  `json:"name"`
	Description string  `json:"description"`
	ID          string  `json:"recordIdDescription"`
	Search      *Search `json:"search,omitempty"`
	Detail      *Detail `json:"detail,omitempty"`
}

func (r Resource) description() description {
	return description{
		Version: 1, Name: r.Name, Description: r.Description,
		ID: r.RecordIDDescription, Search: r.Search, Detail: r.Detail,
	}
}

func (d description) validate() error {
	if d.Version != 1 || !namePattern.MatchString(d.Name) || !hasDescription(d.Description) || !hasDescription(d.ID) ||
		(d.Search == nil && d.Detail == nil) {
		return errors.New("mcp: a named resource, description, record ID description and at least one read operation are required")
	}
	if d.Search != nil {
		if err := d.Search.validate(); err != nil {
			return err
		}
	}
	if detail := d.Detail; detail != nil {
		if err := validateTool(detail.ToolName, detail.Description, detail.Fields); err != nil {
			return err
		}
		if !detail.ID.valid() || !detail.ID.Required || (detail.ID.Kind != Text && detail.ID.Kind != PositiveID) {
			return errors.New("mcp: invalid detail ID parameter")
		}
		if d.Search != nil && d.Search.ToolName == detail.ToolName {
			return errors.New("mcp: search and detail tool names must be distinct")
		}
	}
	return nil
}

func (s Search) validate() error {
	if err := validateTool(s.ToolName, s.Description, s.Fields); err != nil {
		return err
	}
	inputs := map[string]bool{"page": true, "page_size": true}
	if s.Query != nil {
		if !s.Query.valid() || s.Query.Kind != Text || inputs[s.Query.Name] {
			return errors.New("mcp: invalid search query parameter")
		}
		inputs[s.Query.Name] = true
	}
	keys := make(map[string]bool, len(s.Filters))
	for _, filter := range s.Filters {
		if !filter.valid() || !namePattern.MatchString(filter.FilterKey) || inputs[filter.Name] || keys[filter.FilterKey] {
			return fmt.Errorf("mcp: invalid or duplicate search filter %q", filter.Name)
		}
		inputs[filter.Name], keys[filter.FilterKey] = true, true
	}
	return nil
}

func validateTool(name, description string, fields []Field) error {
	if !namePattern.MatchString(name) || !hasDescription(description) {
		return errors.New("mcp: each operation requires a tool name and description")
	}
	keys := map[string]bool{"id": true}
	for _, f := range fields {
		if !namePattern.MatchString(f.Key) || keys[f.Key] || !hasDescription(f.Description) || !f.Kind.valid() {
			return fmt.Errorf("mcp: invalid or duplicate projected field %q", f.Key)
		}
		keys[f.Key] = true
	}
	return nil
}

func (p Parameter) valid() bool {
	if !namePattern.MatchString(p.Name) || !hasDescription(p.Description) || !p.Kind.valid() {
		return false
	}
	if len(p.Enum) != 0 && p.Kind != Text && p.Kind != PositiveID {
		return false
	}
	seen := make(map[string]bool, len(p.Enum))
	for _, option := range p.Enum {
		if seen[option] || p.Kind == PositiveID && !validPositiveID(option) {
			return false
		}
		seen[option] = true
	}
	return true
}

func (k Kind) valid() bool {
	switch k {
	case Text, Integer, Number, Boolean, PositiveID:
		return true
	default:
		return false
	}
}

func hasDescription(value string) bool {
	return strings.TrimSpace(value) != ""
}
