package common

import (
	"database/sql"

	_ "github.com/lib/pq"
)

func ConnectDB(cfg *Config) (*sql.DB, error) {
	return sql.Open("postgres", cfg.DatabaseURL)
}
