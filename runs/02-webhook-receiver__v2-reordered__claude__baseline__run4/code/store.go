package main

import (
	"context"
	"net/http"

	"github.com/jackc/pgx/v5/pgxpool"
)

const schema = `
CREATE TABLE IF NOT EXISTS events (
	id         TEXT PRIMARY KEY,
	type       TEXT NOT NULL,
	created_at TIMESTAMPTZ NOT NULL,
	payload    JSONB NOT NULL
)`

type PostgresStore struct {
	pool *pgxpool.Pool
}

func NewPostgresStore(ctx context.Context, url string) (*PostgresStore, error) {
	pool, err := pgxpool.New(ctx, url)
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

// SaveEvent is idempotent: redelivery of a stored event ID is a no-op.
func (p *PostgresStore) SaveEvent(r *http.Request, e Event) error {
	_, err := p.pool.Exec(r.Context(),
		`INSERT INTO events (id, type, created_at, payload) VALUES ($1, $2, $3, $4)
		 ON CONFLICT (id) DO NOTHING`,
		e.ID, e.Type, e.Created, e.Payload)
	return err
}
