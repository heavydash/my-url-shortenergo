package migrations

import (
	"strings"

	_ "github.com/jackc/pgx/v5/stdlib"
	"github.com/pressly/goose/v3"
)

func RunMigrations(dsn string) error {
	db, err := goose.OpenDBWithDriver("pgx", dsn)
	if err != nil {
		return err
	}
	defer db.Close()

	err = goose.Up(db, "migrations")
	if err != nil {
		if strings.Contains(err.Error(), "no next version found") {
			return nil
		}
		return err
	}
	return nil
}
