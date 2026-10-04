package bootstrap //nolint:testpackage // verifies canonical handler strategy registration

import (
	"testing"

	uploadhost "github.com/assurrussa/gouploads/host"
	"github.com/stretchr/testify/require"

	uploadfiles "github.com/assurrussa/goadmin/http/strategies/uploadfiles"
)

type strategyRecorder map[string]uploadhost.UploadStrategy

func (r strategyRecorder) RegisterStrategy(name string, strategy uploadhost.UploadStrategy) {
	r[name] = strategy
}

func TestRegisterUploadStrategiesPreservesBuiltins(t *testing.T) {
	t.Parallel()
	for _, custom := range []map[string]uploadhost.UploadStrategy{nil, {"meditation-audio": uploadfiles.NewGenericStrategy()}} {
		recorder := strategyRecorder{}
		require.NoError(t, registerUploadStrategies(recorder, nil, custom))
		require.Len(t, recorder, 4+len(custom))
		require.IsType(t, &uploadfiles.AvatarStrategy{}, recorder["avatar"])
		require.IsType(t, &uploadfiles.RichTextStrategy{}, recorder["rich-text"])
		require.IsType(t, &uploadfiles.GenericStrategy{}, recorder["default"])
		require.IsType(t, &uploadfiles.GenericStrategy{}, recorder["image-uploader"])
		for name, strategy := range custom {
			require.Same(t, strategy, recorder[name])
		}
	}
}

func TestRegisterUploadStrategiesRejectsInvalidBeforeRegistration(t *testing.T) {
	t.Parallel()
	var typedNil *uploadfiles.GenericStrategy
	for name, strategy := range map[string]uploadhost.UploadStrategy{
		"avatar": uploadfiles.NewGenericStrategy(), "": uploadfiles.NewGenericStrategy(), "audio": nil, "typed-nil": typedNil,
	} {
		recorder := strategyRecorder{}
		require.Error(t, registerUploadStrategies(recorder, nil, map[string]uploadhost.UploadStrategy{name: strategy}))
		require.Empty(t, recorder)
	}
}
