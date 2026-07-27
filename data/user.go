package data

import (
	"context"

	"github.com/google/uuid"
	"github.com/kairosedubf/wobsongo/model"
)

// UserRepoer defines the interface for interacting with user data storage.
// Deliberately not CrudableWithTx — the web layer's auth flows only need
// Create/lookup/role-update, not pagination, generic Update, or Delete.
type UserRepoer interface {
	// Create inserts a new user. Returns ErrConflict if the email is
	// already registered.
	Create(ctx context.Context, user *model.User) error

	// GetByID retrieves a user by ID. Returns ErrNotFound if none exists.
	GetByID(ctx context.Context, id uuid.UUID) (*model.User, error)

	// GetByEmail retrieves a user by email. Returns ErrNotFound if none exists.
	GetByEmail(ctx context.Context, email string) (*model.User, error)

	// UpdateRole changes a user's role. Returns ErrNotFound if no user has that ID.
	UpdateRole(ctx context.Context, id uuid.UUID, role model.UserRole) (*model.User, error)
}
