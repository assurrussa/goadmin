//go:build integration

package profile //nolint:testpackage // Verify private pending-state transitions without exporting test-only APIs.

import (
	"context"
	"encoding/json"
	"errors"
	"sync"
	"testing"
	"time"

	"github.com/assurrussa/goauth"
	"github.com/assurrussa/goauth/postgres"
	"github.com/gofiber/fiber/v3"
	"github.com/google/uuid"
	"github.com/stretchr/testify/require"

	"github.com/assurrussa/goadmin/host"
	"github.com/assurrussa/goadmin/tests"
)

const (
	remoteDisplay  = "Remote display"
	logoutCase     = "logout"
	selectionCase  = "selection"
	relinkCase     = "relink"
	membershipCase = "membership"
)

var completedNative = host.CSRFSessionBinding("newly-published-native-cookie")

type profileFixture struct {
	component *Component
	auth      *host.AuthAdapter
	old       host.ExternalBinding
	fresh     host.ExternalBinding
	session   string
}

func newProfileFixture(t *testing.T) *profileFixture {
	t.Helper()
	db, _, cleanup := tests.PrepareDB(t.Context(), t, "ProfilePresentation")
	auth, err := host.NewAuthAdapter(host.AuthAdapterConfig{
		Database: db, TxManager: host.NewTxManager(db), Runtime: tests.AuthRuntimeConfig(t),
		NotificationSender: tests.AuthNotificationSender(),
	})
	require.NoError(t, err)
	t.Cleanup(func() { require.NoError(t, auth.Close()); cleanup(context.Background()) })
	account, err := auth.Runtime().ProvisionTrustedLocalAccount(t.Context(), goauth.RegisterRequest{
		Email: "profile-fixture@example.test", Password: "Synthetic-Profile-Fixture-2026!",
	})
	require.NoError(t, err)
	binding := testBinding()
	binding.LocalSubject, binding.LinkID = account.Subject.ID, uuid.NewString()
	sqlDB := auth.Runtime().Database()
	_, err = sqlDB.ExecContext(t.Context(), "INSERT INTO administrations(subject_id,uuid) VALUES($1,$2)",
		account.Subject.ID, uuid.NewString())
	require.NoError(t, err)
	_, err = sqlDB.ExecContext(t.Context(), `INSERT INTO auth_identity_links
 (id,subject_id,issuer,external_subject,created_at,updated_at)
 VALUES($1,$2,$3,$4,$5,$5)`, binding.LinkID, binding.LocalSubject, binding.Issuer, binding.Subject, time.Now())
	require.NoError(t, err)
	session := uuid.NewString()
	_, err = sqlDB.ExecContext(t.Context(), `INSERT INTO auth_sessions
 (id,subject_id,realm,scope,security_version,created_at,expires_at) VALUES($1,$2,'admin','authenticated',$3,$4,$5)`,
		session, binding.LocalSubject, account.Subject.SecurityVersion, time.Now(), time.Now().Add(time.Hour))
	require.NoError(t, err)
	component, err := New(t.Context(), Config{
		Runtime: auth.Runtime(), Issuer: binding.Issuer, ClientID: binding.ClientID, ProjectID: binding.ProjectID,
		AvatarHosts: []string{testAvatarHost}, Start: func(fiber.Ctx, string) error { return nil },
	})
	require.NoError(t, err)
	fresh := binding
	fresh.AuthorityID, fresh.Generation, fresh.LoginGeneration = "new-rp-reference", 2, 2
	return &profileFixture{component: component, auth: auth, old: binding, fresh: fresh, session: session}
}

func (f *profileFixture) prepare(t *testing.T) {
	t.Helper()
	now := time.Now()
	require.NoError(t, f.component.owned(t.Context(), f.old, func(ctx context.Context, ex postgres.SQLExecutor) error {
		subject := f.old.LocalSubject.String()
		if err := ensureRecord(ctx, ex, subject); err != nil {
			return err
		}
		return storePending(ctx, ex, subject, pending{
			Selector: digest(testSelector), Native: testNative, AuthSession: f.session, Version: 1,
			Started: now, Until: now.Add(refreshLifetime), Phase: phaseStarted, Original: f.old,
		})
	}))
}

func (f *profileFixture) snapshot(t *testing.T) string {
	t.Helper()
	var value string
	require.NoError(t, f.auth.Runtime().Database().QueryRowContext(t.Context(),
		"SELECT display_name FROM admin_profile_presentations WHERE subject_id=$1", f.old.LocalSubject).Scan(&value))
	return value
}

func (f *profileFixture) capture(t *testing.T) {
	t.Helper()
	require.NoError(t, f.component.Capture(t.Context(), testSelector, testNative, f.fresh,
		Snapshot{Name: remoteDisplay, Login: "remote-label", AvatarURL: testAvatarURL, IssuedAt: f.fresh.Proof.IssuedAt}))
}

func TestProfilePostgresCallbackNativeCommitAndSingleConsumption(t *testing.T) {
	f := newProfileFixture(t)
	f.prepare(t)
	f.capture(t)
	require.Empty(t, f.snapshot(t), "callback alone must not apply a snapshot")
	require.ErrorIs(t, f.component.Capture(t.Context(), testSelector, testNative, f.fresh,
		Snapshot{IssuedAt: f.fresh.Proof.IssuedAt}), ErrDenied, "replayed capture must not replace candidate")
	claim, err := f.component.Claim(t.Context(), testSelector, testNative, f.fresh)
	require.NoError(t, err)
	require.Empty(t, f.snapshot(t), "claim alone must not apply a snapshot")
	require.NoError(t, f.component.arm(t.Context(), testSelector, claim, completedNative, f.fresh))
	require.NoError(t, f.component.complete(t.Context(), testSelector, claim, f.fresh, f.session, completedNative))
	require.Equal(t, remoteDisplay, f.snapshot(t))
	require.ErrorIs(t, f.component.complete(t.Context(), testSelector, claim, f.fresh, f.session, completedNative), ErrDenied)
	var local string
	require.NoError(t, f.auth.Runtime().Database().QueryRowContext(t.Context(),
		"SELECT display_name FROM auth_basic_profiles WHERE subject_id=$1", f.old.LocalSubject).Scan(&local))
	require.NotEqual(t, remoteDisplay, local, "canonical local profile must be preserved")
	view, err := f.component.read(t.Context(), f.fresh)
	require.NoError(t, err)
	require.False(t, view.Fields.Name, "refresh never selects fields")
	require.True(t, view.Available)
}

func TestProfilePostgresWrongBrowserPinsExpirySelectionAndLogoutDeny(t *testing.T) {
	for _, kind := range []string{"browser", "identity", "expired", selectionCase, logoutCase, relinkCase, membershipCase} {
		t.Run(kind, func(t *testing.T) {
			f := newProfileFixture(t)
			f.prepare(t)
			f.capture(t)
			switch kind {
			case "browser":
				_, err := f.component.Claim(t.Context(), testSelector, "wrong-native-browser", f.fresh)
				require.ErrorIs(t, err, ErrDenied)
			case "identity":
				other := f.fresh
				other.Subject = "another-person"
				_, err := f.component.Claim(t.Context(), testSelector, testNative, other)
				require.Error(t, err)
			case "expired":
				var raw []byte
				require.NoError(t, f.auth.Runtime().Database().QueryRowContext(t.Context(),
					"SELECT pending FROM admin_profile_presentations WHERE subject_id=$1", f.old.LocalSubject).Scan(&raw))
				var p pending
				require.NoError(t, json.Unmarshal(raw, &p))
				p.Until = time.Now().Add(-time.Second)
				raw, err := json.Marshal(p)
				require.NoError(t, err)
				_, err = f.auth.Runtime().Database().ExecContext(t.Context(),
					"UPDATE admin_profile_presentations SET pending=$2::jsonb WHERE subject_id=$1", f.old.LocalSubject, string(raw))
				require.NoError(t, err)
				_, err = f.component.Claim(t.Context(), testSelector, testNative, f.fresh)
				require.ErrorIs(t, err, ErrDenied)
			case selectionCase:
				require.NoError(t, f.component.selectFields(t.Context(), f.old, "1", Fields{Name: true}, f.session))
				_, err := f.component.Claim(t.Context(), testSelector, testNative, f.fresh)
				require.ErrorIs(t, err, ErrDenied)
			case logoutCase:
				require.NoError(t, f.auth.Runtime().Logout(t.Context(), f.old.LocalSubject, f.session))
				_, err := f.component.Claim(t.Context(), testSelector, testNative, f.fresh)
				require.ErrorIs(t, err, ErrDenied)
			case relinkCase:
				_, err := f.auth.Runtime().Database().ExecContext(t.Context(),
					"UPDATE auth_identity_links SET id=$2 WHERE id=$1", f.old.LinkID, uuid.NewString())
				require.NoError(t, err)
				_, err = f.component.Claim(t.Context(), testSelector, testNative, f.fresh)
				require.ErrorIs(t, err, ErrDenied)
			case membershipCase:
				_, err := f.auth.Runtime().Database().ExecContext(t.Context(),
					"UPDATE administrations SET deleted_at=$2 WHERE subject_id=$1", f.old.LocalSubject, time.Now())
				require.NoError(t, err)
				_, err = f.component.Claim(t.Context(), testSelector, testNative, f.fresh)
				require.ErrorIs(t, err, ErrDenied)
			}
			require.Empty(t, f.snapshot(t))
		})
	}
}

func TestProfilePostgresConcurrentClaimsAndCancellationFenceCompletion(t *testing.T) {
	f := newProfileFixture(t)
	f.prepare(t)
	f.capture(t)
	var wg sync.WaitGroup
	type result struct {
		claim string
		err   error
	}
	results := make(chan result, 2)
	for range 2 {
		wg.Go(func() {
			claim, err := f.component.Claim(t.Context(), testSelector, testNative, f.fresh)
			results <- result{claim: claim, err: err}
		})
	}
	wg.Wait()
	close(results)
	claims := make([]string, 0, 1)
	for value := range results {
		if value.err == nil {
			claims = append(claims, value.claim)
		} else {
			require.ErrorIs(t, value.err, ErrDenied)
		}
	}
	require.Len(t, claims, 1, "concurrent claim has exactly one winner")
	require.NoError(t, f.component.Cancel(t.Context(), testSelector))
	require.ErrorIs(t, f.component.complete(t.Context(), testSelector, claims[0], f.fresh, f.session, completedNative), ErrDenied)
	require.Empty(t, f.snapshot(t))
}

func TestProfilePostgresSelectionAndCompletionRaceHasOneCASWinner(t *testing.T) {
	f := newProfileFixture(t)
	f.prepare(t)
	f.capture(t)
	claim, err := f.component.Claim(t.Context(), testSelector, testNative, f.fresh)
	require.NoError(t, err)
	require.NoError(t, f.component.arm(t.Context(), testSelector, claim, completedNative, f.fresh))
	start := make(chan struct{})
	results := make(chan error, 2)
	var wg sync.WaitGroup
	wg.Go(func() {
		<-start
		results <- f.component.selectFields(t.Context(), f.old, "1", Fields{Name: true}, f.session)
	})
	wg.Go(func() {
		<-start
		results <- f.component.complete(t.Context(), testSelector, claim, f.fresh, f.session, completedNative)
	})
	close(start)
	wg.Wait()
	close(results)
	successes := 0
	for err := range results {
		if err == nil {
			successes++
		} else {
			require.True(t, errors.Is(err, ErrDenied) || errors.Is(err, ErrConflict), "unexpected storage failure: %v", err)
		}
	}
	require.Equal(t, 1, successes, "selection and snapshot must not both commit from one CAS version")
	view, err := f.component.read(t.Context(), f.fresh)
	require.NoError(t, err)
	if view.Fields.Name {
		require.Empty(t, f.snapshot(t), "selection winner canceled pending snapshot")
	} else {
		require.Equal(t, remoteDisplay, f.snapshot(t), "completion winner preserves previous field selection")
	}
}

func TestProfilePostgresArmedCompletionPinsNewNativeAndPreservesSnapshotOnDenial(t *testing.T) {
	for _, kind := range []string{"native", "claim", logoutCase, relinkCase, membershipCase, selectionCase, "blocked"} {
		t.Run(kind, func(t *testing.T) {
			f := newProfileFixture(t)
			f.prepare(t)
			_, err := f.auth.Runtime().Database().ExecContext(t.Context(),
				"UPDATE admin_profile_presentations SET display_name=$2 WHERE subject_id=$1",
				f.old.LocalSubject, "previous verified snapshot")
			require.NoError(t, err)
			f.capture(t)
			claim, err := f.component.Claim(t.Context(), testSelector, testNative, f.fresh)
			require.NoError(t, err)
			require.NoError(t, f.component.arm(t.Context(), testSelector, claim, completedNative, f.fresh))
			require.ErrorIs(t, f.component.arm(t.Context(), testSelector, claim, completedNative, f.fresh), ErrDenied)
			native := completedNative
			switch kind {
			case "native":
				native = host.CSRFSessionBinding("intervening-same-subject-login")
			case "claim":
				claim = "another-completion-claim"
			case logoutCase:
				require.NoError(t, f.auth.Runtime().Logout(t.Context(), f.old.LocalSubject, f.session))
			case relinkCase:
				_, err = f.auth.Runtime().Database().ExecContext(t.Context(),
					"UPDATE auth_identity_links SET id=$2 WHERE id=$1", f.old.LinkID, uuid.NewString())
				require.NoError(t, err)
			case membershipCase:
				_, err = f.auth.Runtime().Database().ExecContext(t.Context(),
					"UPDATE administrations SET deleted_at=$2 WHERE subject_id=$1", f.old.LocalSubject, time.Now())
				require.NoError(t, err)
			case "blocked":
				_, err = f.auth.Runtime().Database().ExecContext(t.Context(),
					"UPDATE auth_subjects SET status=$2 WHERE id=$1", f.old.LocalSubject, goauth.SubjectStatusSuspended)
				require.NoError(t, err)
			case selectionCase:
				require.NoError(t, f.component.selectFields(t.Context(), f.fresh, "1", Fields{Name: true}, f.session))
			}
			require.Error(t, f.component.complete(t.Context(), testSelector, claim, f.fresh, f.session, native))
			require.Equal(t, "previous verified snapshot", f.snapshot(t))
		})
	}
}

func TestProfilePostgresExpiredPendingDeniesBeforeLazyCleanup(t *testing.T) {
	f := newProfileFixture(t)
	f.prepare(t)
	f.capture(t)
	claim, err := f.component.Claim(t.Context(), testSelector, testNative, f.fresh)
	require.NoError(t, err)
	require.NoError(t, f.component.arm(t.Context(), testSelector, claim, completedNative, f.fresh))
	_, err = f.auth.Runtime().Database().ExecContext(t.Context(), `UPDATE admin_profile_presentations
 SET pending=jsonb_set(pending,'{until}',to_jsonb($2::text)) WHERE subject_id=$1`,
		f.old.LocalSubject, time.Now().Add(-time.Second).UTC().Format(time.RFC3339Nano))
	require.NoError(t, err)
	require.ErrorIs(t, f.component.complete(t.Context(), testSelector, claim, f.fresh, f.session, completedNative), ErrDenied)
	require.Empty(t, f.snapshot(t), "expiry denies even before physical cleanup")
	_, err = f.component.read(t.Context(), f.fresh)
	require.NoError(t, err)
	var empty bool
	require.NoError(t, f.auth.Runtime().Database().QueryRowContext(t.Context(),
		"SELECT pending='{}'::jsonb FROM admin_profile_presentations WHERE subject_id=$1", f.old.LocalSubject).Scan(&empty))
	require.True(t, empty, "authenticated presentation read lazily clears expired pending data")
}
