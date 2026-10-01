package host

import (
	"context"

	"github.com/assurrussa/goadmin/internal/uploadintegration"
)

type PreviewBinding = uploadintegration.PreviewBinding

var ErrPreviewBindingConflict = uploadintegration.ErrPreviewBindingConflict

// BindPostgresPreview is the supported narrow SQL adapter for host file stores.
// Call it from BindPreview; it requires the managed command transaction context.
func BindPostgresPreview(ctx context.Context, db StoragePgsqlClient, request PreviewBinding) error {
	return uploadintegration.BindPreview(ctx, db, request)
}

func (r *localPathFileRepo) BindPreview(ctx context.Context, request PreviewBinding) error {
	return BindPostgresPreview(ctx, r.db, request)
}
