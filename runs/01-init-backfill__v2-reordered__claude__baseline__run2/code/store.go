package main

import (
	"context"
	"database/sql"
	"fmt"
)

// Tables lists the mirrored tables in display order.
var Tables = []string{"customers", "subscriptions"}

const schema = `
CREATE TABLE IF NOT EXISTS customers (
	id      TEXT PRIMARY KEY,
	object  TEXT,
	email   TEXT,
	name    TEXT,
	created BIGINT
);
CREATE TABLE IF NOT EXISTS subscriptions (
	id       TEXT PRIMARY KEY,
	object   TEXT,
	customer TEXT,
	status   TEXT,
	created  BIGINT
);`

type Store struct{ DB *sql.DB }

func (s *Store) CreateSchema(ctx context.Context) error {
	_, err := s.DB.ExecContext(ctx, schema)
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
			return err
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
			return err
		}
	}
	return tx.Commit()
}

func (s *Store) Count(ctx context.Context, table string) (int, error) {
	var n int
	// table comes from the fixed Tables list, never user input.
	err := s.DB.QueryRowContext(ctx, "SELECT count(*) FROM "+table).Scan(&n)
	if err != nil {
		return 0, fmt.Errorf("counting %s: %w (has `mirror init` been run?)", table, err)
	}
	return n, nil
}
