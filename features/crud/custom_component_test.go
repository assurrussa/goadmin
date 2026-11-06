//nolint:goconst // Literal component/owner pairs make grammar fixtures directly comparable with the client registry.
package crud //nolint:testpackage // Exercises the constructor grammar matching the client field registry.

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestCustomComponentMatchesClientRegistryGrammar(t *testing.T) {
	for _, tc := range []struct {
		name, component, owner string
		valid                  bool
	}{
		{"client fixture", "host.example/slug", "host.example", true},
		{"existing fixture", "host.fields/value", "host.fields", true},
		{"single letters", "a/b", "a", true},
		{"allowed punctuation and digits", "host.fields_2-test/value._-2", "host.fields_2-test", true},
		{"empty field", "host.fields/", "host.fields", false},
		{"extra field segment", "host.fields/value/extra", "host.fields", false},
		{"missing namespace", "value", "host.fields", false},
		{"empty namespace", "/value", "host.fields", false},
		{"uppercase namespace", "Host.fields/value", "Host.fields", false},
		{"uppercase field", "host.fields/Value", "host.fields", false},
		{"numeric namespace start", "2host/value", "2host", false},
		{"numeric field start", "host.fields/2value", "host.fields", false},
		{"punctuation field start", "host.fields/_value", "host.fields", false},
		{"space in field", "host.fields/value key", "host.fields", false},
		{"non ASCII field", "host.fields/значение", "host.fields", false},
		{"namespace boundary", "host.fields.extra/value", "host.fields", false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			d := scalarDefinition[string]("custom")
			d.Fields[0].Component = tc.component
			d.ClientRequirements[0].Key = tc.owner
			_, err := New(d)
			if tc.valid {
				require.NoError(t, err)
			} else {
				require.Error(t, err)
			}
		})
	}
}
