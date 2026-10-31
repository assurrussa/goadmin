//go:build integration

package appearance_test

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/base64"
	"fmt"
	"image"
	"image/color"
	"image/jpeg"
	"image/png"
	"sync"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"

	"github.com/assurrussa/goadmin/host"
	audit "github.com/assurrussa/goadmin/infrastructure/pgsql/repositories/adminactionauditrepo"
	"github.com/assurrussa/goadmin/internal/adminpreferences"
	"github.com/assurrussa/goadmin/internal/appearance"
	"github.com/assurrussa/goadmin/tests"
)

func TestPostgresAppearanceCASAuditRollbackAndReplicaConvergence(t *testing.T) {
	ctx := context.Background()
	db, _, cleanup := tests.PrepareDB(ctx, t, "Appearance")
	t.Cleanup(func() { cleanup(ctx) })
	newOwner := func() int64 {
		subject := uuid.New()
		_, err := db.DB().Exec(ctx, "appearance.test.subject", `INSERT INTO auth_subjects(id,status,created_at,updated_at) VALUES($1,'active',NOW(),NOW())`, subject)
		require.NoError(t, err)
		var id int64
		require.NoError(t, db.DB().ScanOne(ctx, "appearance.test.owner", &id, `INSERT INTO administrations(subject_id,uuid) VALUES($1,$2) RETURNING id`, subject, uuid.New()))
		return id
	}
	owner, other := newOwner(), newOwner()
	writer := audit.Must(audit.NewOptions(db))
	tx := host.NewTxManager(db)
	s := appearance.NewService(db.DB(), tx.RunInTx, writer)
	replica := appearance.NewService(db.DB(), tx.RunInTx, writer)
	require.NoError(t, s.Start(ctx))
	require.NoError(t, replica.Start(ctx))
	t.Cleanup(func() { require.NoError(t, s.Close(ctx)); require.NoError(t, replica.Close(ctx)) })
	payload := appearance.DefaultPersonal()
	payload.Theme = "dark"
	doc, err := s.SavePersonal(ctx, owner, appearance.PersonalRequest{Payload: payload})
	require.NoError(t, err)
	require.EqualValues(t, 1, doc.Version)
	otherDoc, err := s.LoadPersonal(ctx, other)
	require.NoError(t, err)
	require.Zero(t, otherDoc.Version)
	require.Equal(t, "system", otherDoc.Payload.Theme)
	errs := make(chan error, 2)
	var wg sync.WaitGroup
	for range 2 {
		wg.Go(func() {
			_, e := s.SavePersonal(ctx, owner, appearance.PersonalRequest{Version: 1, Payload: payload})
			errs <- e
		})
	}
	wg.Wait()
	close(errs)
	success, conflict := 0, 0
	for e := range errs {
		if e == nil {
			success++
		} else {
			require.ErrorIs(t, e, adminpreferences.ErrConflict)
			conflict++
		}
	}
	require.Equal(t, 1, success)
	require.Equal(t, 1, conflict)
	// Race distinct complete publications from the same version. The winner's
	// entire row and exactly one audit must commit; neither loser's blobs may leak.
	logo := func(c color.RGBA) string {
		img := image.NewRGBA(image.Rect(0, 0, 3, 2))
		for y := range 2 {
			for x := range 3 {
				img.SetRGBA(x, y, c)
			}
		}
		var encoded bytes.Buffer
		require.NoError(t, jpeg.Encode(&encoded, img, nil))
		return base64.StdEncoding.EncodeToString(encoded.Bytes())
	}
	lightA, darkA := logo(color.RGBA{R: 220, A: 255}), logo(color.RGBA{B: 220, A: 255})
	lightB, darkB := logo(color.RGBA{G: 220, A: 255}), logo(color.RGBA{R: 100, G: 100, A: 255})
	requests := []appearance.SaveBrandingRequest{
		{Name: "  First  ", AccentColor: "#aabbcc", LogoLight: &lightA, LogoDark: &darkA},
		{Name: "  Second  ", AccentColor: "#ddeeff", LogoLight: &lightB, LogoDark: &darkB},
	}
	type result struct {
		index int
		doc   appearance.BrandingDocument
		err   error
	}
	results, start := make(chan result, 2), make(chan struct{})
	for i, request := range requests {
		go func() { <-start; doc, e := s.SaveBranding(ctx, owner, request); results <- result{i, doc, e} }()
	}
	close(start)
	var winner result
	success, conflict = 0, 0
	for range 2 {
		r := <-results
		if r.err == nil {
			success++
			winner = r
		} else {
			require.ErrorIs(t, r.err, adminpreferences.ErrConflict)
			conflict++
		}
	}
	require.Equal(t, 1, success)
	require.Equal(t, 1, conflict)
	require.EqualValues(t, 1, winner.doc.Version)
	require.False(t, winner.doc.Pending)
	expectedLight, err := appearance.NormalizeLogo(*requests[winner.index].LogoLight)
	require.NoError(t, err)
	expectedDark, err := appearance.NormalizeLogo(*requests[winner.index].LogoDark)
	require.NoError(t, err)
	for _, raw := range [][]byte{expectedLight, expectedDark} {
		cfg, e := png.DecodeConfig(bytes.NewReader(raw))
		require.NoError(t, e)
		require.Equal(t, 3, cfg.Width)
		require.Equal(t, 2, cfg.Height)
	}
	expected := appearance.Snapshot{
		Version: 1, Name: []string{"First", "Second"}[winner.index],
		AccentColor: []string{"#AABBCC", "#DDEEFF"}[winner.index],
		LogoLight:   string(expectedLight), LogoDark: string(expectedDark),
	}
	store := appearance.NewBrandStore(db.DB())
	persisted, err := store.Load(ctx)
	require.NoError(t, err)
	require.Equal(t, expected, persisted)
	assertReplica := func(service *appearance.Service) bool {
		if service.Brand() != expected.Public() {
			return false
		}
		for variant, want := range map[string][]byte{"light": expectedLight, "dark": expectedDark} {
			raw, etag, ok := service.Asset(variant)
			if !ok || !bytes.Equal(raw, want) || etag != fmt.Sprintf(`"%x"`, sha256.Sum256(want)) {
				return false
			}
		}
		return true
	}
	require.Eventually(t, func() bool { return assertReplica(replica) }, 5*time.Second, 20*time.Millisecond)
	// Callers own returned values: changing either public metadata or returned
	// image bytes cannot mutate the immutable snapshot on either runtime.
	for _, service := range []*appearance.Service{s, replica} {
		public := service.Brand()
		public.Name = "tampered"
		require.NotEqual(t, public, service.Brand())
		raw, _, ok := service.Asset("light")
		require.True(t, ok)
		raw[0] ^= 0xff
		require.True(t, assertReplica(service))
	}
	// A genuine audit SQL failure must roll back all fields and both blobs,
	// preserving the previous publication and each instance's public assets.
	_, err = db.DB().Exec(ctx, "appearance.test.audit_constraint", `ALTER TABLE admin_action_audit ADD CONSTRAINT appearance_test_reject CHECK (action <> 'branding.updated') NOT VALID`)
	require.NoError(t, err)
	rejected := requests[1-winner.index]
	rejected.Version, rejected.Name = expected.Version, "Must rollback"
	_, err = s.SaveBranding(ctx, owner, rejected)
	require.Error(t, err)
	persisted, err = store.Load(ctx)
	require.NoError(t, err)
	require.Equal(t, expected, persisted)
	require.True(t, assertReplica(s))
	require.True(t, assertReplica(replica))
	_, err = db.DB().Exec(ctx, "appearance.test.drop_constraint", `ALTER TABLE admin_action_audit DROP CONSTRAINT appearance_test_reject`)
	require.NoError(t, err)
	var count int64
	require.NoError(t, db.DB().ScanOne(ctx, "appearance.test.audit_count", &count, `SELECT count(*) FROM admin_action_audit WHERE action='branding.updated'`))
	require.EqualValues(t, 1, count)
	_, err = s.SaveBranding(ctx, owner, appearance.SaveBrandingRequest{Version: 0, Name: "Stale"})
	require.ErrorIs(t, err, adminpreferences.ErrConflict)
}
