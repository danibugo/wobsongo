package service

import (
	"context"
	"fmt"

	"github.com/kairosedubf/wobsongo/auth"
	"github.com/kairosedubf/wobsongo/data"
	"github.com/kairosedubf/wobsongo/model"
)

// AuthService handles registration and login for the web layer.
type AuthService struct {
	userRepo data.UserRepoer
	auth     *auth.Auth
}

// NewAuthService creates an AuthService.
func NewAuthService(userRepo data.UserRepoer, a *auth.Auth) *AuthService {
	return &AuthService{userRepo: userRepo, auth: a}
}

// Register creates a new user account with the default role and returns a
// JWT token pair. Returns data.ErrConflict if the email is already registered.
func (s *AuthService) Register(
	ctx context.Context,
	name, email, password string,
) (*model.User, *auth.JWTTokens, error) {
	user, err := s.createUser(ctx, name, email, password, model.RoleUser)
	if err != nil {
		return nil, nil, err
	}
	tokens, err := s.auth.GenerateJWTPair(&auth.JWTPayload{
		ID:    user.ID,
		Name:  user.Name,
		Email: user.Email,
		Role:  user.Role,
	})
	if err != nil {
		return nil, nil, fmt.Errorf("%w: %w", data.ErrInternal, err)
	}
	return user, tokens, nil
}

// CreateSuperadmin creates a new superadmin account — used only by the
// createsuperadmin CLI command to bootstrap the first operator account, never
// through self-registration. Returns data.ErrConflict if the email is already
// registered.
func (s *AuthService) CreateSuperadmin(
	ctx context.Context,
	name, email, password string,
) (*model.User, error) {
	return s.createUser(ctx, name, email, password, model.RoleSuperadmin)
}

func (s *AuthService) createUser(
	ctx context.Context,
	name, email, password string,
	role model.UserRole,
) (*model.User, error) {
	hash, err := auth.HashPassword(password)
	if err != nil {
		return nil, fmt.Errorf("%w: %w", data.ErrInternal, err)
	}
	user := &model.User{
		Name:         name,
		Email:        email,
		Role:         role,
		PasswordHash: hash,
	}
	if err := s.userRepo.Create(ctx, user); err != nil {
		return nil, err // already mapped by repo layer (ErrConflict, ErrInternal)
	}
	return user, nil
}

// Login validates credentials and returns a JWT token pair.
func (s *AuthService) Login(ctx context.Context, email, password string) (*auth.JWTTokens, error) {
	user, err := s.userRepo.GetByEmail(ctx, email)
	if err != nil {
		// Mask not-found as incorrect credentials to prevent email enumeration.
		return nil, auth.ErrIncorrectCredentials
	}
	if err := auth.ComparePasswords(password, user.PasswordHash); err != nil {
		return nil, auth.ErrIncorrectCredentials
	}
	tokens, err := s.auth.GenerateJWTPair(&auth.JWTPayload{
		ID:    user.ID,
		Name:  user.Name,
		Email: user.Email,
		Role:  user.Role,
	})
	if err != nil {
		return nil, err
	}
	return tokens, nil
}
