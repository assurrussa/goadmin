package browserstate

import (
	"context"
	"time"

	"github.com/assurrussa/goadmin/internal/refreshpolicy"
)

// Guarded prevents a stale Fiber snapshot from restoring a rotating or
// abandoned browser identity. A hidden record is retained, never unlocked;
// the old refresh secret cannot acquire a new owner.
func Guarded(store Store) Store { return guardedStore{Store: store} }

type guardedStore struct{ Store }

func (s guardedStore) Load(ctx context.Context, id string) (Record, bool, error) {
	record, found, err := s.Store.Load(ctx, id)
	if err != nil || !found {
		return record, found, err
	}
	if refreshpolicy.RequiresLogin(record.Owner, time.Now()) {
		return Record{}, false, nil
	}
	return record, true, nil
}

func (s guardedStore) Complete(ctx context.Context, id string, version int64, owner string, record Record) (bool, error) {
	// An owner that exceeded the maximum operation budget must not resurrect
	// an identity after readers have already required reauthentication.
	if owner == "" || refreshpolicy.RequiresLogin(owner, time.Now()) {
		return false, nil
	}
	return s.Store.Complete(ctx, id, version, owner, record)
}
