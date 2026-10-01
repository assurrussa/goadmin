package main

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestConsumerGraphBoundary(t *testing.T) {
	const candidatePath = "/tmp/admin"
	const module = "github.com/assurrussa/goadmin"
	for _, tc := range []struct {
		name, graph string
		expected    map[string]string
		valid       bool
	}{
		{"published", `{"Path":"github.com/assurrussa/goadmin","Version":"v0.7.0"}`, nil, true},
		{
			"candidate", `{"Path":"github.com/assurrussa/goadmin","Replace":{"Path":"/tmp/admin"}}`,
			map[string]string{module: candidatePath},
			true,
		},
		{"hidden replacement", `{"Path":"dep","Replace":{"Path":"/tmp/local"}}`, nil, false},
		{
			"local admin with published dependencies",
			`{"Path":"github.com/assurrussa/goadmin","Replace":{"Path":"/tmp/admin"}}
			 {"Path":"github.com/assurrussa/goauth","Version":"v0.4.0"}
			 {"Path":"github.com/assurrussa/gouploads","Version":"v0.10.0"}`,
			map[string]string{module: candidatePath},
			true,
		},
		{
			"public dependency gate rejects sibling auth",
			`{"Path":"github.com/assurrussa/goadmin","Replace":{"Path":"/tmp/admin"}}
			 {"Path":"github.com/assurrussa/goauth","Replace":{"Path":"/tmp/goauth"}}`,
			map[string]string{module: candidatePath},
			false,
		},
		{
			"wrong candidate", `{"Path":"github.com/assurrussa/goadmin","Replace":{"Path":"/tmp/other"}}`,
			map[string]string{module: candidatePath},
			false,
		},
		{"missing candidate", `{"Path":"github.com/assurrussa/goadmin"}`, map[string]string{module: candidatePath}, false},
		{"absent candidate", `{"Path":"main"}`, map[string]string{module: candidatePath}, false},
		{"legacy shared", `{"Path":"github.com/assurrussa/goshared"}`, nil, false},
		{"legacy redis", `{"Path":"github.com/assurrussa/goredis"}`, nil, false},
		{"legacy fiber wrapper", `{"Path":"github.com/assurrussa/gofiber"}`, nil, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			err := checkGraph(strings.NewReader(tc.graph), tc.expected)
			if tc.valid {
				require.NoError(t, err)
			} else {
				require.Error(t, err)
			}
		})
	}
}
