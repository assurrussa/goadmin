package uploadintegration

import (
	"path"
	"strings"

	uploadhost "github.com/assurrussa/gouploads/host"
)

// Finalized requires a recorded final artifact; legacy and staging records fail closed.
func Finalized(file uploadhost.File) bool {
	if file.ID <= 0 || file.DeletedAt.Valid || file.Data == nil {
		return false
	}
	status := file.Data.Uploader.Status
	if status != "" && status != uploadhost.FileUploadTaskStatusCompleted {
		return false
	}
	relative := path.Join(file.FolderPath, file.FileName)
	if file.FileName == "" || file.FileName != path.Base(file.FileName) ||
		file.FolderPath != path.Clean(file.FolderPath) ||
		!strings.HasPrefix(relative, "media/v1/") || strings.Contains(relative, "../") {
		return false
	}
	preset, ok := file.Data.Presets[uploadhost.PresetName("main")]
	return ok && preset.RelativePath == relative
}
