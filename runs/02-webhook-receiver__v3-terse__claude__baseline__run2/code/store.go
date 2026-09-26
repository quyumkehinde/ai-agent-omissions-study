package main

import (
	"context"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
)

// Event is a webhook event ready to be stored.
type Event struct {
	ID      string
	Type    string
	Created time.Time
	Payload []byte // full raw JSON body
}

// Store persists events. Save must be idempotent per event ID.
type Store interface {
	Save(ctx context.Context, e Event) error
}

const schema = `
CREATE TABLE IF NOT EXISTS events (
	id          TEXT PRIMARY KEY,
	type        TEXT NOT NULL,
	created_at  TIMESTAMPTZ NOT NULL,
	payload     JSONB NOT NULL,
	received_at TIMESTAMPTZ NOT NULL DEFAULT now()
)`

type PGStore struct{ pool *pgxpool.Pool }

func NewPGStore(ctx context.Context, url string) (*PGStore, error) {
	pool, err := pgxpool.New(ctx, url)
	if err != nil {
		return nil, err
	}
	if err := pool.Ping(ctx); err != nil {
		pool.Close()
		return nil, err
	}
	if _, err := pool.Exec(ctx, schema); err != nil {
		pool.Close()
		return nil, err
	}
	return &PGStore{pool: pool}, nil
}

func (s *PGStore) Close() { s.pool.Close() }

// Save inserts the event; a redelivery of an already-stored ID is a no-op.
func (s *PGStore) Save(ctx context.Context, e Event) error {
	_, err := s.pool.Exec(ctx,
		`INSERT INTO events (id, type, created_at, payload) VALUES ($1, $2, $3, $4)
		 ON CONFLICT (id) DO NOTHING`,
		e.ID, e.Type, e.Created, e.Payload)
	return err
}
