package main

import (
	"context"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/joho/godotenv"
)

func main() {
	godotenv.Load()

	dbURL := os.Getenv("DATABASE_URL")
	if dbURL == "" {
		log.Fatal("DATABASE_URL is not set")
	}

	ctx := context.Background()
	pool, err := pgxpool.New(ctx, dbURL)
	if err != nil {
		log.Fatalf("connect: %v", err)
	}
	defer pool.Close()

	if err := ensureTable(ctx, pool); err != nil {
		log.Fatalf("ensure schema_migrations: %v", err)
	}

	applied, err := appliedMigrations(ctx, pool)
	if err != nil {
		log.Fatalf("load applied: %v", err)
	}

	migrationsDir := filepath.Join(filepath.Dir(os.Args[0]), "../../migrations")
	if _, err := os.Stat(migrationsDir); err != nil {
		migrationsDir = "migrations"
	}

	files, err := filepath.Glob(filepath.Join(migrationsDir, "*.sql"))
	if err != nil {
		log.Fatalf("glob migrations: %v", err)
	}
	sort.Strings(files)

	for _, f := range files {
		name := filepath.Base(f)
		if applied[name] {
			fmt.Printf("skip  %s (already applied)\n", name)
			continue
		}

		sql, err := os.ReadFile(f)
		if err != nil {
			log.Fatalf("read %s: %v", f, err)
		}

		if err := runMigration(ctx, pool, name, string(sql)); err != nil {
			log.Fatalf("apply %s: %v", name, err)
		}
		fmt.Printf("apply %s\n", name)
	}

	fmt.Println("migrations: all done")
}

func ensureTable(ctx context.Context, pool *pgxpool.Pool) error {
	_, err := pool.Exec(ctx, `
		CREATE TABLE IF NOT EXISTS schema_migrations (
			filename   TEXT PRIMARY KEY,
			applied_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
		)
	`)
	return err
}

func appliedMigrations(ctx context.Context, pool *pgxpool.Pool) (map[string]bool, error) {
	rows, err := pool.Query(ctx, `SELECT filename FROM schema_migrations`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	applied := make(map[string]bool)
	for rows.Next() {
		var name string
		if err := rows.Scan(&name); err != nil {
			return nil, err
		}
		applied[name] = true
	}
	return applied, rows.Err()
}

func runMigration(ctx context.Context, pool *pgxpool.Pool, name, sql string) error {
	return pgx.BeginTxFunc(ctx, pool, pgx.TxOptions{}, func(tx pgx.Tx) error {
		if _, err := tx.Exec(ctx, sql); err != nil {
			return fmt.Errorf("exec SQL: %w", err)
		}
		if _, err := tx.Exec(ctx,
			`INSERT INTO schema_migrations(filename) VALUES($1)`, name,
		); err != nil {
			if strings.Contains(err.Error(), "duplicate key") {
				return nil
			}
			return fmt.Errorf("record migration: %w", err)
		}
		return nil
	})
}
