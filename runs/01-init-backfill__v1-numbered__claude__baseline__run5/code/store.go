package main

import (
	"context"
	"database/sql"
	"fmt"

	_ "github.com/jackc/pgx/v5/stdlib"
)

const schemaSQL = `
CREATE TABLE IF NOT EXISTS customers (
	id      text PRIMARY KEY,
	object  text NOT NULL,
	email   text,
	name    text,
	created bigint NOT NULL
);
CREATE TABLE IF NOT EXISTS subscriptions (
	id       text PRIMARY KEY,
	object   text NOT NULL,
	customer text NOT NULL,
	status   text NOT NULL,
	created  bigint NOT NULL
);`

type Store struct{ DB *sql.DB }

func OpenStore(url string) (*Store, error) {
	db, err := sql.Open("pgx", url)
	if err != nil {
		return nil, err
	}
	return &Store{DB: db}, nil
}

func (s *Store) Close() error { return s.DB.Close() }

func (s *Store) CreateTables(ctx context.Context) error {
	_, err := s.DB.ExecContext(ctx, schemaSQL)
	return err
}

func (s *Store) UpsertCustomers(ctx context.Context, cs []Customer) error {
	tx, err := s.DB.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	for _, c := range cs {
		_, err := tx.ExecContext(ctx, `
INSERT INTO customers (id, object, email, name, created) VALUES ($1,$2,$3,$4,$5)
ON CONFLICT (id) DO UPDATE SET object=EXCLUDED.object, email=EXCLUDED.email, name=EXCLUDED.name, created=EXCLUDED.created`,
			c.ID, c.Object, c.Email, c.Name, c.Created)
		if err != nil {
			return fmt.Errorf("upsert customer %s: %w", c.ID, err)
		}
	}
	return tx.Commit()
}

func (s *Store) UpsertSubscriptions(ctx context.Context, ss []Subscription) error {
	tx, err := s.DB.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	for _, x := range ss {
		_, err := tx.ExecContext(ctx, `
INSERT INTO subscriptions (id, object, customer, status, created) VALUES ($1,$2,$3,$4,$5)
ON CONFLICT (id) DO UPDATE SET object=EXCLUDED.object, customer=EXCLUDED.customer, status=EXCLUDED.status, created=EXCLUDED.created`,
			x.ID, x.Object, x.Customer, x.Status, x.Created)
		if err != nil {
			return fmt.Errorf("upsert subscription %s: %w", x.ID, err)
		}
	}
	return tx.Commit()
}

// Count returns the number of rows in a known table.
func (s *Store) Count(ctx context.Context, table string) (int64, error) {
	if table != "customers" && table != "subscriptions" {
		return 0, fmt.Errorf("unknown table %q", table)
	}
	var n int64
	err := s.DB.QueryRowContext(ctx, "SELECT count(*) FROM "+table).Scan(&n)
	return n, err
}
