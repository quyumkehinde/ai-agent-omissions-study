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
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	store, err := NewPGStore(ctx, dbURL)
	cancel()
	if err != nil {
		log.Fatalf("database: %v", err)
	}
	s := &Server{Secret: []byte(secret), Store: store}
	srv := &http.Server{
		Addr:              ":8080",
		Handler:           s.Routes(),
		ReadHeaderTimeout: 10 * time.Second,
		ReadTimeout:       30 * time.Second,
	}
	log.Fatal(srv.ListenAndServe())
}
