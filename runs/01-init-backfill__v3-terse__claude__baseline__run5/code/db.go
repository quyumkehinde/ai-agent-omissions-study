package main

import (
	"context"
	"database/sql"
	"fmt"

	_ "github.com/jackc/pgx/v5/stdlib"
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
CREATE INDEX IF NOT EXISTS subscriptions_customer_idx ON subscriptions (customer);
`

func openDB(ctx context.Context, url string) (*sql.DB, error) {
	db, err := sql.Open("pgx", url)
	if err != nil {
		return nil, fmt.Errorf("opening database: %w", err)
	}
	if err := db.PingContext(ctx); err != nil {
		db.Close()
		return nil, fmt.Errorf("connecting to database: %w", err)
	}
	return db, nil
}

func createSchema(ctx context.Context, db *sql.DB) error {
	_, err := db.ExecContext(ctx, schema)
	return err
}

func upsertCustomers(ctx context.Context, db *sql.DB, cs []Customer) error {
	tx, err := db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	for _, c := range cs {
		_, err := tx.ExecContext(ctx, `
INSERT INTO customers (id, object, email, name, created)
VALUES ($1, $2, $3, $4, to_timestamp($5))
ON CONFLICT (id) DO UPDATE SET object = EXCLUDED.object, email = EXCLUDED.email,
	name = EXCLUDED.name, created = EXCLUDED.created`,
			c.ID, c.Object, c.Email, c.Name, c.Created)
		if err != nil {
			return fmt.Errorf("upserting customer %s: %w", c.ID, err)
		}
	}
	return tx.Commit()
}

func upsertSubscriptions(ctx context.Context, db *sql.DB, ss []Subscription) error {
	tx, err := db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	for _, s := range ss {
		_, err := tx.ExecContext(ctx, `
INSERT INTO subscriptions (id, object, customer, status, created)
VALUES ($1, $2, $3, $4, to_timestamp($5))
ON CONFLICT (id) DO UPDATE SET object = EXCLUDED.object, customer = EXCLUDED.customer,
	status = EXCLUDED.status, created = EXCLUDED.created`,
			s.ID, s.Object, s.Customer, s.Status, s.Created)
		if err != nil {
			return fmt.Errorf("upserting subscription %s: %w", s.ID, err)
		}
	}
	return tx.Commit()
}

// tableNames is fixed; never derived from user input.
var tableNames = []string{"customers", "subscriptions"}

func rowCounts(ctx context.Context, db *sql.DB) (map[string]int64, error) {
	out := map[string]int64{}
	for _, t := range tableNames {
		var n int64
		if err := db.QueryRowContext(ctx, "SELECT count(*) FROM "+t).Scan(&n); err != nil {
			return nil, fmt.Errorf("counting %s (has `mirror init` been run?): %w", t, err)
		}
		out[t] = n
	}
	return out, nil
}
