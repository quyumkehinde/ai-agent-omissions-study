package main

import (
	"context"
	"fmt"

	"github.com/jackc/pgx/v5"
)

const schema = `
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
`

var tables = []string{"customers", "subscriptions"}

func CreateSchema(ctx context.Context, conn *pgx.Conn) error {
	_, err := conn.Exec(ctx, schema)
	if err != nil {
		return fmt.Errorf("creating tables: %w", err)
	}
	return nil
}

func UpsertCustomers(ctx context.Context, conn *pgx.Conn, cs []Customer) error {
	b := &pgx.Batch{}
	for _, c := range cs {
		b.Queue(`INSERT INTO customers (id, object, email, name, created)
			VALUES ($1, $2, $3, $4, to_timestamp($5))
			ON CONFLICT (id) DO UPDATE SET
				object = EXCLUDED.object, email = EXCLUDED.email,
				name = EXCLUDED.name, created = EXCLUDED.created`,
			c.ID, c.Object, c.Email, c.Name, c.Created)
	}
	return sendBatch(ctx, conn, b)
}

func UpsertSubscriptions(ctx context.Context, conn *pgx.Conn, ss []Subscription) error {
	b := &pgx.Batch{}
	for _, s := range ss {
		b.Queue(`INSERT INTO subscriptions (id, object, customer, status, created)
			VALUES ($1, $2, $3, $4, to_timestamp($5))
			ON CONFLICT (id) DO UPDATE SET
				object = EXCLUDED.object, customer = EXCLUDED.customer,
				status = EXCLUDED.status, created = EXCLUDED.created`,
			s.ID, s.Object, s.Customer, s.Status, s.Created)
	}
	return sendBatch(ctx, conn, b)
}

// sendBatch runs the batch in a single transaction (pgx batches are implicitly transactional).
func sendBatch(ctx context.Context, conn *pgx.Conn, b *pgx.Batch) error {
	return conn.SendBatch(ctx, b).Close()
}

func Counts(ctx context.Context, conn *pgx.Conn) (map[string]int64, error) {
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
