package cmd

import (
	"errors"
	"os"

	appconfig "github.com/kairosedubf/wobsongo/config"
	"github.com/kairosedubf/wobsongo/auth"
	"github.com/kairosedubf/wobsongo/data"
	"github.com/kairosedubf/wobsongo/db"
	"github.com/kairosedubf/wobsongo/repo"
	"github.com/kairosedubf/wobsongo/service"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/spf13/cobra"
)

var (
	createSuperadminEmail    string
	createSuperadminUsername string
	createSuperadminPassword string
	createSuperadminApply    bool
)

var createSuperadminCmd = &cobra.Command{
	Use:   "createsuperadmin",
	Short: "Bootstrap a superadmin account for the web dashboard",
	Long: "Creates a user with the superadmin role, bypassing self-registration's\n" +
		"default 'user' role — this is the only way to get a superadmin account.\n\n" +
		"Without --apply, this is a dry run: the password is validated and hashed,\n" +
		"but nothing is inserted.",
	Run: runCreateSuperadmin,
}

func init() {
	createSuperadminCmd.Flags().
		StringVar(&createSuperadminEmail, "email", "", "Superadmin email (required)")
	createSuperadminCmd.Flags().
		StringVar(&createSuperadminUsername, "username", "", "Superadmin display name (required)")
	createSuperadminCmd.Flags().
		StringVar(&createSuperadminPassword, "password", "", "Superadmin password (required)")
	createSuperadminCmd.Flags().
		BoolVar(&createSuperadminApply, "apply", false, "Actually create the account (default is a dry run)")

	_ = createSuperadminCmd.MarkFlagRequired("email")
	_ = createSuperadminCmd.MarkFlagRequired("username")
	_ = createSuperadminCmd.MarkFlagRequired("password")
}

func runCreateSuperadmin(cmd *cobra.Command, _ []string) {
	if !createSuperadminApply {
		cmd.Printf(
			"Dry run: would create superadmin (email=%q, name=%q)\n",
			createSuperadminEmail, createSuperadminUsername,
		)
		cmd.Println("Re-run with --apply to actually create the account.")
		return
	}

	ctx := cmd.Context()
	cfg := appconfig.NewConfig(EnvFile)

	pool, err := pgxpool.New(ctx, cfg.PostgresURI)
	if err != nil {
		cmd.PrintErrf("Failed to connect to database: %s\n", err.Error())
		os.Exit(1)
		return
	}
	defer pool.Close()

	userRepo := repo.NewUserRepo(db.New(pool), pool)
	authSvc := service.NewAuthService(userRepo, auth.New(cfg.JWTSecret, cfg.JWTExpiryHours))

	user, err := authSvc.CreateSuperadmin(
		ctx,
		createSuperadminUsername,
		createSuperadminEmail,
		createSuperadminPassword,
	)
	if err != nil {
		if errors.Is(err, data.ErrConflict) {
			cmd.PrintErrf("An account with email %q already exists.\n", createSuperadminEmail)
		} else {
			cmd.PrintErrf("Failed to create superadmin: %s\n", err.Error())
		}
		os.Exit(1)
		return
	}

	cmd.Printf("Created superadmin %s (email=%q, name=%q)\n", user.ID, user.Email, user.Name)
}
