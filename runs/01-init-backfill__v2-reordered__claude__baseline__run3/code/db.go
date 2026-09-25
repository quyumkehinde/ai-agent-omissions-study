package main

import (
	"context"
	"errors"
	"fmt"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
)

var tables = []string{"customers", "subscriptions"}

const schema = `
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
);`

func CreateSchema(ctx context.Context, db *pgx.Conn) error {
	_, err := db.Exec(ctx, schema)
	return err
}

func upsertCustomers(ctx context.Context, db *pgx.Conn, cs []Customer) error {
	b := &pgx.Batch{}
	for _, c := range cs {
		b.Queue(`INSERT INTO customers (id, object, email, name, created) VALUES ($1,$2,$3,$4,$5)
			ON CONFLICT (id) DO UPDATE SET object=EXCLUDED.object, email=EXCLUDED.email, name=EXCLUDED.name, created=EXCLUDED.created`,
			c.ID, c.Object, c.Email, c.Name, c.Created)
	}
	return db.SendBatch(ctx, b).Close()
}

func upsertSubscriptions(ctx context.Context, db *pgx.Conn, ss []Subscription) error {
	b := &pgx.Batch{}
	for _, s := range ss {
		b.Queue(`INSERT INTO subscriptions (id, object, customer, status, created) VALUES ($1,$2,$3,$4,$5)
			ON CONFLICT (id) DO UPDATE SET object=EXCLUDED.object, customer=EXCLUDED.customer, status=EXCLUDED.status, created=EXCLUDED.created`,
			s.ID, s.Object, s.Customer, s.Status, s.Created)
	}
	return db.SendBatch(ctx, b).Close()
}

// Backfill imports every customer and subscription from the API. It is idempotent.
func Backfill(ctx context.Context, c *Client, db *pgx.Conn) (customers, subs int, err error) {
	err = c.ListCustomers(ctx, func(p []Customer) error {
		customers += len(p)
		return upsertCustomers(ctx, db, p)
	})
	if err != nil {
		return customers, subs, fmt.Errorf("importing customers: %w", err)
	}
	err = c.ListSubscriptions(ctx, func(p []Subscription) error {
		subs += len(p)
		return upsertSubscriptions(ctx, db, p)
	})
	if err != nil {
		return customers, subs, fmt.Errorf("importing subscriptions: %w", err)
	}
	return customers, subs, nil
}

// Counts returns the row count of each table in `tables`.
func Counts(ctx context.Context, db *pgx.Conn) (map[string]int64, error) {
	out := map[string]int64{}
	for _, t := range tables {
		var n int64
		if err := db.QueryRow(ctx, "SELECT count(*) FROM "+t).Scan(&n); err != nil {
			var pe *pgconn.PgError
			if errors.As(err, &pe) && pe.Code == "42P01" {
				return nil, fmt.Errorf("table %q does not exist; run `mirror init` first", t)
			}
			return nil, err
		}
		out[t] = n
	}
	return out, nil
}
