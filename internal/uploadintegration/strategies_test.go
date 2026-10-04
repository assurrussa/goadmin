package uploadintegration_test

import (
	"context"
	"strings"
	"testing"

	uploadhost "github.com/assurrussa/gouploads/host"
	"github.com/stretchr/testify/require"

	"github.com/assurrussa/goadmin/internal/uploadintegration"
)

type (
	strategyProbe    struct{ uploadhost.UploadStrategy }
	mapStrategyProbe map[string]uploadhost.UploadStrategy
)

func (mapStrategyProbe) CanUpload(context.Context, uploadhost.UploadContext) error {
	return nil
}

func (mapStrategyProbe) GetConfig(context.Context, uploadhost.UploadContext) *uploadhost.FileUploadConfig {
	return nil
}

func (mapStrategyProbe) GetAfterJobs(context.Context, uploadhost.UploadContext) ([]uploadhost.FileEventAfterJob, error) {
	return nil, nil
}

func TestValidateUploadStrategies(t *testing.T) {
	t.Parallel()
	valid := &strategyProbe{}
	for _, name := range []string{"meditation-audio", "audio_1", "a", strings.Repeat("a", 64)} {
		require.NoError(t, uploadintegration.ValidateStrategies(map[string]uploadhost.UploadStrategy{name: valid}))
	}
	require.NoError(t, uploadintegration.ValidateStrategies(nil))
	for _, name := range []string{
		"", " ", " audio", "audio ", "Audio", "1audio", "audio/track", "audio.track",
		"audio\n", "аудио", strings.Repeat("a", 65),
	} {
		require.ErrorContains(t, uploadintegration.ValidateStrategies(map[string]uploadhost.UploadStrategy{name: valid}),
			"invalid strategy name")
	}
	for _, name := range []string{"avatar", "rich-text", "default", "image-uploader"} {
		require.ErrorContains(t, uploadintegration.ValidateStrategies(map[string]uploadhost.UploadStrategy{name: valid}),
			"reserved strategy name")
	}
	var pointer *strategyProbe
	var mapped mapStrategyProbe
	for _, strategy := range []uploadhost.UploadStrategy{nil, pointer, mapped} {
		require.ErrorContains(t, uploadintegration.ValidateStrategies(map[string]uploadhost.UploadStrategy{"audio": strategy}),
			`strategy "audio" is nil`)
	}
}
