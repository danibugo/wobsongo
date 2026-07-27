package migrator

import (
	"embed"
	"errors"

	"github.com/golang-migrate/migrate/v4"
	_ "github.com/golang-migrate/migrate/v4/database/postgres"
	"github.com/golang-migrate/migrate/v4/source/iofs"
	"github.com/kairosedubf/wobsongo/config"
)

//go:embed *.sql
var migrations embed.FS

type Migrator struct {
	conf *config.Config
	migr *migrate.Migrate
}

func NewMigrator(cfg *config.Config) (*Migrator, error) {
	if cfg == nil {
		cfg = config.NewConfig()
	}
	source, err := iofs.New(migrations, ".")
	if err != nil {
		return nil, err
	}
	migrator, err := migrate.NewWithSourceInstance("iofs", source, cfg.PostgresURI)
	if err != nil {
		return nil, err
	}
	mg := &Migrator{
		conf: cfg,
		migr: migrator,
	}
	return mg, nil
}

func (m *Migrator) CheckMigrationVersion() (uint, bool, error) {
	return m.migr.Version()
}

func (m *Migrator) MigrateUp() error {
	if err := m.migr.Up(); err != nil {
		return err
	}
	return nil
}

func (m *Migrator) MigrateDown() error {
	if err := m.migr.Steps(-1); err != nil {
		return err
	}
	return nil
}

func (m *Migrator) Reset() error {
	if m.conf.IsProduction() {
		return errors.New("cannot reset on production environment")
	}
	if err := m.migr.Drop(); err != nil {
		return err
	}
	return nil
}
