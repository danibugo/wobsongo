package auth

import "golang.org/x/crypto/bcrypt"

// defaultCost is the bcrypt cost factor used in production.
const defaultCost = 12

type hashOptions struct {
	cost int
}

type hashOption func(*hashOptions)

// WithMinCost sets bcrypt cost to the minimum (4).
// Use in tests and seed scripts to avoid slow hashing at high iteration counts.
func WithMinCost() hashOption {
	return func(opts *hashOptions) {
		opts.cost = bcrypt.MinCost
	}
}

// HashPassword hashes a plaintext password using bcrypt.
func HashPassword(password string, options ...hashOption) (string, error) {
	opts := &hashOptions{cost: defaultCost}
	for _, o := range options {
		o(opts)
	}
	hashed, err := bcrypt.GenerateFromPassword([]byte(password), opts.cost)
	if err != nil {
		return "", err
	}
	return string(hashed), nil
}

// ComparePasswords returns nil if the plaintext password matches the hash.
func ComparePasswords(password, hashed string) error {
	return bcrypt.CompareHashAndPassword([]byte(hashed), []byte(password))
}
