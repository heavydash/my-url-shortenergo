package migrations

import (
	"embed"
	"github.com/pressly/goose/v3"
)

var embedMigrations embed.FS

func RunMigrations(dsn string) error {
	goose.SetBaseFS(embedMigrations)

	db, err := goose.OpenDBWithDriver("postgres", dsn)
	if err != nil {
		return err
	}
	defer db.Close()

	return goose.Up(db, "migrations")
}
