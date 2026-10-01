package auth

import (
	"context"
	"errors"

	"github.com/assurrussa/goauth"
	"github.com/assurrussa/goauth/postgres"
)

type TrustedAccountRuntime interface {
	FindAccount(ctx context.Context, identifier goauth.IdentifierInput) (goauth.Account, error)
	ProvisionTrustedLocalAccount(ctx context.Context, request goauth.RegisterRequest) (goauth.Account, error)
	PrepareCredential(ctx context.Context, credential goauth.Credential) (*postgres.CredentialProof, error)
	RevalidateCredential(ctx context.Context, proof *postgres.CredentialProof) (goauth.Account, error)
}

// PrepareTrustedAccount admits and verifies existing credentials before the
// command transaction, then revalidates the opaque proof under the canonical
// row lock inside it. Fresh provisioning remains part of the command transaction.
func PrepareTrustedAccount(ctx context.Context, runtime TrustedAccountRuntime,
	request goauth.RegisterRequest,
) (func(context.Context) (goauth.Account, error), error) {
	identifier := goauth.IdentifierInput{Scheme: goauth.IdentifierSchemeEmail, Value: request.Email}
	account, err := runtime.FindAccount(ctx, identifier)
	if errors.Is(err, goauth.ErrAccountNotFound) {
		return func(ctx context.Context) (goauth.Account, error) {
			return runtime.ProvisionTrustedLocalAccount(ctx, request)
		}, nil
	}
	if err != nil {
		return nil, err
	}
	if !account.EmailVerified() {
		return nil, goauth.ErrInvalidCredentials
	}
	proof, err := runtime.PrepareCredential(ctx, goauth.Credential{Identifier: identifier, Password: request.Password})
	if err != nil {
		return nil, err
	}
	return func(ctx context.Context) (goauth.Account, error) {
		account, err := runtime.RevalidateCredential(ctx, proof)
		if err != nil {
			return goauth.Account{}, err
		}
		if !account.EmailVerified() {
			return goauth.Account{}, goauth.ErrInvalidCredentials
		}
		return account, nil
	}, nil
}
