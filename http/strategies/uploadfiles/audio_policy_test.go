package uploadfiles_test

import (
	"testing"

	uploadhost "github.com/assurrussa/gouploads/host"
	"github.com/stretchr/testify/require"

	"github.com/assurrussa/goadmin/http/strategies/uploadfiles"
)

func TestBuiltinPoliciesDoNotEnableAudio(t *testing.T) {
	t.Parallel()
	const (
		jpgExtension  = ".jpg"
		jpegExtension = ".jpeg"
		pngExtension  = ".png"
		webpExtension = ".webp"
	)
	for _, test := range []struct {
		name       string
		strategy   uploadhost.UploadStrategy
		extensions []string
	}{
		{
			"generic", uploadfiles.NewGenericStrategy(),
			[]string{jpgExtension, jpegExtension, pngExtension, ".gif", webpExtension, ".mp4", ".webm", ".pdf"},
		},
		{"rich-text", uploadfiles.NewRichTextStrategy(), []string{jpgExtension, jpegExtension, pngExtension, ".gif", webpExtension}},
		{"avatar", uploadfiles.NewAvatarStrategy(nil), []string{jpgExtension, jpegExtension, pngExtension, webpExtension}},
	} {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()
			config := test.strategy.GetConfig(t.Context(), uploadhost.UploadContext{Metadata: map[string]string{
				"file_type": "audio", "entity_type": "meditation", "entity_id": "17",
			}})
			require.Equal(t, test.extensions, config.AllowedExtensions)
			require.NotContains(t, config.AllowedMimeTypes, ".mp3")
			require.NotContains(t, config.AllowedMimeTypes, ".wav")
		})
	}
}
