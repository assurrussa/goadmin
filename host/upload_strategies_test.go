package host //nolint:testpackage // verifies immutable uploads module configuration

import (
	"context"
	"testing"

	"github.com/stretchr/testify/require"
)

type uploadStrategyProbe struct{ UploadStrategy }

type uploadModuleRepository struct{ FileRepository }

func (uploadModuleRepository) GetByPath(context.Context, string) (File, error)   { return File{}, nil }
func (uploadModuleRepository) BindPreview(context.Context, PreviewBinding) error { return nil }

type (
	uploadModuleLoader       struct{ FileLoader }
	uploadModuleUploader     struct{ TaskUploader }
	uploadModuleStore        struct{ TusStore }
	uploadModuleAfterProcess struct{ AfterProcessHandler }
)

func TestUploadsModuleCopiesCustomStrategies(t *testing.T) {
	t.Parallel()
	const strategyName = "meditation-audio"
	strategy := &uploadStrategyProbe{}
	strategies := map[string]UploadStrategy{strategyName: strategy}
	module := UploadsModule(UploadsConfig{
		Repository: uploadModuleRepository{}, Loader: uploadModuleLoader{},
		Uploads: Uploads{
			Service: uploadModuleUploader{}, TusStore: uploadModuleStore{}, AfterProcess: uploadModuleAfterProcess{},
		},
		Strategies: strategies,
	})
	delete(strategies, strategyName)
	strategies["avatar"] = nil
	first, second := &assembly{}, &assembly{}
	require.NoError(t, module.configure(first))
	require.Equal(t, map[string]UploadStrategy{strategyName: strategy}, first.input.UploadStrategies)
	first.input.UploadStrategies["avatar"] = nil
	require.NoError(t, module.configure(second))
	require.Equal(t, map[string]UploadStrategy{strategyName: strategy}, second.input.UploadStrategies)
}

func TestUploadsModuleRejectsInvalidStrategiesBeforeDependencies(t *testing.T) {
	t.Parallel()
	var typedNil *uploadStrategyProbe
	for _, test := range []struct {
		name     string
		strategy UploadStrategy
		message  string
	}{
		{"default", &uploadStrategyProbe{}, "reserved strategy name"},
		{" audio", &uploadStrategyProbe{}, "invalid strategy name"},
		{"audio", nil, "is nil"},
		{"audio", typedNil, "is nil"},
	} {
		a := &assembly{}
		err := UploadsModule(UploadsConfig{Strategies: map[string]UploadStrategy{test.name: test.strategy}}).configure(a)
		require.ErrorContains(t, err, test.message)
		require.Nil(t, a.input.UploadStrategies)
	}
}
