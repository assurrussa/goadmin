package externalconsumer_test

import (
	"testing"

	"github.com/assurrussa/goadmin/features/uploads"
	"github.com/assurrussa/goadmin/host"
)

func TestCustomUploadStrategyContract(t *testing.T) {
	t.Parallel()
	// Module construction snapshots the public map without running a strategy.
	// host.New validates every strategy and the required upload services.
	module := uploads.New(uploads.Config{Strategies: map[string]host.UploadStrategy{}})
	if module.Descriptor().Key != "uploads" {
		t.Fatal("custom strategies must use the canonical uploads module")
	}
}
