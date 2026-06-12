package postgres

import (
	"context"
	"os"
	"sync"

	"github.com/jackc/pgx/v5/pgxpool"
)

var (
	poolOnce sync.Once
	pool     *pgxpool.Pool
)

func GetPool() *pgxpool.Pool {
	poolOnce.Do(func() {
		var err error
		pool, err = pgxpool.New(context.Background(), os.Getenv("DATABASE_URL"))
		if err != nil {
			panic("postgres: failed to create pool: " + err.Error())
		}
	})
	return pool
}

func Ping(ctx context.Context) error {
	return GetPool().Ping(ctx)
}
