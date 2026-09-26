package main

import (
	"context"

	"github.com/jackc/pgx/v5/pgxpool"
)

const schema = `
CREATE TABLE IF NOT EXISTS events (
	id         TEXT PRIMARY KEY,
	type       TEXT NOT NULL,
	created_at TIMESTAMPTZ NOT NULL,
	payload    JSONB NOT NULL
)`

type PostgresStore struct{ pool *pgxpool.Pool }

func NewPostgresStore(ctx context.Context, dsn string) (*PostgresStore, error) {
	pool, err := pgxpool.New(ctx, dsn)
	if err != nil {
		return nil, err
	}
	if _, err := pool.Exec(ctx, schema); err != nil {
		pool.Close()
		return nil, err
	}
	return &PostgresStore{pool: pool}, nil
}

func (p *PostgresStore) Close() { p.pool.Close() }

// SaveEvent is idempotent: redelivered event IDs are ignored, so duplicates
// are acknowledged without creating a second row.
func (p *PostgresStore) SaveEvent(ctx context.Context, e Event) error {
	_, err := p.pool.Exec(ctx,
		`INSERT INTO events (id, type, created_at, payload) VALUES ($1, $2, $3, $4)
		 ON CONFLICT (id) DO NOTHING`,
		e.ID, e.Type, e.Created, e.Payload)
	return err
}
