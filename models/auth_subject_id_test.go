package models_test

import (
	"testing"

	"github.com/stretchr/testify/require"

	authshared "github.com/assurrussa/goadmin/internal/auth"
	identity "github.com/assurrussa/goadmin/internal/identity"
	"github.com/assurrussa/goadmin/models"
)

func TestAdminAuthSubjectIDUsesSubjectIDOnly(t *testing.T) {
	t.Parallel()

	subjectID := authshared.NewSubjectID()
	admin := models.Admin{
		ID:        42,
		SubjectID: subjectID,
		UUID:      identity.NewUserID(),
	}
	require.Equal(t, subjectID, admin.AuthSubjectID())

	admin.SubjectID = authshared.SubjectIDNil
	require.Equal(t, authshared.SubjectIDNil, admin.AuthSubjectID())
}

func TestSessionAdminAuthSubjectIDUsesSubjectIDOnly(t *testing.T) {
	t.Parallel()

	subjectID := authshared.NewSubjectID()
	admin := &models.SessionAdmin{
		ID:        42,
		SubjectID: subjectID,
		UUID:      identity.NewUserID(),
	}
	require.Equal(t, subjectID, admin.AuthSubjectID())

	admin.SubjectID = authshared.SubjectIDNil
	require.Equal(t, authshared.SubjectIDNil, admin.AuthSubjectID())
}
