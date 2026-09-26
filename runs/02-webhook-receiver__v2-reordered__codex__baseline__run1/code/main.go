package main

import (
	"context"
	"database/sql"
	"errors"
	"log"
	"net/http"
	"os"

	"github.com/jackc/pgx/v5/stdlib"
)

func main() {
	secret := os.Getenv("MIRROR_WEBHOOK_SECRET")
	if secret == "" {
		log.Fatal("MIRROR_WEBHOOK_SECRET is required")
	}
	databaseURL := os.Getenv("MIRROR_DATABASE_URL")
	if databaseURL == "" {
		log.Fatal("MIRROR_DATABASE_URL is required")
	}

	// Register the pgx database/sql driver without relying on a global init side effect.
	stdlib.GetDefaultDriver()
	db, err := sql.Open("pgx", databaseURL)
	if err != nil {
		log.Fatalf("open database: %v", err)
	}
	defer db.Close()

	ctx := context.Background()
	if err := db.PingContext(ctx); err != nil {
		log.Fatalf("connect to database: %v", err)
	}
	if err := CreateEventsTable(ctx, db); err != nil {
		log.Fatalf("create events table: %v", err)
	}

	h := NewHandler(secret, PostgresStore{DB: db})
	log.Printf("webhook receiver listening on :8080")
	if err := http.ListenAndServe(":8080", h); !errors.Is(err, http.ErrServerClosed) {
		log.Fatalf("serve HTTP: %v", err)
	}
}
