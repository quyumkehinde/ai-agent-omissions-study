package main

import (
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"io"
	"net/http"
	"strconv"
	"strings"
	"time"
)

const maxBodyBytes = 1 << 20

type Event struct {
	ID        string
	Type      string
	CreatedAt time.Time
	Payload   []byte
}

type EventStore interface {
	Store(context.Context, Event) error
}

type eventPayload struct {
	ID      string `json:"id"`
	Type    string `json:"type"`
	Created *int64 `json:"created"`
}

func NewHandler(secret string, store EventStore) http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /healthz", func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
	})
	mux.HandleFunc("POST /webhooks", webhookHandler(secret, store))
	return mux
}

func webhookHandler(secret string, store EventStore) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		body, err := io.ReadAll(http.MaxBytesReader(w, r.Body, maxBodyBytes))
		if err != nil {
			http.Error(w, "request body too large", http.StatusRequestEntityTooLarge)
			return
		}
		timestamp, suppliedSignature, ok := parseSignature(r.Header.Get("Signature"))
		if !ok || !validSignature(secret, timestamp, body, suppliedSignature) {
			http.Error(w, "invalid signature", http.StatusUnauthorized)
			return
		}

		var payload eventPayload
		if err := json.Unmarshal(body, &payload); err != nil || payload.ID == "" || payload.Type == "" || payload.Created == nil {
			http.Error(w, "invalid event payload", http.StatusBadRequest)
			return
		}
		event := Event{ID: payload.ID, Type: payload.Type, CreatedAt: time.Unix(*payload.Created, 0).UTC(), Payload: body}
		if err := store.Store(r.Context(), event); err != nil {
			http.Error(w, "could not store event", http.StatusInternalServerError)
			return
		}
		w.WriteHeader(http.StatusOK)
	}
}

func parseSignature(header string) (timestamp string, signature []byte, ok bool) {
	parts := strings.Split(header, ",")
	var value string
	for _, part := range parts {
		key, candidate, found := strings.Cut(strings.TrimSpace(part), "=")
		if !found {
			return "", nil, false
		}
		switch key {
		case "t":
			if timestamp != "" || candidate == "" {
				return "", nil, false
			}
			if _, err := strconv.ParseInt(candidate, 10, 64); err != nil {
				return "", nil, false
			}
			timestamp = candidate
		case "v1":
			if value != "" || candidate == "" {
				return "", nil, false
			}
			value = candidate
		}
	}
	if timestamp == "" || value == "" {
		return "", nil, false
	}
	signature, err := hex.DecodeString(value)
	if err != nil || len(signature) != sha256.Size {
		return "", nil, false
	}
	return timestamp, signature, true
}

func validSignature(secret, timestamp string, body, supplied []byte) bool {
	if secret == "" {
		return false
	}
	mac := hmac.New(sha256.New, []byte(secret))
	mac.Write([]byte(timestamp))
	mac.Write([]byte("."))
	mac.Write(body)
	return hmac.Equal(mac.Sum(nil), supplied)
}
