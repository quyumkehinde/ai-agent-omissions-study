package main

import (
	"context"
	"database/sql"
	"fmt"

	_ "github.com/lib/pq"
)

const schemaSQL = `
CREATE TABLE IF NOT EXISTS customers (
	id      TEXT PRIMARY KEY,
	object  TEXT NOT NULL,
	email   TEXT,
	name    TEXT,
	created BIGINT NOT NULL
);
CREATE TABLE IF NOT EXISTS subscriptions (
	id       TEXT PRIMARY KEY,
	object   TEXT NOT NULL,
	customer TEXT NOT NULL,
	status   TEXT NOT NULL,
	created  BIGINT NOT NULL
);
CREATE INDEX IF NOT EXISTS subscriptions_customer_idx ON subscriptions (customer);
`

var tables = []string{"customers", "subscriptions"}

func CreateSchema(ctx context.Context, db *sql.DB) error {
	_, err := db.ExecContext(ctx, schemaSQL)
	return err
}

func UpsertCustomers(ctx context.Context, db *sql.DB, cs []Customer) error {
	tx, err := db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	stmt, err := tx.PrepareContext(ctx, `
INSERT INTO customers (id, object, email, name, created) VALUES ($1,$2,$3,$4,$5)
ON CONFLICT (id) DO UPDATE SET object=EXCLUDED.object, email=EXCLUDED.email, name=EXCLUDED.name, created=EXCLUDED.created`)
	if err != nil {
		return err
	}
	defer stmt.Close()
	for _, c := range cs {
		if _, err := stmt.ExecContext(ctx, c.ID, c.Object, c.Email, c.Name, c.Created); err != nil {
			return fmt.Errorf("upserting customer %s: %w", c.ID, err)
		}
	}
	return tx.Commit()
}

func UpsertSubscriptions(ctx context.Context, db *sql.DB, ss []Subscription) error {
	tx, err := db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	stmt, err := tx.PrepareContext(ctx, `
INSERT INTO subscriptions (id, object, customer, status, created) VALUES ($1,$2,$3,$4,$5)
ON CONFLICT (id) DO UPDATE SET object=EXCLUDED.object, customer=EXCLUDED.customer, status=EXCLUDED.status, created=EXCLUDED.created`)
	if err != nil {
		return err
	}
	defer stmt.Close()
	for _, s := range ss {
		if _, err := stmt.ExecContext(ctx, s.ID, s.Object, s.Customer, s.Status, s.Created); err != nil {
			return fmt.Errorf("upserting subscription %s: %w", s.ID, err)
		}
	}
	return tx.Commit()
}

// Backfill copies all customers and subscriptions, returning the counts fetched.
func Backfill(ctx context.Context, c *Client, db *sql.DB) (customers, subs int, err error) {
	err = c.ListCustomers(ctx, func(p []Customer) error {
		customers += len(p)
		return UpsertCustomers(ctx, db, p)
	})
	if err != nil {
		return customers, subs, fmt.Errorf("backfilling customers: %w", err)
	}
	err = c.ListSubscriptions(ctx, func(p []Subscription) error {
		subs += len(p)
		return UpsertSubscriptions(ctx, db, p)
	})
	if err != nil {
		return customers, subs, fmt.Errorf("backfilling subscriptions: %w", err)
	}
	return customers, subs, nil
}

// Counts returns the row count for each mirrored table.
func Counts(ctx context.Context, db *sql.DB) (map[string]int64, error) {
	out := map[string]int64{}
	for _, t := range tables {
		var n int64
		if err := db.QueryRowContext(ctx, "SELECT count(*) FROM "+t).Scan(&n); err != nil {
			return nil, fmt.Errorf("counting %s (has `mirror init` been run?): %w", t, err)
		}
		out[t] = n
	}
	return out, nil
}
