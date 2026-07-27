package data

import (
	"context"

	"github.com/google/uuid"
	"github.com/kairosedubf/wobsongo/dto"
)

// SupportsPagination is a marker interface for types that support pagination.
type SupportsPagination interface {
	Limit() int32
	Offset() int32
}

// Crudable is a generic interface for types that support CRUD operations.
type Crudable[T any] interface {
	// GetByID retrieves an entity of type T by its UUID.
	GetByID(ctx context.Context, id uuid.UUID) (*T, error)

	// Paginate retrieves a paginated list of entities of type T based on the provided pagination parameters.
	Paginate(ctx context.Context, q SupportsPagination) (*dto.PaginationResults[T], error)

	// Create creates a new entity of type T in the data store.
	Create(ctx context.Context, entity *T) error

	// Update updates an existing entity of type T in the data store.
	Update(ctx context.Context, entity *T) error

	// Delete removes an entity of type T from the data store by its UUID.
	Delete(ctx context.Context, id uuid.UUID) error
}

// CrudableWithTx is a generic interface for types that support CRUD operations with transaction support.
type CrudableWithTx[T any, R any] interface {
	Crudable[T]
	TxAware[R]
}
