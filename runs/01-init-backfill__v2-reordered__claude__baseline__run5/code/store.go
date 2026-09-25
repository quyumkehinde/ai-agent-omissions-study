package main

import (
	"context"
	"database/sql"
	"fmt"
	"time"
)

var tables = []string{"customers", "subscriptions"}

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
	customer TEXT,
	status   TEXT,
	created  TIMESTAMPTZ NOT NULL
);`

type Store struct{ DB *sql.DB }

func (s *Store) CreateTables(ctx context.Context) error {
	_, err := s.DB.ExecContext(ctx, schema)
	return err
}

func (s *Store) UpsertCustomers(ctx context.Context, cs []Customer) error {
	return s.inTx(ctx, func(tx *sql.Tx) error {
		for _, c := range cs {
			_, err := tx.ExecContext(ctx, `
INSERT INTO customers (id, object, email, name, created) VALUES ($1,$2,$3,$4,$5)
ON CONFLICT (id) DO UPDATE SET object=EXCLUDED.object, email=EXCLUDED.email, name=EXCLUDED.name, created=EXCLUDED.created`,
				c.ID, c.Object, c.Email, c.Name, time.Unix(c.Created, 0).UTC())
			if err != nil {
				return err
			}
		}
		return nil
	})
}

func (s *Store) UpsertSubscriptions(ctx context.Context, ss []Subscription) error {
	return s.inTx(ctx, func(tx *sql.Tx) error {
		for _, x := range ss {
			_, err := tx.ExecContext(ctx, `
INSERT INTO subscriptions (id, object, customer, status, created) VALUES ($1,$2,$3,$4,$5)
ON CONFLICT (id) DO UPDATE SET object=EXCLUDED.object, customer=EXCLUDED.customer, status=EXCLUDED.status, created=EXCLUDED.created`,
				x.ID, x.Object, x.Customer, x.Status, time.Unix(x.Created, 0).UTC())
			if err != nil {
				return err
			}
		}
		return nil
	})
}

func (s *Store) inTx(ctx context.Context, fn func(*sql.Tx) error) error {
	tx, err := s.DB.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	if err := fn(tx); err != nil {
		tx.Rollback()
		return err
	}
	return tx.Commit()
}

// Counts returns row counts per table, in the order of `tables`.
func (s *Store) Counts(ctx context.Context) (map[string]int64, error) {
	out := map[string]int64{}
	for _, t := range tables {
		var n int64
		// table names come from the fixed list above
		if err := s.DB.QueryRowContext(ctx, "SELECT count(*) FROM "+t).Scan(&n); err != nil {
			return nil, fmt.Errorf("counting %s (has `mirror init` been run?): %w", t, err)
		}
		out[t] = n
	}
	return out, nil
}
