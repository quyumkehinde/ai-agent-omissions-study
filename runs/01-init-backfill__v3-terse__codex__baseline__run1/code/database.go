package main

import (
	"context"
	"database/sql"
	"fmt"
	"io"

	_ "github.com/jackc/pgx/v5/stdlib"
)

func openPostgres(url string) (*sql.DB, error) {
	db, err := sql.Open("pgx", url)
	if err != nil {
		return nil, err
	}
	return db, nil
}

func createTables(ctx context.Context, db *sql.DB) error {
	_, err := db.ExecContext(ctx, `
CREATE TABLE IF NOT EXISTS customers (
  id TEXT PRIMARY KEY,
  object TEXT NOT NULL,
  email TEXT NOT NULL,
  name TEXT NOT NULL,
  created BIGINT NOT NULL
);
CREATE TABLE IF NOT EXISTS subscriptions (
  id TEXT PRIMARY KEY,
  object TEXT NOT NULL,
  customer TEXT NOT NULL,
  status TEXT NOT NULL,
  created BIGINT NOT NULL
);`)
	if err != nil {
		return fmt.Errorf("create tables: %w", err)
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

type store interface {
	upsertCustomer(context.Context, customer) error
	upsertSubscription(context.Context, subscription) error
}

type sqlStore struct{ db *sql.DB }

func (s sqlStore) upsertCustomer(ctx context.Context, c customer) error {
	_, err := s.db.ExecContext(ctx, `INSERT INTO customers (id, object, email, name, created)
VALUES ($1, $2, $3, $4, $5)
ON CONFLICT (id) DO UPDATE SET object = EXCLUDED.object, email = EXCLUDED.email,
name = EXCLUDED.name, created = EXCLUDED.created`, c.ID, c.Object, c.Email, c.Name, c.Created)
	return err
}

func (s sqlStore) upsertSubscription(ctx context.Context, sub subscription) error {
	_, err := s.db.ExecContext(ctx, `INSERT INTO subscriptions (id, object, customer, status, created)
VALUES ($1, $2, $3, $4, $5)
ON CONFLICT (id) DO UPDATE SET object = EXCLUDED.object, customer = EXCLUDED.customer,
status = EXCLUDED.status, created = EXCLUDED.created`, sub.ID, sub.Object, sub.Customer, sub.Status, sub.Created)
	return err
}
