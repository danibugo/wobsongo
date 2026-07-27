package data

import "context"

// TxAware is a generic interface for types that support transactions.
// T is the repo type that supports transactions.
type TxAware[T any] interface {
	// WithTx executes the given function within a transaction.
	WithTx(ctx context.Context, fn func(T) error) error
}
