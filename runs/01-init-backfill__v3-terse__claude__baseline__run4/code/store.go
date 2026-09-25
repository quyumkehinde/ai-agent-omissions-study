package main

import (
	"context"
	"fmt"

	"github.com/jackc/pgx/v5"
)

const schemaSQL = `
CREATE TABLE IF NOT EXISTS customers (
	id      text PRIMARY KEY,
	object  text NOT NULL,
	email   text,
	name    text,
	created timestamptz NOT NULL
);
CREATE TABLE IF NOT EXISTS subscriptions (
	id       text PRIMARY KEY,
	object   text NOT NULL,
	customer text NOT NULL,
	status   text NOT NULL,
	created  timestamptz NOT NULL
);
CREATE INDEX IF NOT EXISTS subscriptions_customer_idx ON subscriptions (customer);
`

var tables = []string{"customers", "subscriptions"}

func createSchema(ctx context.Context, conn *pgx.Conn) error {
	_, err := conn.Exec(ctx, schemaSQL)
	return err
}

func upsertCustomers(ctx context.Context, conn *pgx.Conn, cs []Customer) error {
	b := &pgx.Batch{}
	for _, c := range cs {
		b.Queue(`INSERT INTO customers (id, object, email, name, created)
			VALUES ($1, $2, $3, $4, to_timestamp($5))
			ON CONFLICT (id) DO UPDATE SET object = EXCLUDED.object, email = EXCLUDED.email,
				name = EXCLUDED.name, created = EXCLUDED.created`,
			c.ID, c.Object, c.Email, c.Name, c.Created)
	}
	return conn.SendBatch(ctx, b).Close()
}

func upsertSubscriptions(ctx context.Context, conn *pgx.Conn, ss []Subscription) error {
	b := &pgx.Batch{}
	for _, s := range ss {
		b.Queue(`INSERT INTO subscriptions (id, object, customer, status, created)
			VALUES ($1, $2, $3, $4, to_timestamp($5))
			ON CONFLICT (id) DO UPDATE SET object = EXCLUDED.object, customer = EXCLUDED.customer,
				status = EXCLUDED.status, created = EXCLUDED.created`,
			s.ID, s.Object, s.Customer, s.Status, s.Created)
	}
	return conn.SendBatch(ctx, b).Close()
}

// Backfill copies all customers and subscriptions (including canceled). It is idempotent.
func Backfill(ctx context.Context, c *Client, conn *pgx.Conn) (customers, subs int, err error) {
	err = c.ListCustomers(ctx, func(p []Customer) error {
		customers += len(p)
		return upsertCustomers(ctx, conn, p)
	})
	if err != nil {
		return customers, subs, fmt.Errorf("backfilling customers: %w", err)
	}
	err = c.ListSubscriptions(ctx, func(p []Subscription) error {
		subs += len(p)
		return upsertSubscriptions(ctx, conn, p)
	})
	if err != nil {
		return customers, subs, fmt.Errorf("backfilling subscriptions: %w", err)
	}
	return customers, subs, nil
}

func RowCounts(ctx context.Context, conn *pgx.Conn) (map[string]int64, error) {
	out := map[string]int64{}
	for _, t := range tables {
		var n int64
		if err := conn.QueryRow(ctx, "SELECT count(*) FROM "+t).Scan(&n); err != nil {
			return nil, fmt.Errorf("counting %s (has `mirror init` been run?): %w", t, err)
		}
		out[t] = n
	}
	return out, nil
}
