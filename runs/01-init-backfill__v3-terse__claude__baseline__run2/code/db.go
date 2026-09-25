package main

import (
	"context"
	"fmt"

	"github.com/jackc/pgx/v5"
)

const schema = `
CREATE TABLE IF NOT EXISTS customers (
	id      TEXT PRIMARY KEY,
	object  TEXT NOT NULL,
	email   TEXT,
	name    TEXT,
	created TIMESTAMPTZ NOT NULL
);
CREATE TABLE IF NOT EXISTS subscriptions (
	id       TEXT PRIMARY KEY,
	object   TEXT NOT NULL,
	customer TEXT NOT NULL,
	status   TEXT NOT NULL,
	created  TIMESTAMPTZ NOT NULL
);
CREATE INDEX IF NOT EXISTS subscriptions_customer_idx ON subscriptions (customer);
`

var tables = []string{"customers", "subscriptions"}

func CreateSchema(ctx context.Context, conn *pgx.Conn) error {
	_, err := conn.Exec(ctx, schema)
	return err
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

func sendBatch(ctx context.Context, conn *pgx.Conn, b *pgx.Batch) error {
	// Batches run in an implicit transaction, so a page is all-or-nothing.
	return conn.SendBatch(ctx, b).Close()
}

func CountRows(ctx context.Context, conn *pgx.Conn, table string) (int64, error) {
	var n int64
	// table comes from the fixed list above, never user input.
	err := conn.QueryRow(ctx, fmt.Sprintf("SELECT count(*) FROM %s", pgx.Identifier{table}.Sanitize())).Scan(&n)
	return n, err
}
