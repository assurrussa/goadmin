package bootstrap

import (
	"maps"
	"slices"

	uploadhost "github.com/assurrussa/gouploads/host"

	uploadfiles "github.com/assurrussa/goadmin/http/strategies/uploadfiles"
	"github.com/assurrussa/goadmin/internal/uploadintegration"
)

type uploadStrategyRegistrar interface {
	RegisterStrategy(contextName string, strategy uploadhost.UploadStrategy)
}

func registerUploadStrategies(handler uploadStrategyRegistrar, admins AdminRepo,
	strategies map[string]uploadhost.UploadStrategy,
) error {
	if err := uploadintegration.ValidateStrategies(strategies); err != nil {
		return err
	}
	handler.RegisterStrategy("avatar", uploadfiles.NewAvatarStrategy(admins))
	handler.RegisterStrategy("rich-text", uploadfiles.NewRichTextStrategy())
	handler.RegisterStrategy("default", uploadfiles.NewGenericStrategy())
	handler.RegisterStrategy("image-uploader", uploadfiles.NewGenericStrategy())
	for _, name := range slices.Sorted(maps.Keys(strategies)) {
		handler.RegisterStrategy(name, strategies[name])
	}
	return nil
}
