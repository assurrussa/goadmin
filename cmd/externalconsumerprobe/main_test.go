package main

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestCheckSelectedModuleVersion(t *testing.T) {
	const path = "github.com/assurrussa/goauth"
	const expected = "v0.4.0"

	require.NoError(t, checkSelectedModuleVersion(path, expected, selectedModule{Path: path, Version: expected}))
	require.ErrorContains(t, checkSelectedModuleVersion(path, expected, selectedModule{Path: path, Version: "v0.3.0"}), "selected")
	require.ErrorContains(t, checkSelectedModuleVersion(path, expected, selectedModule{
		Path: path, Version: expected, Replace: &struct {
			Path string `json:"path"`
		}{Path: "/tmp/goauth"},
	}), "replace=true")
}
