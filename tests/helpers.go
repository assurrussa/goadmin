package tests

import (
	"bytes"
	"context"
	"database/sql"
	"image"
	"image/color"
	"image/png"
	"io"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"path"
	"strings"
	"testing"
	"time"

	uploadhost "github.com/assurrussa/gouploads/host"
	"github.com/google/uuid"
	"github.com/stretchr/testify/require"

	"github.com/assurrussa/goadmin/internal/pointer"
)

func MakeFileHeader(t *testing.T, fieldName, fileName, content, contentType string) *multipart.FileHeader {
	t.Helper()

	body := &bytes.Buffer{}
	writer := multipart.NewWriter(body)
	part, err := writer.CreateFormFile(fieldName, fileName)
	require.NoError(t, err)
	_, err = io.Copy(part, strings.NewReader(content))
	require.NoError(t, err)
	require.NoError(t, writer.Close())

	req := httptest.NewRequestWithContext(context.Background(), http.MethodPost, "/", body)
	req.Header.Set("Content-Type", writer.FormDataContentType())
	require.NoError(t, req.ParseMultipartForm(int64(body.Len())))

	fileHeaders := req.MultipartForm.File[fieldName]
	require.Len(t, fileHeaders, 1)

	fh := fileHeaders[0]
	if contentType != "" {
		fh.Header.Set("Content-Type", contentType)
	}

	return fh
}

func MakeFileHeaderImage(t *testing.T, fieldName, fileName, contentType string) *multipart.FileHeader {
	t.Helper()

	body := &bytes.Buffer{}
	writer := multipart.NewWriter(body)
	part, err := writer.CreateFormFile(fieldName, fileName)
	require.NoError(t, err)
	_, err = io.Copy(part, CreateTestImage(t))
	require.NoError(t, err)
	require.NoError(t, writer.Close())

	req := httptest.NewRequestWithContext(context.Background(), http.MethodPost, "/", body)
	req.Header.Set("Content-Type", writer.FormDataContentType())
	require.NoError(t, req.ParseMultipartForm(int64(body.Len())))

	fileHeaders := req.MultipartForm.File[fieldName]
	require.Len(t, fileHeaders, 1)

	fh := fileHeaders[0]
	if contentType != "" {
		fh.Header.Set("Content-Type", contentType)
	}

	return fh
}

func CreateTestImage(t *testing.T) io.Reader {
	t.Helper()

	img := image.NewRGBA(image.Rect(0, 0, 2, 2))
	img.Set(0, 0, color.RGBA{R: 255, G: 0, B: 0, A: 255})
	img.Set(1, 1, color.RGBA{R: 0, G: 255, B: 0, A: 255})

	var buf bytes.Buffer
	require.NoError(t, png.Encode(&buf, img))

	return bytes.NewReader(buf.Bytes())
}

func CreateFile(t *testing.T) uploadhost.File {
	t.Helper()

	objectType := uploadhost.ObjectTypeAdmin
	objectID := uploadhost.ObjectID(12)
	uploadPath := []string{"uploads", objectType.String(), objectID.String()}
	folderPath := path.Join(uploadPath...)
	fileName := "example.png"
	fileURL := "https://ceph.localhost/" + path.Join(folderPath, fileName)
	tmNow := time.Now()

	return uploadhost.File{
		ID:               123456,
		UserID:           pointer.To[int64](123),
		ManagerID:        pointer.To[int64](24),
		ObjectType:       objectType,
		ObjectID:         pointer.To(objectID),
		OriginalFileName: "example_origin.png",
		FileName:         fileName,
		FolderPath:       folderPath,
		Size:             12345,
		MimeType:         "image/png",
		FileType:         uploadhost.FileTypeImage,
		URL:              fileURL,
		Slug:             uuid.NewString(),
		Data: &uploadhost.FileData{
			Width:  2,
			Height: 2,
		},
		IsPrimary: false,
		CreatedAt: tmNow,
		UpdatedAt: tmNow,
		PublishedAt: sql.NullTime{
			Valid: true,
			Time:  tmNow,
		},
	}
}
