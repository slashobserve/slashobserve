package db

import (
	"github.com/jackc/pgx/v5/pgxpool"
)

func (d *DB) Pool() *pgxpool.Pool {
	return d.pool
}
