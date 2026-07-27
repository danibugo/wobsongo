package webhandler

import (
	"net/http"

	"github.com/kairosedubf/wobsongo/config"
	authpkg "github.com/kairosedubf/wobsongo/auth"
	"github.com/kairosedubf/wobsongo/data"
	"github.com/kairosedubf/wobsongo/service"
	"github.com/labstack/echo/v4"
)

// WebRepos holds the repo implementations for the HTML layer.
type WebRepos struct {
	UserRepo data.UserRepoer
}

// RegisterWebRoutes mounts all HTML routes onto the given Echo group.
func RegisterWebRoutes(
	g *echo.Group,
	repos *WebRepos,
	jwtAuth *authpkg.Auth,
	cfg *config.Config,
) {
	authSvc := service.NewAuthService(repos.UserRepo, jwtAuth)

	// Public: auth pages (redirect to / if already logged in).
	authHandler := NewAuthHandler(authSvc, cfg)
	g.GET("/login", authHandler.loginPage)
	g.POST("/login", authHandler.loginPost, loginRateLimit(cfg))
	g.POST("/logout", authHandler.logoutPost)
	g.GET("/register", authHandler.registerPage)
	g.POST("/register", authHandler.registerPost)

	// All remaining routes require a valid session.
	protected := g.Group("", RequireAuthMiddleware())
	protected.GET("/", func(c echo.Context) error {
		return c.Redirect(http.StatusFound, "/dashboard")
	})

	dash := NewDashboardHandler()
	protected.GET("/dashboard", dash.dashboardPage)
}
