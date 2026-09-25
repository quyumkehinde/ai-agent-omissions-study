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
	created BIGINT NOT NULL
);
CREATE TABLE IF NOT EXISTS subscriptions (
	id       TEXT PRIMARY KEY,
	object   TEXT NOT NULL,
	customer TEXT NOT NULL,
	status   TEXT NOT NULL,
	created  BIGINT NOT NULL
);
`

const upsertCustomer = `
INSERT INTO customers (id, object, email, name, created) VALUES ($1,$2,$3,$4,$5)
ON CONFLICT (id) DO UPDATE SET object=EXCLUDED.object, email=EXCLUDED.email, name=EXCLUDED.name, created=EXCLUDED.created`

const upsertSubscription = `
INSERT INTO subscriptions (id, object, customer, status, created) VALUES ($1,$2,$3,$4,$5)
ON CONFLICT (id) DO UPDATE SET object=EXCLUDED.object, customer=EXCLUDED.customer, status=EXCLUDED.status, created=EXCLUDED.created`

type Store struct{ conn *pgx.Conn }

func OpenStore(ctx context.Context, dsn string) (*Store, error) {
	conn, err := pgx.Connect(ctx, dsn)
	if err != nil {
		return nil, fmt.Errorf("connecting to Postgres: %w", err)
	}
	return &Store{conn: conn}, nil
}

func (s *Store) Close(ctx context.Context) { s.conn.Close(ctx) }

func (s *Store) CreateSchema(ctx context.Context) error {
	_, err := s.conn.Exec(ctx, schema)
	return err
}

func (s *Store) UpsertCustomers(ctx context.Context, cs []Customer) error {
	b := &pgx.Batch{}
	for _, c := range cs {
		b.Queue(upsertCustomer, c.ID, c.Object, c.Email, c.Name, c.Created)
	}
	return s.sendBatch(ctx, b)
}

func (s *Store) UpsertSubscriptions(ctx context.Context, ss []Subscription) error {
	b := &pgx.Batch{}
	for _, x := range ss {
		b.Queue(upsertSubscription, x.ID, x.Object, x.Customer, x.Status, x.Created)
	}
	return s.sendBatch(ctx, b)
}

func (s *Store) sendBatch(ctx context.Context, b *pgx.Batch) error {
	return pgx.BeginFunc(ctx, s.conn, func(tx pgx.Tx) error {
		return tx.SendBatch(ctx, b).Close()
	})
}

func (s *Store) Count(ctx context.Context, table string) (int64, error) {
	if table != "customers" && table != "subscriptions" {
		return 0, fmt.Errorf("unknown table %q", table)
	}
	var n int64
	err := s.conn.QueryRow(ctx, "SELECT count(*) FROM "+table).Scan(&n)
	return n, err
}
