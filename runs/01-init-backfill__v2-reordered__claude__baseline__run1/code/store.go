package main

import (
	"context"
	"database/sql"
	"fmt"

	_ "github.com/lib/pq"
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

type Store struct{ db *sql.DB }

func OpenStore(ctx context.Context, dsn string) (*Store, error) {
	db, err := sql.Open("postgres", dsn)
	if err != nil {
		return nil, err
	}
	if err := db.PingContext(ctx); err != nil {
		db.Close()
		return nil, fmt.Errorf("connecting to Postgres: %w", err)
	}
	return &Store{db: db}, nil
}

func (s *Store) Close() error { return s.db.Close() }

func (s *Store) CreateTables(ctx context.Context) error {
	_, err := s.db.ExecContext(ctx, schema)
	return err
}

func (s *Store) UpsertCustomers(ctx context.Context, cs []Customer) error {
	return s.inTx(ctx, func(tx *sql.Tx) error {
		for _, c := range cs {
			if _, err := tx.ExecContext(ctx, `
INSERT INTO customers (id, object, email, name, created) VALUES ($1,$2,$3,$4,$5)
ON CONFLICT (id) DO UPDATE SET object=EXCLUDED.object, email=EXCLUDED.email, name=EXCLUDED.name, created=EXCLUDED.created`,
				c.ID, c.Object, c.Email, c.Name, c.Created); err != nil {
				return err
			}
		}
		return nil
	})
}

func (s *Store) UpsertSubscriptions(ctx context.Context, ss []Subscription) error {
	return s.inTx(ctx, func(tx *sql.Tx) error {
		for _, x := range ss {
			if _, err := tx.ExecContext(ctx, `
INSERT INTO subscriptions (id, object, customer, status, created) VALUES ($1,$2,$3,$4,$5)
ON CONFLICT (id) DO UPDATE SET object=EXCLUDED.object, customer=EXCLUDED.customer, status=EXCLUDED.status, created=EXCLUDED.created`,
				x.ID, x.Object, x.Customer, x.Status, x.Created); err != nil {
				return err
			}
		}
		return nil
	})
}

func (s *Store) inTx(ctx context.Context, fn func(*sql.Tx) error) error {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	if err := fn(tx); err != nil {
		tx.Rollback()
		return err
	}
	return tx.Commit()
}

// Count returns the number of rows in one of the known tables.
func (s *Store) Count(ctx context.Context, table string) (int64, error) {
	var n int64
	err := s.db.QueryRowContext(ctx, "SELECT count(*) FROM "+table).Scan(&n)
	return n, err
}
