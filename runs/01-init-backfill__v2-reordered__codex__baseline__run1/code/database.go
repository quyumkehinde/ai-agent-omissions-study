package main

import (
	"context"
	"database/sql"
	"fmt"
	"io"

	_ "github.com/lib/pq"
)

func openPostgres(url string) (*sql.DB, error) { return sql.Open("postgres", url) }

func createTables(ctx context.Context, db *sql.DB) error {
	const customers = `CREATE TABLE IF NOT EXISTS customers (
  id TEXT PRIMARY KEY, object TEXT NOT NULL, email TEXT NOT NULL, name TEXT NOT NULL, created BIGINT NOT NULL
)`
	const subscriptions = `CREATE TABLE IF NOT EXISTS subscriptions (
  id TEXT PRIMARY KEY, object TEXT NOT NULL, customer TEXT NOT NULL, status TEXT NOT NULL, created BIGINT NOT NULL
)`
	if _, err := db.ExecContext(ctx, customers); err != nil {
		return fmt.Errorf("create customers table: %w", err)
	}
	if _, err := db.ExecContext(ctx, subscriptions); err != nil {
		return fmt.Errorf("create subscriptions table: %w", err)
	}
	return nil
}

func printStatus(ctx context.Context, db *sql.DB, out io.Writer) error {
	for _, table := range []string{"customers", "subscriptions"} {
		var count int64
		if err := db.QueryRowContext(ctx, "SELECT COUNT(*) FROM "+table).Scan(&count); err != nil {
			return fmt.Errorf("count %s: %w", table, err)
		}
		if _, err := fmt.Fprintf(out, "%s: %d\n", table, count); err != nil {
			return err
		}
	}
	return nil
}
