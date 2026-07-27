package handler

import (
	"github.com/kairosedubf/wobsongo/auth"
	"github.com/labstack/echo/v4"
)

// withAuthUser is a custom Echo context that carries the authenticated
// user's JWT claims, set once JWTParserMiddleware has validated a token.
type withAuthUser struct {
	echo.Context
	User *auth.JWTClaims
}

// JWTFromCookieMiddleware extracts a token from the named cookie and stores
// the raw token string in the context. Use on HTML route groups — the API
// group uses JWTFromHeaderMiddleware instead, if/when it needs auth.
func JWTFromCookieMiddleware(cookieName string) echo.MiddlewareFunc {
	return func(next echo.HandlerFunc) echo.HandlerFunc {
		return func(c echo.Context) error {
			if cookie, err := c.Cookie(cookieName); err == nil {
				c.Set(contextRequestToken, cookie.Value)
			}
			return next(c)
		}
	}
}

// JWTParserMiddleware validates the JWT stored in the context (by
// JWTFromCookieMiddleware or an equivalent). On success it wraps the
// context with withAuthUser so route handlers can access claims via
// ClaimsFromContext. A missing or invalid token is not an error here — it
// just leaves the request unauthenticated; route-level middleware (e.g.
// webhandler.RequireAuthMiddleware) decides whether that's allowed.
func JWTParserMiddleware(a *auth.Auth) echo.MiddlewareFunc {
	return func(next echo.HandlerFunc) echo.HandlerFunc {
		return func(c echo.Context) error {
			token, ok := c.Get(contextRequestToken).(string)
			if !ok || token == "" {
				return next(c)
			}
			claims, err := a.ValidateJWT(token, string(auth.AccessTokenSubject))
			if err != nil {
				return next(c)
			}
			return next(&withAuthUser{Context: c, User: claims})
		}
	}
}

// ClaimsFromContext returns the JWT claims stored by JWTParserMiddleware, or
// nil if the request is unauthenticated.
func ClaimsFromContext(c echo.Context) *auth.JWTClaims {
	if wau, ok := c.(*withAuthUser); ok {
		return wau.User
	}
	return nil
}
