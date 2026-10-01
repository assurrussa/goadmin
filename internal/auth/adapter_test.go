package auth //nolint:testpackage // verifies private membership-gate composition

import (
	"context"
	"errors"
	"testing"

	"github.com/assurrussa/goauth"
	"github.com/stretchr/testify/require"

	identity "github.com/assurrussa/goadmin/internal/identity"
)

type membershipStub struct {
	allowed bool
	err     error
	calls   int
	subject goauth.SubjectID
}

func (m *membershipStub) HasAdminMembership(_ context.Context, subject goauth.SubjectID) (bool, error) {
	m.calls++
	m.subject = subject
	return m.allowed, m.err
}

func (*membershipStub) ProvisionAdminMembership(context.Context, goauth.Account, identity.UserID) (int64, error) {
	panic("not used")
}

func TestAdminMembershipGate(t *testing.T) {
	const staffRealm goauth.Realm = "staff"
	denied := errors.New("host gate denied")
	unavailable := errors.New("membership database unavailable")
	for _, tc := range []struct {
		name                           string
		realm                          goauth.Realm
		upstream                       bool
		upstreamErr                    error
		member                         bool
		memberErr                      error
		want                           error
		upstreamCalls, membershipCalls int
	}{
		{name: "custom without gate", realm: staffRealm, want: goauth.ErrMembershipDenied},
		{name: "custom denied", realm: staffRealm, upstream: true, upstreamErr: denied, want: denied, upstreamCalls: 1},
		{name: "custom allowed", realm: staffRealm, upstream: true, upstreamCalls: 1},
		{
			name: "admin denied upstream despite membership", realm: goauth.RealmAdmin,
			upstream: true, upstreamErr: denied, member: true, want: denied, upstreamCalls: 1,
		},
		{
			name: "admin allowed upstream without membership", realm: goauth.RealmAdmin,
			upstream: true, want: goauth.ErrMembershipDenied, upstreamCalls: 1, membershipCalls: 1,
		},
		{name: "admin allowed both", realm: goauth.RealmAdmin, upstream: true, member: true, upstreamCalls: 1, membershipCalls: 1},
		{name: "admin allowed without upstream", realm: goauth.RealmAdmin, member: true, membershipCalls: 1},
		{name: "admin denied without upstream", realm: goauth.RealmAdmin, want: goauth.ErrMembershipDenied, membershipCalls: 1},
		{name: "admin membership error", realm: goauth.RealmAdmin, memberErr: unavailable, want: unavailable, membershipCalls: 1},
		{name: "user no gate", realm: goauth.RealmUser},
		{name: "user retains goauth gate bypass", realm: goauth.RealmUser, upstream: true, upstreamErr: denied},
	} {
		t.Run(tc.name, func(t *testing.T) {
			memberships := &membershipStub{allowed: tc.member, err: tc.memberErr}
			account := goauth.Account{Subject: goauth.Subject{ID: goauth.SubjectID{1}}}
			ctx := t.Context()
			calls := 0
			var upstream goauth.MembershipGate
			if tc.upstream {
				upstream = goauth.MembershipGateFunc(func(actual context.Context, realm goauth.Realm, got goauth.Account) error {
					calls++
					require.Equal(t, ctx, actual)
					require.Equal(t, tc.realm, realm)
					require.Equal(t, account, got)
					return tc.upstreamErr
				})
			}
			err := adminMembershipGate(upstream, memberships).AllowMembership(ctx, tc.realm, account)
			if tc.want == nil {
				require.NoError(t, err)
			} else {
				require.ErrorIs(t, err, tc.want)
			}
			require.Equal(t, tc.upstreamCalls, calls)
			require.Equal(t, tc.membershipCalls, memberships.calls)
			if memberships.calls > 0 {
				require.Equal(t, account.Subject.ID, memberships.subject)
			}
		})
	}
}
