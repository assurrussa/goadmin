package gridworkflow

import (
	"bytes"
	"context"
	"encoding/csv"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"slices"
	"strconv"
	"strings"

	"github.com/gofiber/fiber/v3"

	"github.com/assurrussa/goadmin/toolkit/datagrid"
)

const (
	MaxSelected   = 100
	MaxExportRows = 10000
)

type Options[T any] struct {
	Key          string
	Store        Store
	CurrentAdmin func(fiber.Ctx) (int64, error)
	SelectionID  func(T) string
	LoadSelected func(context.Context, []string) ([]T, error)
	BulkURL      string
	BulkActions  []datagrid.WorkflowAction
}

// Register mounts routes on an already authorized resource group. No I/O occurs
// during construction. Session/CSRF middleware and the resource read permission
// must protect the supplied router, including PUT preferences.
func Register[T any](router fiber.Router, grid *datagrid.Handler[T], options Options[T]) error {
	if router == nil || grid == nil || options.Store == nil || options.CurrentAdmin == nil || options.SelectionID == nil {
		return errors.New("invalid table workflow configuration")
	}
	if !namespacePattern.MatchString(options.Key) {
		return errors.New("invalid table workflow namespace")
	}
	cfg := grid.GetConfig()
	if cfg.Repository == nil || len(cfg.Columns) == 0 || !strings.HasPrefix(cfg.RoutePath, "/") {
		return errors.New("table workflow requires repository, columns and route path")
	}
	grid.ConfigureWorkflow(&datagrid.WorkflowConfig{
		Key:            options.Key,
		PreferencesURL: cfg.RoutePath + "/table-preferences",
		ExportURL:      cfg.RoutePath + "/table-export",
		BulkURL:        options.BulkURL,
		BulkActions:    options.BulkActions,
	}, options.SelectionID)
	w := workflow[T]{grid: grid, config: cfg, options: options, schema: newSchema(cfg)}
	router.Get("/table-preferences", APIHandler(w.getPreferences))
	router.Put("/table-preferences", APIHandler(w.putPreferences))
	router.Get("/table-export", APIHandler(w.export))
	return nil
}

type workflow[T any] struct {
	grid    *datagrid.Handler[T]
	config  datagrid.FilterConfig[T]
	options Options[T]
	schema  schema
}

func (w workflow[T]) owner(c fiber.Ctx) (int64, error) {
	c.Set("Cache-Control", "no-store")
	id, err := w.options.CurrentAdmin(c)
	if err != nil || id <= 0 {
		return 0, fiber.NewError(fiber.StatusUnauthorized)
	}
	return id, nil
}

func (w workflow[T]) getPreferences(c fiber.Ctx) error {
	id, err := w.owner(c)
	if err != nil {
		return err
	}
	p, version, err := w.options.Store.Load(c, id, w.options.Key)
	if err != nil {
		return fiber.NewError(fiber.StatusInternalServerError, "Unable to load table preferences")
	}
	if version == 0 {
		p = w.schema.defaultsPayload()
	} else {
		p = w.schema.reconcile(p)
	}
	return c.JSON(Preferences{Owner: strconv.FormatInt(id, 10), Version: version, Payload: p})
}

func (w workflow[T]) putPreferences(c fiber.Ctx) error {
	id, err := w.owner(c)
	if err != nil {
		return err
	}
	if len(c.Body()) > MaxPayloadBytes {
		return fiber.NewError(fiber.StatusRequestEntityTooLarge, "Preferences exceed 64 KiB")
	}
	var input struct {
		Version *int64  `json:"version"`
		Payload Payload `json:"payload"`
	}
	decoder := json.NewDecoder(bytes.NewReader(c.Body()))
	decoder.DisallowUnknownFields()
	if err = decoder.Decode(&input); err != nil {
		return fiber.NewError(fiber.StatusBadRequest, "Invalid preferences body")
	}
	if err = decoder.Decode(new(any)); !errors.Is(err, io.EOF) {
		return fiber.NewError(fiber.StatusBadRequest, "Invalid preferences body")
	}
	if input.Version == nil || *input.Version < 0 || *input.Version >= 9007199254740991 {
		return fiber.NewError(fiber.StatusBadRequest, "Invalid preferences version")
	}
	if err = w.schema.validate(input.Payload); err != nil {
		return fiber.NewError(fiber.StatusBadRequest, err.Error())
	}
	input.Payload = w.schema.reconcile(input.Payload)
	version, err := w.options.Store.Save(c, id, w.options.Key, *input.Version, input.Payload)
	if errors.Is(err, ErrConflict) {
		return fiber.NewError(fiber.StatusConflict, ErrConflict.Error())
	}
	if err != nil {
		return fiber.NewError(fiber.StatusInternalServerError, "Unable to save table preferences")
	}
	return c.JSON(Preferences{Owner: strconv.FormatInt(id, 10), Version: version, Payload: input.Payload})
}

func (w workflow[T]) export(c fiber.Ctx) error {
	if _, err := w.owner(c); err != nil {
		return err
	}
	columns := w.schema.defaultColumns()
	if raw := c.Query("columns"); raw != "" {
		columns = strings.Split(raw, ",")
	}
	if err := w.schema.validateColumns(columns); err != nil {
		return fiber.NewError(fiber.StatusBadRequest, err.Error())
	}
	var rows []T
	var err error
	truncated := false
	switch c.Query("scope", "filtered") {
	case "selected":
		rows, err = w.selected(c)
	case "filtered":
		rows, truncated, err = w.filtered(c)
	default:
		return fiber.NewError(fiber.StatusBadRequest, "Invalid export scope")
	}
	if err != nil {
		return err
	}
	items, err := w.grid.DecorateItems(c, rows)
	if err != nil {
		return fiber.NewError(fiber.StatusInternalServerError, "Unable to prepare export")
	}
	body, err := encodeCSV(items, columns, w.schema)
	if err != nil {
		return fiber.NewError(fiber.StatusInternalServerError, "Unable to encode export")
	}
	c.Set("Content-Type", "text/csv; charset=utf-8")
	c.Set("Content-Disposition", `attachment; filename="`+w.options.Key+`.csv"`)
	c.Set("X-Goadmin-Export-Truncated", strconv.FormatBool(truncated))
	return c.Send(body)
}

func (w workflow[T]) selected(c fiber.Ctx) ([]T, error) {
	ids, err := ParseIDs(c.Query("ids"))
	if err != nil {
		return nil, fiber.NewError(fiber.StatusBadRequest, err.Error())
	}
	if w.options.LoadSelected == nil {
		return nil, fiber.NewError(fiber.StatusBadRequest, "Selected export is unavailable")
	}
	rows, err := w.options.LoadSelected(c, ids)
	if err != nil {
		var requestError *fiber.Error
		if errors.As(err, &requestError) && requestError.Code == fiber.StatusBadRequest {
			return nil, fiber.NewError(fiber.StatusBadRequest, "Invalid selected row identities")
		}
		return nil, fiber.NewError(fiber.StatusInternalServerError, "Unable to load selected rows")
	}
	// Fence callback results to requested identities, preserving their order.
	byID := make(map[string]T, len(rows))
	for _, row := range rows {
		id := w.options.SelectionID(row)
		if slices.Contains(ids, id) {
			byID[id] = row
		}
	}
	rows = make([]T, 0, len(ids))
	for _, id := range ids {
		if row, ok := byID[id]; ok {
			rows = append(rows, row)
		}
	}
	return rows, nil
}

func (w workflow[T]) filtered(c fiber.Ctx) ([]T, bool, error) {
	rows := make([]T, 0)
	filters := w.grid.ParseRequestFilters(c)
	filters.Page = 1
	filters.Limit = 100
	for len(rows) < MaxExportRows {
		batch, total, err := w.config.Repository.GetList(c, filters)
		if err != nil {
			return nil, false, fiber.NewError(fiber.StatusInternalServerError, "Unable to export rows")
		}
		remaining := MaxExportRows - len(rows)
		oversized := len(batch) > remaining
		if oversized {
			batch = batch[:remaining]
		}
		rows = append(rows, batch...)
		if len(rows) >= MaxExportRows {
			return rows, oversized || total > len(rows), nil
		}
		if len(batch) < filters.Limit || len(rows) >= total {
			break
		}
		filters.Page++
	}
	return rows, false, nil
}

// ParseIDs preserves opaque string identities and rejects oversized or malformed selections.
func ParseIDs(raw string) ([]string, error) {
	if len(raw) > MaxSelected*256 {
		return nil, errors.New("selection is too large")
	}
	ids := strings.Split(raw, ",")
	if len(ids) == 0 || len(ids) > MaxSelected {
		return nil, errors.New("select between 1 and 100 rows")
	}
	seen := map[string]bool{}
	for _, id := range ids {
		if id == "" || len(id) > 255 || strings.TrimSpace(id) != id || strings.ContainsAny(id, "\r\n\t\x00") || seen[id] {
			return nil, errors.New("invalid or duplicate row identity")
		}
		seen[id] = true
	}
	return ids, nil
}

func encodeCSV[T any](items []datagrid.Item[T], columns []string, s schema) ([]byte, error) {
	var buffer bytes.Buffer
	writer := csv.NewWriter(&buffer)
	header := make([]string, 0, len(columns))
	for _, key := range columns {
		c, _ := s.column(key)
		header = append(header, safeCell(c.Label))
	}
	if err := writer.Write(header); err != nil {
		return nil, err
	}
	for _, item := range items {
		raw, err := json.Marshal(item.Item)
		if err != nil {
			return nil, err
		}
		// UseNumber retains full int64 identity precision in CSV as well as JSON rows.
		data := map[string]any{}
		decoder := json.NewDecoder(bytes.NewReader(raw))
		decoder.UseNumber()
		if err = decoder.Decode(&data); err != nil {
			return nil, err
		}
		values := map[string]any{}
		if item.Values != nil {
			encodedValues, valueErr := json.Marshal(item.Values)
			if valueErr != nil {
				return nil, valueErr
			}
			valueDecoder := json.NewDecoder(bytes.NewReader(encodedValues))
			valueDecoder.UseNumber()
			if valueErr = valueDecoder.Decode(&values); valueErr != nil {
				return nil, valueErr
			}
		}
		row := make([]string, 0, len(columns))
		for _, key := range columns {
			value := data[key]
			if decorated, ok := values[key]; ok {
				value = decorated
			}
			row = append(row, safeCell(cell(value)))
		}
		if err = writer.Write(row); err != nil {
			return nil, err
		}
	}
	writer.Flush()
	return buffer.Bytes(), writer.Error()
}

func cell(value any) string {
	if value == nil {
		return ""
	}
	switch v := value.(type) {
	case string:
		return v
	case json.Number:
		return v.String()
	case bool:
		return strconv.FormatBool(v)
	case map[string]any:
		// Decorated badges/links export their display text; never their URL or markup.
		for _, key := range []string{"label", "text", "name", "value"} {
			if text, ok := v[key]; ok {
				return cell(text)
			}
		}
		return ""
	case []any:
		parts := make([]string, 0, len(v))
		for _, item := range v {
			parts = append(parts, cell(item))
		}
		return strings.Join(parts, ", ")
	default:
		return fmt.Sprint(v)
	}
}

func safeCell(value string) string {
	// Spreadsheet formula injection remains possible despite CSV quoting.
	if strings.HasPrefix(value, "\t") || strings.HasPrefix(value, "\r") || strings.HasPrefix(value, "\n") {
		return "'" + value
	}
	trimmed := strings.TrimLeft(value, " ")
	if trimmed != "" && strings.ContainsRune("=+-@", rune(trimmed[0])) {
		return "'" + value
	}
	return value
}

// APIHandler writes sanitized API errors directly, avoiding the global Inertia
// form-error redirect. It wraps only endpoint logic: outer authentication,
// authorization, CSRF and preview middleware retain their own error handling.
func APIHandler(handler fiber.Handler) fiber.Handler {
	return func(c fiber.Ctx) error {
		err := handler(c)
		if err == nil {
			return nil
		}
		status := fiber.StatusInternalServerError
		var requestError *fiber.Error
		if errors.As(err, &requestError) {
			switch requestError.Code {
			case fiber.StatusBadRequest, fiber.StatusUnauthorized, fiber.StatusConflict, fiber.StatusRequestEntityTooLarge:
				status = requestError.Code
			}
		}
		message := "Unable to complete table request"
		switch status {
		case fiber.StatusBadRequest:
			message = "Invalid table request"
		case fiber.StatusUnauthorized:
			message = "Authentication required"
		case fiber.StatusConflict:
			message = "Table preferences changed; reload before saving"
		case fiber.StatusRequestEntityTooLarge:
			message = "Table preferences exceed 64 KiB"
		}
		c.Set("Cache-Control", "no-store")
		// This fixed string-only payload is always JSON-serializable. Returning nil
		// keeps the form-oriented global error listener out of this API boundary.
		_ = c.Status(status).JSON(map[string]string{"message": message})
		return nil
	}
}
