package auth

import "errors"

var (
	ErrIncorrectCredentials = errors.New("auth: incorrect credentials")
	ErrTokenExpired         = errors.New("auth: token expired")
	ErrInvalidToken         = errors.New("auth: invalid token")
	ErrWrongTokenType       = errors.New("auth: wrong token type")
)
