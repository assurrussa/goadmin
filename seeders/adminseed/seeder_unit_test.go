package adminseed_test

import (
	"context"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/assurrussa/goadmin/seeders/adminseed"
)

func TestSeedRejectsImplicitDefaultCredentialsBeforeDatabaseAccess(t *testing.T) {
	t.Parallel()

	seed := adminseed.NewSeed(nil)
	err := seed.Handle(context.Background())
	require.Error(t, err)
	require.ErrorContains(t, err, "auth adapter")
}
