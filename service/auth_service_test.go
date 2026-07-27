package service

import (
	"context"
	"errors"
	"testing"

	"github.com/kairosedubf/wobsongo/auth"
	"github.com/kairosedubf/wobsongo/data"
	"github.com/kairosedubf/wobsongo/mockrepo"
	"github.com/kairosedubf/wobsongo/model"
)

func newTestAuth() *auth.Auth {
	return auth.New("test-secret", 1)
}

func TestAuthService_Register_CreatesUserAndReturnsTokens(t *testing.T) {
	var created *model.User
	userRepo := &mockrepo.UserRepoerMock{
		CreateFunc: func(_ context.Context, user *model.User) error {
			created = user
			return nil
		},
	}

	s := NewAuthService(userRepo, newTestAuth())
	user, tokens, err := s.Register(t.Context(), "Alice", "alice@example.com", "hunter22")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if user.Role != model.RoleUser {
		t.Errorf("expected new registrations to default to RoleUser, got %s", user.Role)
	}
	if created == nil || created.Email != "alice@example.com" {
		t.Errorf("expected repo to receive the new user, got %+v", created)
	}
	if user.PasswordHash == "" || user.PasswordHash == "hunter22" {
		t.Error("expected password to be hashed before storing")
	}
	if tokens == nil || tokens.AccessToken == "" || tokens.RefreshToken == "" {
		t.Error("expected a non-empty token pair")
	}
}

func TestAuthService_Register_ConflictOnDuplicateEmail(t *testing.T) {
	userRepo := &mockrepo.UserRepoerMock{
		CreateFunc: func(context.Context, *model.User) error {
			return data.ErrConflict
		},
	}

	s := NewAuthService(userRepo, newTestAuth())
	_, _, err := s.Register(t.Context(), "Alice", "alice@example.com", "hunter22")
	if !errors.Is(err, data.ErrConflict) {
		t.Errorf("expected data.ErrConflict, got %v", err)
	}
}

func TestAuthService_Login_Success(t *testing.T) {
	hash, err := auth.HashPassword("hunter22", auth.WithMinCost())
	if err != nil {
		t.Fatalf("failed to hash password: %v", err)
	}
	userRepo := &mockrepo.UserRepoerMock{
		GetByEmailFunc: func(context.Context, string) (*model.User, error) {
			return &model.User{
				Email:        "alice@example.com",
				PasswordHash: hash,
				Role:         model.RoleAdmin,
			}, nil
		},
	}

	s := NewAuthService(userRepo, newTestAuth())
	tokens, err := s.Login(t.Context(), "alice@example.com", "hunter22")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if tokens == nil || tokens.AccessToken == "" {
		t.Error("expected a non-empty access token")
	}
}

func TestAuthService_Login_WrongPasswordMasksAsIncorrectCredentials(t *testing.T) {
	hash, err := auth.HashPassword("hunter22", auth.WithMinCost())
	if err != nil {
		t.Fatalf("failed to hash password: %v", err)
	}
	userRepo := &mockrepo.UserRepoerMock{
		GetByEmailFunc: func(context.Context, string) (*model.User, error) {
			return &model.User{Email: "alice@example.com", PasswordHash: hash}, nil
		},
	}

	s := NewAuthService(userRepo, newTestAuth())
	_, err = s.Login(t.Context(), "alice@example.com", "wrong-password")
	if !errors.Is(err, auth.ErrIncorrectCredentials) {
		t.Errorf("expected ErrIncorrectCredentials, got %v", err)
	}
}

func TestAuthService_Login_UnknownEmailMasksAsIncorrectCredentials(t *testing.T) {
	userRepo := &mockrepo.UserRepoerMock{
		GetByEmailFunc: func(context.Context, string) (*model.User, error) {
			return nil, data.ErrNotFound
		},
	}

	s := NewAuthService(userRepo, newTestAuth())
	_, err := s.Login(t.Context(), "nobody@example.com", "hunter22")
	if !errors.Is(err, auth.ErrIncorrectCredentials) {
		t.Errorf(
			"expected not-found to be masked as ErrIncorrectCredentials (no email enumeration), got %v",
			err,
		)
	}
}
