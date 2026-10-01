// Package csrf signs admin CSRF proofs. Bootstrap owns request origin, cookie
// ambiguity and opaque-session binding checks.
package csrf

import (
	"context"
	"crypto/sha256"
	"errors"
	"fmt"
	"time"

	logger "github.com/assurrussa/gologger"
	"github.com/golang-jwt/jwt/v5"
	"github.com/google/uuid"

	"github.com/assurrussa/goadmin/internal/identity"
)

const (
	HeaderName   = "X-CSRF-Token"
	proofIssuer  = "goadmin-csrf-v1"
	proofPurpose = "browser-csrf"
	// A small, fixed skew allowance applies to iat, nbf and exp. In particular,
	// a proof expires at exp+5s, not at exp; hosts must still synchronize clocks.
	proofClockSkew = 5 * time.Second
)

var (
	ErrTokenInvalid    = errors.New("token is invalid")
	ErrTokenInvalidJWT = errors.New("token is invalid with JWT")
)

//go:generate options-gen -out-filename=service_options.gen.go -from-struct=Options
type Options struct {
	appDomain      string        `option:"mandatory" validate:"required"`
	secretKey      string        `option:"mandatory" validate:"required"`
	allowedOrigins []string      `option:"mandatory" validate:"required"`
	logger         logger.Logger `option:"mandatory" validate:"required"`
	tokenTTL       time.Duration `default:"24h"`
}

type Claims struct {
	SessionID string          `json:"sid"`
	UserID    identity.UserID `json:"sub"`
	Purpose   string          `json:"purpose"`
	jwt.RegisteredClaims
}

type Service struct {
	Options
	secretKeyBytes []byte
	parser         *jwt.Parser
	now            func() time.Time
}

func New(opts Options) (*Service, error) {
	return newWithClock(opts, time.Now)
}

// Keep the host-facing options unchanged. Tests inject independent immutable
// node clocks here; token issuance and verification use the same clock source.
func newWithClock(opts Options, now func() time.Time) (*Service, error) {
	if err := opts.Validate(); err != nil {
		return nil, fmt.Errorf("invalid options: %w", err)
	}
	if opts.tokenTTL <= 0 {
		return nil, errors.New("CSRF token lifetime must be positive")
	}
	return &Service{
		Options: opts, secretKeyBytes: []byte(opts.secretKey), now: now,
		parser: jwt.NewParser(
			jwt.WithValidMethods([]string{jwt.SigningMethodHS256.Alg()}),
			jwt.WithExpirationRequired(), jwt.WithIssuedAt(),
			jwt.WithLeeway(proofClockSkew), jwt.WithTimeFunc(now),
			jwt.WithIssuer(proofIssuer), jwt.WithAudience(opts.appDomain),
		),
	}, nil
}

func (s *Service) Create(_ context.Context, req Request) (Token, error) {
	if req.SessionID == "" {
		req.SessionID = generateAnonymousSession()
	}
	now := s.now()
	claims := &Claims{
		SessionID: req.SessionID, UserID: req.UserID, Purpose: proofPurpose,
		RegisteredClaims: jwt.RegisteredClaims{
			Issuer: proofIssuer, Audience: jwt.ClaimStrings{s.appDomain},
			ExpiresAt: jwt.NewNumericDate(now.Add(s.tokenTTL)), IssuedAt: jwt.NewNumericDate(now),
		},
	}
	tokenString, err := jwt.NewWithClaims(jwt.SigningMethodHS256, claims).SignedString(s.secretKeyBytes)
	if err != nil {
		return Token{}, fmt.Errorf("sign token: %w", err)
	}
	domain := s.appDomain
	if req.Domain != "" {
		domain = req.Domain
	}
	return Token{Domain: domain, Token: tokenString, SessionID: req.SessionID, UserID: req.UserID}, nil
}

func (s *Service) Check(_ context.Context, req Request, inputToken string) (*Claims, bool, error) {
	if err := req.Validate(); err != nil {
		return nil, false, fmt.Errorf("invalid request: %w", err)
	}
	claims, err := s.ExtractClaims(inputToken)
	if err != nil {
		return nil, false, fmt.Errorf("cannot parse CSRF proof: %w", errors.Join(err, ErrTokenInvalid, ErrTokenInvalidJWT))
	}
	if !s.checkClaims(claims, req) {
		return nil, false, ErrTokenInvalid
	}
	return claims, true, nil
}

func (s *Service) ExtractClaims(tokenString string) (*Claims, error) {
	if len(tokenString) > 8*1024 {
		return nil, ErrTokenInvalidJWT
	}
	token, err := s.parser.ParseWithClaims(tokenString, &Claims{}, s.keyFunc())
	if err != nil {
		return nil, fmt.Errorf("parse token: %w", err)
	}
	if claims, ok := token.Claims.(*Claims); ok && token.Valid &&
		claims.Purpose == proofPurpose && claims.ExpiresAt != nil && claims.IssuedAt != nil {
		return claims, nil
	}
	return nil, ErrTokenInvalid
}

func (s *Service) checkClaims(claims *Claims, req Request) bool {
	return claims.SessionID == req.SessionID && claims.UserID == req.UserID
}

func (s *Service) keyFunc() jwt.Keyfunc {
	return func(token *jwt.Token) (any, error) {
		if token.Method != jwt.SigningMethodHS256 {
			return nil, errors.New("unexpected CSRF signing method")
		}
		return s.secretKeyBytes, nil
	}
}

func generateAnonymousSession() string {
	tmp := uuid.New()
	hash := sha256.Sum256(tmp[:])
	return fmt.Sprintf("anon%x", hash)[:36]
}
