package migrations

import (
	"log"
	"strings"

	_ "github.com/jackc/pgx/v5/stdlib"
	"github.com/pressly/goose/v3"
)

func RunMigrations(dsn string) error {
	db, err := goose.OpenDBWithDriver("pgx", dsn)
	if err != nil {
		return err
	}
	defer func() {
		if err = db.Close(); err != nil {
			log.Printf("error closing db: %v", err)
		}
	}()

	err = goose.Up(db, "migrations")
	if err != nil {
		if strings.Contains(err.Error(), "no next version found") {
			return nil
		}
		return err
	}
	return nil
}
