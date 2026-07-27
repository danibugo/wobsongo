package cmd

import (
	"context"
	"fmt"
	"os"
	"strings"

	appconfig "github.com/kairosedubf/wobsongo/config"
	migrator "github.com/kairosedubf/wobsongo/sql/migrations"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/riverqueue/river/riverdriver/riverpgxv5"
	"github.com/riverqueue/river/rivermigrate"
	"github.com/spf13/cobra"
)

// runRiverMigrationUp runs River migrations to the latest version.
func runRiverMigrationUp(cmd *cobra.Command, pool *pgxpool.Pool) error {
	cmd.Println("\n=== Running River migrations ===")

	riverMigrator, err := rivermigrate.New(riverpgxv5.New(pool), nil)
	if err != nil {
		return fmt.Errorf("failed to create River migrator: %w", err)
	}

	// Migrate to latest version
	res, err := riverMigrator.Migrate(
		context.Background(),
		rivermigrate.DirectionUp,
		&rivermigrate.MigrateOpts{},
	)
	if err != nil {
		return fmt.Errorf("failed to run River migrations: %w", err)
	}

	if len(res.Versions) == 0 {
		cmd.Println("River schema is already up to date")
	} else {
		cmd.Printf("Applied %d River migration(s)\n", len(res.Versions))
		for _, v := range res.Versions {
			cmd.Printf("  - Version %d\n", v.Version)
		}
	}

	return nil
}

// runRiverMigrationDown is intentionally not used in migratedown command.
// We keep River schema intact during application rollbacks for safety.
// This function is preserved for future use if needed.
//
//nolint:unused
func runRiverMigrationDown(cmd *cobra.Command, pool *pgxpool.Pool) error {
	cmd.Println("\n=== Rolling back River migrations ===")

	riverMigrator, err := rivermigrate.New(riverpgxv5.New(pool), nil)
	if err != nil {
		return fmt.Errorf("failed to create River migrator: %w", err)
	}

	// Migrate down by 1 step (default behavior)
	res, err := riverMigrator.Migrate(
		context.Background(),
		rivermigrate.DirectionDown,
		&rivermigrate.MigrateOpts{MaxSteps: 1},
	)
	if err != nil {
		return fmt.Errorf("failed to roll back River migrations: %w", err)
	}

	if len(res.Versions) == 0 {
		cmd.Println("No River migrations to roll back")
	} else {
		cmd.Printf("Rolled back %d River migration(s)\n", len(res.Versions))
		for _, v := range res.Versions {
			cmd.Printf("  - Version %d\n", v.Version)
		}
	}

	return nil
}

// resetRiverSchema completely removes all River tables and data.
func resetRiverSchema(cmd *cobra.Command, pool *pgxpool.Pool) error {
	cmd.Println("\n=== Dropping River schema ===")

	riverMigrator, err := rivermigrate.New(riverpgxv5.New(pool), nil)
	if err != nil {
		return fmt.Errorf("failed to create River migrator: %w", err)
	}

	// TargetVersion -1 removes all River schema
	res, err := riverMigrator.Migrate(
		context.Background(),
		rivermigrate.DirectionDown,
		&rivermigrate.MigrateOpts{TargetVersion: -1},
	)
	if err != nil {
		return fmt.Errorf("failed to drop River schema: %w", err)
	}

	cmd.Printf("Dropped %d River migration(s)\n", len(res.Versions))
	return nil
}

var migrateUpCmd = &cobra.Command{
	Use:   "migrateup",
	Short: "Runs the UP migrations",
	Long:  "Runs all unapplied migrations up to the latest version.",
	Run: func(cmd *cobra.Command, _ []string) {
		cfg := appconfig.NewConfig(EnvFile)
		pool, err := pgxpool.New(context.Background(), cfg.PostgresURI)
		if err != nil {
			panic(err)
		}
		defer pool.Close()

		mg, err := migrator.NewMigrator(cfg)
		if err != nil {
			cmd.PrintErrln("error when creating migrator", err.Error())
			os.Exit(1)
			return
		}

		if cfg.IsTesting() {
			cmd.Println("Skipping confirmation for testing environment")

			// Run River migrations first
			if err := runRiverMigrationUp(cmd, pool); err != nil {
				cmd.PrintErrf("failed to run River migrations: %s\n", err.Error())
				os.Exit(1)
			}

			// Then run application migrations
			cmd.Println("\n=== Running application migrations ===")
			err := mg.MigrateUp()
			if err != nil {
				cmd.PrintErrf("failed to run UP migrations: %s\n", err.Error())
				os.Exit(1)
			} else {
				cmd.Println("UP migrations run successfully!")
				v, d, _ := mg.CheckMigrationVersion()
				cmd.Printf("Current applied migration version: %d\n", v)
				if d {
					cmd.Println("=== Database is in dirty state, needs fixing ===")
				}
				os.Exit(0)
			}
		}
		v, d, _ := mg.CheckMigrationVersion()
		var confirm string
		cmd.Printf("Current applied migration version: %d\n", v)
		if d {
			cmd.Println("=== Database is in dirty state, needs fixing ===")
		}
		cmd.Println(
			"You're about to run River migrations and application migrations to the latest version, are you sure?",
		)
		cmd.Println("(type 'yes' to continue otherwise will exit)")
		if _, err := fmt.Scanln(&confirm); err != nil {
			os.Exit(1)
		}
		if !strings.EqualFold(confirm, "yes") {
			fmt.Println("UP migration aborted")
			os.Exit(0)
		}

		// Run River migrations first
		if err := runRiverMigrationUp(cmd, pool); err != nil {
			cmd.PrintErrf("failed to run River migrations: %s\n", err.Error())
			os.Exit(1)
		}

		// Then run application migrations
		cmd.Println("\n=== Running application migrations ===")
		if err = mg.MigrateUp(); err != nil {
			cmd.PrintErrf("failed to run UP migrations: %s\n", err.Error())
			os.Exit(1)
		} else {
			cmd.Println("UP migrations run successfully!")
			os.Exit(0)
		}
	},
}

var migrateDownCmd = &cobra.Command{
	Use:   "migratedown",
	Short: "Runs the DOWN migration to the previous version.",
	Long:  "Runs the DOWN migration to the previous version. Effectively undoes the previous UP migration.",
	Run: func(cmd *cobra.Command, _ []string) {
		cfg := appconfig.NewConfig(EnvFile)
		pool, err := pgxpool.New(context.Background(), cfg.PostgresURI)
		if err != nil {
			panic(err)
		}
		defer pool.Close()

		mg, err := migrator.NewMigrator(cfg)
		if err != nil {
			cmd.PrintErrln("error when creating migrator", err.Error())
			os.Exit(1)
			return
		}

		v, d, _ := mg.CheckMigrationVersion()
		var confirm string
		cmd.Printf("Current applied migration version: %d\n", v)
		if d {
			cmd.Println("=== Database is in dirty state, needs fixing ===")
		}
		cmd.Printf(
			"You're about to run migration DOWN by 1 version (to version %d), are you sure?\n",
			v-1,
		)
		cmd.Println(
			"Note: River schema will NOT be affected (only application migrations will be rolled back)",
		)
		cmd.Println("(type 'yes' to continue otherwise will exit)")
		if _, err := fmt.Scanln(&confirm); err != nil {
			os.Exit(1)
		}
		if !strings.EqualFold(confirm, "yes") {
			cmd.Println("DOWN migration aborted")
			os.Exit(0)
		}

		// Only roll back application migrations, leave River schema intact
		cmd.Println("\n=== Rolling back application migrations ===")
		if err := mg.MigrateDown(); err != nil {
			cmd.PrintErrf("failed to run DOWN migrations: %s\n", err.Error())
			os.Exit(1)
		} else {
			cmd.Println("DOWN migrations run successfully!")
			cmd.Println("(River schema unchanged)")
			os.Exit(0)
		}
	},
}

var resetCmd = &cobra.Command{
	Use:   "reset",
	Short: "Drops everything in the database. Only works on non-production environments.",
	Run: func(cmd *cobra.Command, _ []string) {
		cfg := appconfig.NewConfig(EnvFile)
		if cfg.IsProduction() {
			cmd.PrintErr("Cannot reset on production environment! Exiting.")
			return
		}

		fmt.Printf("Config env: %s\n", cfg.Env)

		// Create database pool for River migrations
		pool, err := pgxpool.New(context.Background(), cfg.PostgresURI)
		if err != nil {
			cmd.PrintErrf("Failed to connect to database: %s\n", err.Error())
			os.Exit(1)
			return
		}
		defer pool.Close()

		mg, err := migrator.NewMigrator(cfg)
		if err != nil {
			cmd.PrintErrln("error when creating migrator", err.Error())
			os.Exit(1)
			return
		}

		if cfg.IsTesting() {
			// Drop River schema first
			if err := resetRiverSchema(cmd, pool); err != nil {
				cmd.PrintErrf("failed to drop River schema: %s\n", err.Error())
				os.Exit(1)
			}

			// Then drop application schema
			cmd.Println("\n=== Dropping application schema ===")
			err := mg.Reset()
			if err != nil {
				cmd.PrintErrf("failed to DROP: %s\n", err.Error())
				os.Exit(1)
			} else {
				cmd.Println("The database has been reset")
				os.Exit(0)
			}
		}
		v, d, _ := mg.CheckMigrationVersion()
		var confirm string
		var confirm2 string
		cmd.Printf("Current applied migration version: %d\n", v)
		if d {
			cmd.Println("=== Database is in dirty state, needs fixing ===")
		}
		cmd.Println(
			"You're about to DROP EVERYTHING on the database (including River tables), are you ABSOLUTELY sure?",
		)
		cmd.Println("(type 'yes' to continue otherwise will exit)")
		if _, err := fmt.Scanln(&confirm); err != nil {
			os.Exit(1)
		}
		if !strings.EqualFold(confirm, "yes") {
			cmd.Println("DOWN migration aborted")
			os.Exit(0)
		}

		cmd.Println("But I have to ask again, are you ABSOLUTELY sure?")
		cmd.Println("(type 'yes' to continue otherwise will exit)")
		if _, err := fmt.Scanln(&confirm2); err != nil {
			os.Exit(1)
		}
		if !strings.EqualFold(confirm2, "yes") {
			cmd.Println("DOWN migration aborted")
			os.Exit(0)
		}

		// Drop River schema first
		if err := resetRiverSchema(cmd, pool); err != nil {
			cmd.PrintErrf("failed to drop River schema: %s\n", err.Error())
			os.Exit(1)
		}

		// Then drop application schema
		cmd.Println("\n=== Dropping application schema ===")
		if err := mg.Reset(); err != nil {
			cmd.PrintErrf("failed to DROP: %s\n", err.Error())
			os.Exit(1)
		} else {
			cmd.Println("The database has been reset")
			os.Exit(0)
		}
	},
}
