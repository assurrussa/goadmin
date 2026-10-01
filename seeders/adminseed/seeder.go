package adminseed

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/assurrussa/goauth"

	authadapter "github.com/assurrussa/goadmin/internal/auth"
	identity "github.com/assurrussa/goadmin/internal/identity"
)

const name = "adminseed"

type adapter interface {
	ProvisionTrustedAdmin(
		ctx context.Context,
		request goauth.RegisterRequest,
		publicID identity.UserID,
	) (goauth.Account, int64, error)
	Roles() *authadapter.RoleService
}

type Seed struct {
	adapter   adapter
	account   Account
	configErr error
}

type Account struct {
	Email    string
	Username string
	Name     string
	LastName string
	Password string
}

func NewSeed(auth adapter, accounts ...Account) *Seed {
	seed := &Seed{adapter: auth}
	if auth == nil {
		seed.configErr = errors.New("admin auth adapter is required")
		return seed
	}
	if len(accounts) != 1 {
		seed.configErr = errors.New("exactly one explicit admin seed account is required")
		return seed
	}
	seed.account = normalizeAccount(accounts[0])
	if seed.account.Email == "" || seed.account.Username == "" ||
		seed.account.Name == "" || seed.account.Password == "" {
		seed.configErr = errors.New("admin seed email, username, name, and password are required")
	}

	return seed
}

func (*Seed) Name() string { return name }

func (s *Seed) Handle(ctx context.Context) error {
	if s == nil {
		return errors.New("admin seed is nil")
	}
	if s.configErr != nil {
		return fmt.Errorf("seeder %s: %w", name, s.configErr)
	}
	roles := s.adapter.Roles()
	if roles == nil {
		return fmt.Errorf("seeder %s: roles service is required", name)
	}
	superRoles, err := roles.ListRoles(ctx, authadapter.RoleFilter{
		Slugs: []string{authadapter.SuperAdminRole}, IncludeSys: true,
	})
	if err != nil {
		return fmt.Errorf("seeder %s: load super admin role: %w", name, err)
	}
	if len(superRoles) != 1 {
		return fmt.Errorf(
			"seeder %s: super admin role %q not found; run rolesseed first",
			name,
			authadapter.SuperAdminRole,
		)
	}

	account, _, err := s.adapter.ProvisionTrustedAdmin(ctx, goauth.RegisterRequest{
		Email:    s.account.Email,
		Password: s.account.Password,
		Profile: goauth.BasicProfile{
			Username:    s.account.Username,
			DisplayName: strings.TrimSpace(s.account.Name + " " + s.account.LastName),
			GivenName:   s.account.Name,
			FamilyName:  s.account.LastName,
		},
	}, identity.NewUserID())
	if err != nil {
		return fmt.Errorf("seeder %s: provision trusted admin: %w", name, err)
	}
	if err := roles.AssignRolesToSubject(ctx, account.Subject.ID.String(), []int64{superRoles[0].ID}); err != nil {
		return fmt.Errorf("seeder %s: assign super admin role: %w", name, err)
	}

	return nil
}

func normalizeAccount(account Account) Account {
	account.Email = strings.TrimSpace(account.Email)
	account.Username = strings.TrimSpace(account.Username)
	account.Name = strings.TrimSpace(account.Name)
	account.LastName = strings.TrimSpace(account.LastName)

	return account
}
