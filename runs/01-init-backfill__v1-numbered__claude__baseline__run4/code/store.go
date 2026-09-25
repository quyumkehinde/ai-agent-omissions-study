package main

import (
	"context"
	"fmt"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
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
);
CREATE INDEX IF NOT EXISTS subscriptions_customer_idx ON subscriptions (customer);
`

type Store struct{ pool *pgxpool.Pool }

func OpenStore(ctx context.Context, dsn string) (*Store, error) {
	pool, err := pgxpool.New(ctx, dsn)
	if err != nil {
		return nil, fmt.Errorf("connecting to postgres: %w", err)
	}
	if err := pool.Ping(ctx); err != nil {
		pool.Close()
		return nil, fmt.Errorf("connecting to postgres: %w", err)
	}
	return &Store{pool: pool}, nil
}

func (s *Store) Close() { s.pool.Close() }

func (s *Store) CreateSchema(ctx context.Context) error {
	_, err := s.pool.Exec(ctx, schema)
	return err
}

func (s *Store) UpsertCustomers(ctx context.Context, cs []Customer) error {
	b := &pgx.Batch{}
	for _, c := range cs {
		b.Queue(`INSERT INTO customers (id, object, email, name, created) VALUES ($1,$2,$3,$4,$5)
			ON CONFLICT (id) DO UPDATE SET object=EXCLUDED.object, email=EXCLUDED.email,
			name=EXCLUDED.name, created=EXCLUDED.created`,
			c.ID, c.Object, c.Email, c.Name, c.Created)
	}
	return s.sendBatch(ctx, b, len(cs))
}

func (s *Store) UpsertSubscriptions(ctx context.Context, ss []Subscription) error {
	b := &pgx.Batch{}
	for _, x := range ss {
		b.Queue(`INSERT INTO subscriptions (id, object, customer, status, created) VALUES ($1,$2,$3,$4,$5)
			ON CONFLICT (id) DO UPDATE SET object=EXCLUDED.object, customer=EXCLUDED.customer,
			status=EXCLUDED.status, created=EXCLUDED.created`,
			x.ID, x.Object, x.Customer, x.Status, x.Created)
	}
	return s.sendBatch(ctx, b, len(ss))
}

func (s *Store) sendBatch(ctx context.Context, b *pgx.Batch, n int) error {
	br := s.pool.SendBatch(ctx, b)
	defer br.Close()
	for i := 0; i < n; i++ {
		if _, err := br.Exec(); err != nil {
			return err
		}
	}
	return nil
}

// Count returns the number of rows in a known table.
func (s *Store) Count(ctx context.Context, table string) (int64, error) {
	if table != "customers" && table != "subscriptions" {
		return 0, fmt.Errorf("unknown table %q", table)
	}
	var n int64
	err := s.pool.QueryRow(ctx, "SELECT count(*) FROM "+table).Scan(&n)
	return n, err
}
