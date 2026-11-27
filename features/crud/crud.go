// Package crud builds declarative, actor-scoped admin resources. Business
// callbacks own transactions, atomic revision checks, domain scope and audit.
package crud

import (
	"context"
	"errors"

	"github.com/assurrussa/goadmin/host"
	"github.com/assurrussa/goadmin/toolkit/datagrid"
)

const (
	revisionKey         = "_revision"
	formKey             = "_form"
	invalidFieldMessage = "Invalid field value"
)

type Operation string

const (
	Read   Operation = "read"
	Create Operation = "create"
	Update Operation = "update"
	Delete Operation = "delete"
)

// Record carries an opaque stable identity and an opaque concurrency token.
type Record[T any] struct {
	ID       string `json:"id"`
	Revision string `json:"revision"`
	Data     T      `json:"data"`
}
type Option struct {
	Value string `json:"value"`
	Label string `json:"label"`
}

// Field explicitly opts a JSON key into each supported mode. Undeclared and
// unreadable row fields are never sent to the browser or exported.
type Field struct {
	Key         string `json:"key"`
	Label       string `json:"label"`
	Description string `json:"description,omitempty"`
	Type        string `json:"type"`
	Read        bool   `json:"read"`
	Create      bool   `json:"create"`
	Update      bool   `json:"update"`
	Required    bool   `json:"required"`
	// MaxLength counts Unicode code points in writable text and textarea fields.
	MaxLength *int `json:"maxLength,omitempty"`
	// Minimum is an inclusive bound on writable numbers. A nil bound is absent.
	Minimum   *float64 `json:"minimum,omitempty"`
	Options   []Option `json:"options,omitempty"`
	Component string   `json:"component,omitempty"`
}

// ActionPlacement controls where an authorized action is offered.
type ActionPlacement string

const (
	Row    ActionPlacement = "row"    // Offer the action on list rows.
	Detail ActionPlacement = "detail" // Offer the action on the detail page.
	Both   ActionPlacement = "both"   // Offer the action in both places. The zero value does the same.
)

func (p ActionPlacement) includes(location ActionPlacement) bool {
	return p == "" || p == Both || p == location
}

type (
	Permissions   struct{ Read, Create, Update, Delete host.PermissionKey }
	Action[T any] struct {
		Key          string                                                  `json:"key"`
		Label        string                                                  `json:"label"`
		Description  string                                                  `json:"description,omitempty"`
		Confirmation string                                                  `json:"confirmation,omitempty"`
		Placement    ActionPlacement                                         `json:"placement,omitempty"`
		Params       []ActionParam                                           `json:"params,omitempty"`
		Permission   host.PermissionKey                                      `json:"-"`
		Run          func(context.Context, host.Actor, string, string) error `json:"-"`
		input        *actionInput
	}
)

// Definition callbacks must use the supplied actor for every lookup, including
// selected exports. Update/Delete/Action must compare revisions atomically in
// the same transaction as the write and audit. Authorize is a second server-side
// record gate; its presence does not make a tenant-unscoped repository safe.
type Definition[R, C, U any] struct {
	Key, BasePath, Title      string
	Description               string
	Menu                      host.Menu
	Permissions               Permissions
	Fields                    []Field
	Columns                   []datagrid.Column
	ListColumns               []ListColumn
	DefaultSort, DefaultOrder string
	PageSize                  int
	ClientRequirements        []host.ClientExtensionRequirement
	List                      func(context.Context, host.Actor, datagrid.Filtered) ([]Record[R], int, error)
	Get                       func(context.Context, host.Actor, string) (Record[R], error)
	LoadSelected              func(context.Context, host.Actor, []string) ([]Record[R], error)
	Create                    func(context.Context, host.Actor, C) (Record[R], error)
	Update                    func(context.Context, host.Actor, string, string, U) (Record[R], error)
	Delete                    func(context.Context, host.Actor, string, string) error
	Authorize                 func(context.Context, host.Actor, Operation, Record[R]) error
	Actions                   []Action[R]
}

var (
	ErrNotFound = errors.New("record not found")
	ErrDenied   = errors.New("record access denied")
	ErrUnknown  = errors.New("result unknown; check record before retrying")
	ErrConflict = errors.New("record changed; reload before saving")
)

// FieldErrors contains safe, user-facing validation messages. Never put database
// or infrastructure errors into this map; all other errors are sanitized.
type FieldErrors map[string][]string //nolint:errname // Public field-error map API.

func (FieldErrors) Error() string { return "invalid resource fields" }

// New validates the complete resource definition without I/O.
func New[R, C, U any](d Definition[R, C, U]) (host.Module, error) {
	return newModule(d, editableResource, nil)
}

type resourceMode uint8

const (
	editableResource resourceMode = iota
	readOnlyResource
	commandResource
)

func newModule[R, C, U any](d Definition[R, C, U], mode resourceMode, adapter *ReadOnlyAdapter) (host.Module, error) {
	if len(d.ListColumns) != 0 {
		if len(d.Columns) != 0 {
			return host.Module{}, errors.New("crud: declare either Columns or ListColumns")
		}
		var err error
		d.Columns, err = deriveColumns(d.Fields, d.ListColumns)
		if err != nil {
			return host.Module{}, err
		}
		d.ListColumns = nil
	}
	if err := validate(d, mode); err != nil {
		return host.Module{}, err
	}
	d = cloneDefinition(d)
	f := feature[R, C, U]{d: d, mode: mode, adapter: adapter}
	namespaces := []string{d.BasePath}
	if adapter != nil {
		namespaces = append(namespaces, adapter.BasePath)
	}
	return host.DefinedFeatureModule(host.ModuleDescriptor{Key: "crud." + d.Key, RouteNamespaces: namespaces}, f), nil
}

type feature[R, C, U any] struct {
	d       Definition[R, C, U]
	mode    resourceMode
	adapter *ReadOnlyAdapter
}

func (f feature[R, C, U]) Descriptor() host.FeatureDescriptor {
	p := f.d.Permissions
	keys := make([]host.PermissionKey, 0, 4+len(f.d.Actions))
	keys = append(keys, p.Read)
	if f.mode == editableResource {
		keys = append(keys, p.Create, p.Update, p.Delete)
	}
	for _, a := range f.d.Actions {
		keys = append(keys, a.Permission)
	}
	requirements := append([]host.ClientExtensionRequirement{{
		Key:         "goadmin.crud",
		APIVersion:  1,
		Fingerprint: ClientFingerprint,
	}},
		f.d.ClientRequirements...)
	return host.FeatureDescriptor{Key: "crud." + f.d.Key, Menu: f.d.Menu, Permissions: keys, ClientReferences: requirements}
}

func (f feature[R, C, U]) Build(host.Menu) host.ExtensionBuilder {
	return func(app *host.App) (*host.Extension, error) {
		if app == nil || app.Unwrap() == nil {
			return nil, errors.New("crud: admin app required")
		}
		h := &handler[R, C, U]{app: app, d: f.d, mode: f.mode}
		extension := host.NewExtension()
		if f.adapter != nil {
			var err error
			extension, err = f.adapter.Build(app, h.reader())
			if err != nil {
				return nil, err
			}
			if extension == nil {
				return nil, errors.New("crud: read adapter returned no extension")
			}
		}
		return extension.WithHandlers(h), nil
	}
}
