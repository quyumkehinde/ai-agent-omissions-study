package main

import (
	"context"
	"database/sql"
	"fmt"
)

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

func createTables(ctx context.Context, db *sql.DB) error {
	_, err := db.ExecContext(ctx, schema)
	return err
}

func upsertCustomers(ctx context.Context, db *sql.DB, cs []Customer) error {
	return inTx(ctx, db, func(tx *sql.Tx) error {
		for _, c := range cs {
			_, err := tx.ExecContext(ctx, `
INSERT INTO customers (id, object, email, name, created) VALUES ($1,$2,$3,$4,$5)
ON CONFLICT (id) DO UPDATE SET object=EXCLUDED.object, email=EXCLUDED.email, name=EXCLUDED.name, created=EXCLUDED.created`,
				c.ID, c.Object, c.Email, c.Name, c.Created)
			if err != nil {
				return err
			}
		}
		return nil
	})
}

func upsertSubscriptions(ctx context.Context, db *sql.DB, ss []Subscription) error {
	return inTx(ctx, db, func(tx *sql.Tx) error {
		for _, s := range ss {
			_, err := tx.ExecContext(ctx, `
INSERT INTO subscriptions (id, object, customer, status, created) VALUES ($1,$2,$3,$4,$5)
ON CONFLICT (id) DO UPDATE SET object=EXCLUDED.object, customer=EXCLUDED.customer, status=EXCLUDED.status, created=EXCLUDED.created`,
				s.ID, s.Object, s.Customer, s.Status, s.Created)
			if err != nil {
				return err
			}
		}
		return nil
	})
}

func inTx(ctx context.Context, db *sql.DB, fn func(*sql.Tx) error) error {
	tx, err := db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	if err := fn(tx); err != nil {
		tx.Rollback()
		return err
	}
	return tx.Commit()
}

func countRows(ctx context.Context, db *sql.DB, table string) (int64, error) {
	var n int64
	// table is always a package-internal constant.
	err := db.QueryRowContext(ctx, "SELECT count(*) FROM "+table).Scan(&n)
	if err != nil {
		return 0, fmt.Errorf("counting %s: %w (has `mirror init` been run?)", table, err)
	}
	return n, nil
}
