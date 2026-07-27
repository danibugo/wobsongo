// Package auth provides JWT generation/validation for wobsongo's web layer.
package auth

import (
	"errors"
	"time"

	"github.com/golang-jwt/jwt/v5"
	"github.com/google/uuid"
	"github.com/kairosedubf/wobsongo/model"
)

const tokenIssuer = "github.com/kairosedubf/wobsongo"

// TokenSubject identifies the purpose of a JWT.
type TokenSubject string

const (
	AccessTokenSubject  TokenSubject = "wobsongo-access"
	RefreshTokenSubject TokenSubject = "wobsongo-refresh"
)

// JWTClaims are the claims embedded in every wobsongo web-layer JWT.
type JWTClaims struct {
	UserID uuid.UUID      `json:"user_id"`
	Name   string         `json:"name"`
	Email  string         `json:"email"`
	Role   model.UserRole `json:"role"`
	jwt.RegisteredClaims
}

// JWTTokens holds an access/refresh token pair returned after login or registration.
type JWTTokens struct {
	AccessToken  string `json:"access_token"`
	RefreshToken string `json:"refresh_token"`
}

// JWTPayload is the input for generating JWT tokens.
type JWTPayload struct {
	ID     uuid.UUID
	Name   string
	Email  string
	Role   model.UserRole
	AuthID uuid.UUID
}

// Auth holds the signing secret and token expiry used for all JWT operations.
type Auth struct {
	secret      []byte
	expiryHours int
}

// New creates an Auth instance. expiryHours controls access token lifetime.
func New(secret string, expiryHours int) *Auth {
	if expiryHours <= 0 {
		expiryHours = 24
	}
	return &Auth{secret: []byte(secret), expiryHours: expiryHours}
}

// GenerateJWTPair issues a new access + refresh token pair for the given payload.
func (a *Auth) GenerateJWTPair(payload *JWTPayload) (*JWTTokens, error) {
	id, err := uuid.NewV7()
	if err != nil {
		id = uuid.New()
	}
	payload.AuthID = id

	accessToken, err := a.generateAccessJWT(payload)
	if err != nil {
		return nil, err
	}
	refreshToken, err := a.generateRefreshJWT(payload)
	if err != nil {
		return nil, err
	}
	return &JWTTokens{AccessToken: accessToken, RefreshToken: refreshToken}, nil
}

// ValidateJWT parses and validates a signed token. Optionally checks its subject.
func (a *Auth) ValidateJWT(signedToken string, purposes ...string) (*JWTClaims, error) {
	token, err := jwt.ParseWithClaims(
		signedToken,
		new(JWTClaims),
		func(token *jwt.Token) (any, error) {
			if _, ok := token.Method.(*jwt.SigningMethodHMAC); !ok {
				return nil, errors.New("unexpected signing method")
			}
			return a.secret, nil
		},
	)
	if err != nil {
		if errors.Is(err, jwt.ErrTokenExpired) {
			return nil, ErrTokenExpired
		}
		return nil, ErrInvalidToken
	}

	claims, ok := token.Claims.(*JWTClaims)
	if !ok || !token.Valid {
		return nil, ErrInvalidToken
	}
	if claims.Issuer != tokenIssuer {
		return nil, ErrInvalidToken
	}
	if len(purposes) > 0 && claims.Subject != purposes[0] {
		return nil, ErrWrongTokenType
	}
	return claims, nil
}

func (a *Auth) generateAccessJWT(payload *JWTPayload) (string, error) {
	expiry := time.Now().Add(time.Duration(a.expiryHours) * time.Hour)
	claims := &JWTClaims{
		UserID: payload.ID,
		Name:   payload.Name,
		Email:  payload.Email,
		Role:   payload.Role,
		RegisteredClaims: jwt.RegisteredClaims{
			Issuer:    tokenIssuer,
			Subject:   string(AccessTokenSubject),
			IssuedAt:  jwt.NewNumericDate(time.Now()),
			NotBefore: jwt.NewNumericDate(time.Now()),
			ExpiresAt: jwt.NewNumericDate(expiry),
			ID:        payload.AuthID.String(),
		},
	}
	return a.sign(claims)
}

func (a *Auth) generateRefreshJWT(payload *JWTPayload) (string, error) {
	claims := &JWTClaims{
		UserID: payload.ID,
		Name:   payload.Name,
		Email:  payload.Email,
		Role:   payload.Role,
		RegisteredClaims: jwt.RegisteredClaims{
			Issuer:    tokenIssuer,
			Subject:   string(RefreshTokenSubject),
			IssuedAt:  jwt.NewNumericDate(time.Now()),
			NotBefore: jwt.NewNumericDate(time.Now()),
			ID:        payload.AuthID.String(),
		},
	}
	return a.sign(claims)
}

func (a *Auth) sign(claims *JWTClaims) (string, error) {
	token := jwt.NewWithClaims(jwt.SigningMethodHS256, claims)
	return token.SignedString(a.secret)
}
