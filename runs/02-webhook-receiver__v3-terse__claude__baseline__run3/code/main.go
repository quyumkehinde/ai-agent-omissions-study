package main

import (
	"context"
	"log"
	"net/http"
	"os"
	"time"
)

func main() {
	secret := os.Getenv("MIRROR_WEBHOOK_SECRET")
	dbURL := os.Getenv("MIRROR_DATABASE_URL")
	if secret == "" || dbURL == "" {
		log.Fatal("MIRROR_WEBHOOK_SECRET and MIRROR_DATABASE_URL are required")
	}

	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	store, err := NewPGStore(ctx, dbURL)
	cancel()
	if err != nil {
		log.Fatalf("database: %v", err)
	}
	defer store.Close()

	srv := &http.Server{
		Addr:              ":8080",
		Handler:           (&Server{Secret: secret, Store: store, Now: time.Now}).Routes(),
		ReadHeaderTimeout: 10 * time.Second,
		ReadTimeout:       30 * time.Second,
	}
	log.Println("listening on :8080")
	log.Fatal(srv.ListenAndServe())
}
