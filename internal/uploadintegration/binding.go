package uploadintegration

import (
	"context"
	"errors"
	"fmt"

	uploadhost "github.com/assurrussa/gouploads/host"

	"github.com/assurrussa/goadmin/infrastructure/outbox"
	"github.com/assurrussa/goadmin/internal/admintx"
)

var ErrPreviewBindingConflict = errors.New("preview ownership or state changed")

type PreviewBinding struct {
	FileID             int64
	ManagerID          int64
	ExpectedObjectType uploadhost.ObjectType
	ExpectedObjectID   *uploadhost.ObjectID
	AdminID            int64
}

// BindPreview locks and rechecks a finalized file in the shared command
// transaction. It updates only linkage, preserving upload/preset metadata.
func BindPreview(ctx context.Context, db outbox.StoragePgsqlClient, request PreviewBinding) error {
	if _, ok := admintx.Executor(ctx); !ok {
		return errors.New("preview binding requires a managed admin transaction")
	}
	if request.FileID <= 0 || request.ManagerID <= 0 || request.AdminID <= 0 {
		return ErrPreviewBindingConflict
	}
	engine := admintx.Wrap(db.DB())
	var file uploadhost.File
	if err := engine.ScanOne(ctx, "preview.lock", &file, "SELECT * FROM files WHERE id=$1 FOR UPDATE", request.FileID); err != nil {
		return fmt.Errorf("lock preview: %w", err)
	}
	if file.ManagerID == nil || *file.ManagerID != request.ManagerID || file.ObjectType != request.ExpectedObjectType ||
		!equalObjectID(file.ObjectID, request.ExpectedObjectID) || !Finalized(file) {
		return ErrPreviewBindingConflict
	}
	if file.ObjectID != nil && file.ObjectID.Int64() == request.AdminID && file.ObjectType == uploadhost.ObjectTypeAdmin {
		return nil
	}
	const query = "UPDATE files SET object_type=$1, object_id=$2, updated_at=NOW() WHERE id=$3"
	_, err := engine.Exec(ctx, "preview.bind", query, uploadhost.ObjectTypeAdmin, request.AdminID, request.FileID)
	return err
}

func equalObjectID(a, b *uploadhost.ObjectID) bool {
	if a == nil || b == nil {
		return a == nil && b == nil
	}
	return *a == *b
}
