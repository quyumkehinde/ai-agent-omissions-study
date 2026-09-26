package main

import (
	"context"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
)

type Event struct {
	ID      string
	Type    string
	Created time.Time
	Payload []byte
}

// Store persists events. Save must be idempotent per event ID.
type Store interface {
	Save(ctx context.Context, e Event) error
	Ping(ctx context.Context) error
}

const schema = `
CREATE TABLE IF NOT EXISTS events (
	id         TEXT PRIMARY KEY,
	type       TEXT NOT NULL,
	created_at TIMESTAMPTZ NOT NULL,
	payload    JSONB NOT NULL,
	received_at TIMESTAMPTZ NOT NULL DEFAULT now()
)`

type PGStore struct{ pool *pgxpool.Pool }

func NewPGStore(ctx context.Context, url string) (*PGStore, error) {
	pool, err := pgxpool.New(ctx, url)
	if err != nil {
		return nil, err
	}
	if _, err := pool.Exec(ctx, schema); err != nil {
		pool.Close()
		return nil, err
	}
	return &PGStore{pool: pool}, nil
}

func (s *PGStore) Save(ctx context.Context, e Event) error {
	// Redeliveries of the same event ID are a no-op; the first payload wins.
	_, err := s.pool.Exec(ctx,
		`INSERT INTO events (id, type, created_at, payload) VALUES ($1, $2, $3, $4)
		 ON CONFLICT (id) DO NOTHING`,
		e.ID, e.Type, e.Created, e.Payload)
	return err
}

func (s *PGStore) Ping(ctx context.Context) error { return s.pool.Ping(ctx) }

func (s *PGStore) Close() { s.pool.Close() }
