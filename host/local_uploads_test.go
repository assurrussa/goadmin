package host //nolint:testpackage // verifies local facade adapter seams

import (
	"context"
	"database/sql"
	"testing"
	"time"

	uploadhost "github.com/assurrussa/gouploads/host"
	"github.com/stretchr/testify/require"
)

type previewRepo struct {
	FileRepository
	file File
}

func (r previewRepo) GetByID(context.Context, int64) (File, error) { return r.file, nil }
func completedTestFile() File {
	return File{
		ID: 1, FileName: "source.png", FolderPath: "media/v1/1",
		Data: &FileData{Presets: map[uploadhost.PresetName]uploadhost.FilePreset{
			"main": {RelativePath: "media/v1/1/source.png", URL: "https://admin.test/uploads/media/v1/1/source.png"},
		}},
	}
}

func TestLocalPreviewRequiresFinalArtifact(t *testing.T) {
	for _, test := range []struct {
		name   string
		mutate func(*File)
		want   bool
	}{
		{"finalized", func(*File) {}, true},
		{"legacy", func(f *File) { f.Data = nil }, false},
		{"queued", func(f *File) { f.Data.Uploader.Status = uploadhost.FileUploadTaskStatusQueued }, false},
		{"processing", func(f *File) { f.Data.Uploader.Status = uploadhost.FileUploadTaskStatusProcessing }, false},
		{"failed", func(f *File) { f.Data.Uploader.Status = uploadhost.FileUploadTaskStatusFailed }, false},
		{"missing artifact", func(f *File) { f.Data.Presets = nil }, false},
		{"staging", func(f *File) { f.FolderPath = "staging/v1/tus/1" }, false},
		{"wrong artifact", func(f *File) { f.FileName = "other.png" }, false},
		{"deleted", func(f *File) { f.DeletedAt = sql.NullTime{Time: time.Now(), Valid: true} }, false},
	} {
		t.Run(test.name, func(t *testing.T) {
			file := completedTestFile()
			test.mutate(&file)
			loader := localFileLoader{repo: previewRepo{file: file}}
			preview, err := loader.LoadPreview(t.Context(), file.ID)
			require.NoError(t, err)
			require.Equal(t, test.want, preview != nil)
			require.Equal(t, test.want, IsFinalizedUpload(file))
		})
	}
}
