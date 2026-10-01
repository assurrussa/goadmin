package config_test

import (
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/assurrussa/goadmin/config"
)

func TestConfigStaticDataPath(t *testing.T) {
	t.Run("uses configured root", func(t *testing.T) {
		cfg := config.Config{StaticDataRoot: "/app/publicdata"}

		assert.Equal(t, "/app/publicdata", cfg.StaticDataPath())
		assert.Equal(t, filepath.Join("/app/publicdata", "uploads"), cfg.StaticDataPath("uploads"))
		assert.Equal(t, filepath.Join("/app/publicdata", "tmp"), cfg.StaticDataPath("tmp"))
	})

	t.Run("falls back to publicdata", func(t *testing.T) {
		cfg := config.Config{}

		assert.Equal(t, "publicdata", cfg.StaticDataPath())
		assert.Equal(t, filepath.Join("publicdata", "uploads"), cfg.StaticDataPath("uploads"))
	})
}
