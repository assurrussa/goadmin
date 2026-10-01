package uploadfiles

import (
	"strings"

	uploadhost "github.com/assurrussa/gouploads/host"
)

const (
	extJPG  = ".jpg"
	extJPEG = ".jpeg"
	extPNG  = ".png"
	extGIF  = ".gif"
	extWebP = ".webp"
	extMP4  = ".mp4"
	extWebM = ".webm"
	extPDF  = ".pdf"

	mimeImageJPEG = "image/jpeg"
	mimeImagePNG  = "image/png"
	mimeImageGIF  = "image/gif"
	mimeImageWebP = "image/webp"
	mimeVideoMP4  = "video/mp4"
	mimeVideoWebM = "video/webm"
	mimePDF       = "application/pdf"
)

func defaultFileUploadConfig(addPathDir ...string) *uploadhost.FileUploadConfig {
	return &uploadhost.FileUploadConfig{
		MaxFileSize:       10 * 1024 * 1024,
		AllowedExtensions: []string{extJPG, extJPEG, extPNG, extGIF, extWebP, extMP4, extWebM, extPDF},
		AllowedMimeTypes: map[string][]string{
			extJPG:  {mimeImageJPEG},
			extJPEG: {mimeImageJPEG},
			extPNG:  {mimeImagePNG},
			extGIF:  {mimeImageGIF},
			extWebP: {mimeImageWebP},
			extMP4:  {mimeVideoMP4},
			extWebM: {mimeVideoWebM},
			extPDF:  {mimePDF},
		},
		UploadDir: strings.Join(append([]string{"upload_file"}, addPathDir...), "/"),
	}
}

func defaultRichTextUploadConfig(addPathDir ...string) *uploadhost.FileUploadConfig {
	return &uploadhost.FileUploadConfig{
		MaxFileSize:       5 * 1024 * 1024,
		AllowedExtensions: []string{extJPG, extJPEG, extPNG, extGIF, extWebP},
		AllowedMimeTypes: map[string][]string{
			extJPG:  {mimeImageJPEG},
			extJPEG: {mimeImageJPEG},
			extPNG:  {mimeImagePNG},
			extGIF:  {mimeImageGIF},
			extWebP: {mimeImageWebP},
		},
		UploadDir: strings.Join(append([]string{"upload_file", "rich-text"}, addPathDir...), "/"),
	}
}
