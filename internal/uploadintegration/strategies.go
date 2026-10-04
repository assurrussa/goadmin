package uploadintegration

import (
	"fmt"
	"maps"
	"reflect"
	"regexp"
	"slices"

	uploadhost "github.com/assurrussa/gouploads/host"
)

var strategyNamePattern = regexp.MustCompile(`^[a-z][a-z0-9_-]{0,63}$`)

// ValidateStrategies protects the canonical handler's built-in upload contexts.
func ValidateStrategies(strategies map[string]uploadhost.UploadStrategy) error {
	for _, name := range slices.Sorted(maps.Keys(strategies)) {
		if !strategyNamePattern.MatchString(name) {
			return fmt.Errorf("uploads: invalid strategy name %q", name)
		}
		switch name {
		case "avatar", "rich-text", "default", "image-uploader":
			return fmt.Errorf("uploads: reserved strategy name %q", name)
		}
		strategy := strategies[name]
		if strategy == nil {
			return fmt.Errorf("uploads: strategy %q is nil", name)
		}
		value := reflect.ValueOf(strategy)
		switch value.Kind() {
		case reflect.Chan, reflect.Func, reflect.Interface, reflect.Map, reflect.Pointer, reflect.Slice:
			if value.IsNil() {
				return fmt.Errorf("uploads: strategy %q is nil", name)
			}
		default:
		}
	}
	return nil
}
