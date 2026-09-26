package main

import (
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
)

const maxBodyBytes = 1 << 20 // webhook payloads larger than 1 MiB are rejected

type event struct {
	ID      string          `json:"id"`
	Type    string          `json:"type"`
	Created int64           `json:"created"`
	Data    json.RawMessage `json:"data"`
}

// EventStore makes the HTTP handling independent of Postgres and easy to test.
type EventStore interface {
	Store(ctx context.Context, e event, payload []byte) error
}

type postgresStore struct {
	pool *pgxpool.Pool
}

func newPostgresStore(ctx context.Context, databaseURL string) (*postgresStore, error) {
	pool, err := pgxpool.New(ctx, databaseURL)
	if err != nil {
		return nil, fmt.Errorf("connect to postgres: %w", err)
	}

	store := &postgresStore{pool: pool}
	if err := store.createTable(ctx); err != nil {
		pool.Close()
		return nil, err
	}
	return store, nil
}

func (s *postgresStore) createTable(ctx context.Context) error {
	_, err := s.pool.Exec(ctx, `
		CREATE TABLE IF NOT EXISTS events (
			id TEXT PRIMARY KEY,
			type TEXT NOT NULL,
			created_at TIMESTAMPTZ NOT NULL,
			payload JSONB NOT NULL
		)`)
	if err != nil {
		return fmt.Errorf("create events table: %w", err)
	}
	return nil
}

func (s *postgresStore) Store(ctx context.Context, e event, payload []byte) error {
	_, err := s.pool.Exec(ctx, `
		INSERT INTO events (id, type, created_at, payload)
		VALUES ($1, $2, $3, $4::jsonb)
		ON CONFLICT (id) DO NOTHING`,
		e.ID, e.Type, time.Unix(e.Created, 0).UTC(), string(payload))
	if err != nil {
		return fmt.Errorf("insert event: %w", err)
	}
	return nil
}

func (s *postgresStore) Close() { s.pool.Close() }

type server struct {
	secret []byte
	store  EventStore
}

func (s *server) routes() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("/healthz", s.healthz)
	mux.HandleFunc("/webhooks", s.webhooks)
	return mux
}

func (s *server) healthz(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		w.WriteHeader(http.StatusMethodNotAllowed)
		return
	}
	w.WriteHeader(http.StatusOK)
}

func (s *server) webhooks(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		w.WriteHeader(http.StatusMethodNotAllowed)
		return
	}
	r.Body = http.MaxBytesReader(w, r.Body, maxBodyBytes)
	payload, err := io.ReadAll(r.Body)
	if err != nil {
		http.Error(w, "invalid request body", http.StatusBadRequest)
		return
	}

	if !verifySignature(r.Header.Get("Signature"), payload, s.secret) {
		http.Error(w, "invalid signature", http.StatusBadRequest)
		return
	}

	var e event
	if err := json.Unmarshal(payload, &e); err != nil || e.ID == "" || e.Type == "" {
		http.Error(w, "invalid event payload", http.StatusBadRequest)
		return
	}

	if err := s.store.Store(r.Context(), e, payload); err != nil {
		log.Printf("store webhook event %q: %v", e.ID, err)
		http.Error(w, "could not store event", http.StatusInternalServerError)
		return
	}
	w.WriteHeader(http.StatusOK)
}

// verifySignature validates t=<unix seconds>,v1=<hex HMAC-SHA256>. Timestamp
// freshness is intentionally not checked: the API permits delayed redelivery.
func verifySignature(header string, body, secret []byte) bool {
	var timestamp, signature string
	for _, part := range strings.Split(header, ",") {
		key, value, ok := strings.Cut(strings.TrimSpace(part), "=")
		if !ok {
			return false
		}
		switch key {
		case "t":
			if timestamp != "" {
				return false
			}
			timestamp = value
		case "v1":
			if signature != "" {
				return false
			}
			signature = value
		default:
			return false
		}
	}
	if timestamp == "" || signature == "" {
		return false
	}
	if _, err := strconv.ParseInt(timestamp, 10, 64); err != nil {
		return false
	}
	received, err := hex.DecodeString(signature)
	if err != nil || len(received) != sha256.Size {
		return false
	}

	mac := hmac.New(sha256.New, secret)
	_, _ = mac.Write([]byte(timestamp))
	_, _ = mac.Write([]byte("."))
	_, _ = mac.Write(body)
	return hmac.Equal(mac.Sum(nil), received)
}

func main() {
	secret := os.Getenv("MIRROR_WEBHOOK_SECRET")
	databaseURL := os.Getenv("MIRROR_DATABASE_URL")
	if secret == "" || databaseURL == "" {
		log.Fatal("MIRROR_WEBHOOK_SECRET and MIRROR_DATABASE_URL must be set")
	}

	store, err := newPostgresStore(context.Background(), databaseURL)
	if err != nil {
		log.Fatal(err)
	}
	defer store.Close()

	log.Printf("listening on :8080")
	if err := http.ListenAndServe(":8080", (&server{secret: []byte(secret), store: store}).routes()); !errors.Is(err, http.ErrServerClosed) {
		log.Fatal(err)
	}
}
