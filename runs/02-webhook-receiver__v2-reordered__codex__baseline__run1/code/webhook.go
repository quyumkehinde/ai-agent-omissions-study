package main

import (
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"io"
	"net/http"
	"strconv"
	"strings"
	"time"
)

const maxBodyBytes = 1 << 20 // webhook events are expected to be small; avoid unbounded reads.

type Event struct {
	ID      string          `json:"id"`
	Type    string          `json:"type"`
	Created int64           `json:"created"`
	Payload json.RawMessage `json:"-"`
}

type EventStore interface {
	StoreEvent(context.Context, Event) error
}

type PostgresStore struct {
	DB *sql.DB
}

const createEventsTableSQL = `
CREATE TABLE IF NOT EXISTS events (
    id TEXT PRIMARY KEY,
    type TEXT NOT NULL,
    created_at TIMESTAMPTZ NOT NULL,
    payload JSONB NOT NULL
)`

func CreateEventsTable(ctx context.Context, db *sql.DB) error {
	_, err := db.ExecContext(ctx, createEventsTableSQL)
	return err
}

func (s PostgresStore) StoreEvent(ctx context.Context, event Event) error {
	_, err := s.DB.ExecContext(ctx, `
INSERT INTO events (id, type, created_at, payload)
VALUES ($1, $2, $3, $4::jsonb)
ON CONFLICT (id) DO UPDATE
SET type = EXCLUDED.type, created_at = EXCLUDED.created_at, payload = EXCLUDED.payload`,
		event.ID, event.Type, time.Unix(event.Created, 0).UTC(), string(event.Payload))
	return err
}

type webhookHandler struct {
	secret []byte
	store  EventStore
}

func NewHandler(secret string, store EventStore) http.Handler {
	return &webhookHandler{secret: []byte(secret), store: store}
}

func (h *webhookHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	switch {
	case r.Method == http.MethodGet && r.URL.Path == "/healthz":
		w.WriteHeader(http.StatusOK)
	case r.Method == http.MethodPost && r.URL.Path == "/webhooks":
		h.handleWebhook(w, r)
	default:
		http.NotFound(w, r)
	}
}

func (h *webhookHandler) handleWebhook(w http.ResponseWriter, r *http.Request) {
	body, err := io.ReadAll(http.MaxBytesReader(w, r.Body, maxBodyBytes))
	if err != nil {
		http.Error(w, "invalid request body", http.StatusBadRequest)
		return
	}

	if !verifySignature(r.Header.Get("Signature"), body, h.secret) {
		http.Error(w, "invalid signature", http.StatusBadRequest)
		return
	}

	var event Event
	if err := json.Unmarshal(body, &event); err != nil || event.ID == "" || event.Type == "" {
		http.Error(w, "invalid event payload", http.StatusBadRequest)
		return
	}
	event.Payload = append(event.Payload[:0], body...)

	if err := h.store.StoreEvent(r.Context(), event); err != nil {
		http.Error(w, "could not store event", http.StatusInternalServerError)
		return
	}
	w.WriteHeader(http.StatusOK)
}

// verifySignature accepts the comma-separated Signature header documented by the API.
func verifySignature(header string, body, secret []byte) bool {
	var timestamp, signature string
	for _, part := range strings.Split(header, ",") {
		key, value, ok := strings.Cut(strings.TrimSpace(part), "=")
		if !ok {
			continue
		}
		switch key {
		case "t":
			timestamp = value
		case "v1":
			signature = value
		}
	}
	if timestamp == "" || signature == "" {
		return false
	}
	if _, err := strconv.ParseInt(timestamp, 10, 64); err != nil {
		return false
	}
	provided, err := hex.DecodeString(signature)
	if err != nil {
		return false
	}

	mac := hmac.New(sha256.New, secret)
	_, _ = mac.Write([]byte(timestamp + "."))
	_, _ = mac.Write(body)
	return hmac.Equal(mac.Sum(nil), provided)
}
