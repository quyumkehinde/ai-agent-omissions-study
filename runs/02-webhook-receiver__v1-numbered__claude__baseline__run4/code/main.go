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
		log.Fatal("MIRROR_WEBHOOK_SECRET and MIRROR_DATABASE_URL must be set")
	}
	store, err := NewPGStore(context.Background(), dbURL)
	if err != nil {
		log.Fatalf("database: %v", err)
	}
	defer store.Close()

	srv := &http.Server{
		Addr:              ":8080",
		Handler:           NewServer(secret, store).Handler(),
		ReadHeaderTimeout: 10 * time.Second,
		ReadTimeout:       30 * time.Second,
	}
	log.Fatal(srv.ListenAndServe())
}
