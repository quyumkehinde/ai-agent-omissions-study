package main

import (
	"context"
	"errors"
	"log"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"
)

func main() {
	secret := os.Getenv("MIRROR_WEBHOOK_SECRET")
	dbURL := os.Getenv("MIRROR_DATABASE_URL")
	if secret == "" || dbURL == "" {
		log.Fatal("MIRROR_WEBHOOK_SECRET and MIRROR_DATABASE_URL are required")
	}

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	store, err := NewPGStore(ctx, dbURL)
	if err != nil {
		log.Fatalf("database: %v", err)
	}
	defer store.Close()

	srv := &http.Server{
		Addr:              ":8080",
		Handler:           NewServer(secret, store).Handler(),
		ReadHeaderTimeout: 5 * time.Second,
		ReadTimeout:       15 * time.Second,
	}
	go func() {
		<-ctx.Done()
		sctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		srv.Shutdown(sctx)
	}()
	log.Println("listening on :8080")
	if err := srv.ListenAndServe(); !errors.Is(err, http.ErrServerClosed) {
		log.Fatal(err)
	}
}
