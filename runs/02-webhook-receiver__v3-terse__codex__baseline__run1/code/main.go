package main

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"log"
	"net/http"
	"os"
	"time"

	_ "github.com/jackc/pgx/v5/stdlib"
)

func main() {
	secret := os.Getenv("MIRROR_WEBHOOK_SECRET")
	if secret == "" {
		log.Fatal("MIRROR_WEBHOOK_SECRET must be set")
	}
	databaseURL := os.Getenv("MIRROR_DATABASE_URL")
	if databaseURL == "" {
		log.Fatal("MIRROR_DATABASE_URL must be set")
	}

	db, err := sql.Open("pgx", databaseURL)
	if err != nil {
		log.Fatalf("open database: %v", err)
	}
	defer db.Close()
	if err := db.PingContext(context.Background()); err != nil {
		log.Fatalf("connect database: %v", err)
	}
	store := SQLStore{db: db}
	if err := store.Migrate(context.Background()); err != nil {
		log.Fatalf("migrate database: %v", err)
	}

	server := &http.Server{
		Addr:              ":8080",
		Handler:           NewHandler(secret, store),
		ReadHeaderTimeout: 5 * time.Second,
	}
	log.Printf("listening on %s", server.Addr)
	if err := server.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
		log.Fatal(err)
	}
}

// SQLStore persists events to Postgres.
type SQLStore struct{ db *sql.DB }

func (s SQLStore) Migrate(ctx context.Context) error {
	_, err := s.db.ExecContext(ctx, `
		CREATE TABLE IF NOT EXISTS events (
			id TEXT PRIMARY KEY,
			type TEXT NOT NULL,
			created_at TIMESTAMPTZ NOT NULL,
			payload JSONB NOT NULL
		)`)
	return err
}

func (s SQLStore) Store(ctx context.Context, event Event) error {
	_, err := s.db.ExecContext(ctx, `
		INSERT INTO events (id, type, created_at, payload)
		VALUES ($1, $2, $3, $4::jsonb)
		ON CONFLICT (id) DO NOTHING`, event.ID, event.Type, event.CreatedAt, event.Payload)
	if err != nil {
		return fmt.Errorf("insert event: %w", err)
	}
	return nil
}
