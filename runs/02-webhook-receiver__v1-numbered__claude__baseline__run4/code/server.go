package main

import (
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"io"
	"log"
	"net/http"
	"strconv"
	"strings"
	"time"
)

const (
	maxBodyBytes = 1 << 20
	// Signed timestamps older (or newer) than this are rejected, so a
	// captured request cannot be replayed later.
	timestampTolerance = 5 * time.Minute
)

// Event is a webhook event. Payload holds the full raw body.
type Event struct {
	ID      string
	Type    string
	Created time.Time
	Payload []byte
}

// Store persists events. Storing an already-stored ID must succeed (idempotent).
type Store interface {
	SaveEvent(ctx context.Context, e Event) error
}

type Server struct {
	secret []byte
	store  Store
	now    func() time.Time
}

func NewServer(secret string, store Store) *Server {
	return &Server{secret: []byte(secret), store: store, now: time.Now}
}

func (s *Server) Handler() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("POST /webhooks", s.handleWebhook)
	mux.HandleFunc("GET /healthz", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	})
	return mux
}

var errBadSignature = errors.New("invalid signature")

func sign(secret []byte, t string, body []byte) string {
	m := hmac.New(sha256.New, secret)
	m.Write([]byte(t))
	m.Write([]byte("."))
	m.Write(body)
	return hex.EncodeToString(m.Sum(nil))
}

func (s *Server) verify(header string, body []byte) error {
	var t string
	var sigs []string
	for _, part := range strings.Split(header, ",") {
		k, v, ok := strings.Cut(strings.TrimSpace(part), "=")
		if !ok {
			continue
		}
		switch k {
		case "t":
			t = v
		case "v1":
			sigs = append(sigs, v)
		}
	}
	if t == "" || len(sigs) == 0 {
		return errBadSignature
	}
	ts, err := strconv.ParseInt(t, 10, 64)
	if err != nil {
		return errBadSignature
	}
	expected := []byte(sign(s.secret, t, body))
	ok := false
	for _, sig := range sigs {
		if hmac.Equal(expected, []byte(sig)) {
			ok = true
		}
	}
	if !ok {
		return errBadSignature
	}
	age := s.now().Sub(time.Unix(ts, 0))
	if age > timestampTolerance || age < -timestampTolerance {
		return errors.New("timestamp outside tolerance")
	}
	return nil
}

func (s *Server) handleWebhook(w http.ResponseWriter, r *http.Request) {
	body, err := io.ReadAll(http.MaxBytesReader(w, r.Body, maxBodyBytes))
	if err != nil {
		http.Error(w, "cannot read body", http.StatusBadRequest)
		return
	}
	if err := s.verify(r.Header.Get("Signature"), body); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	var raw struct {
		ID      string `json:"id"`
		Type    string `json:"type"`
		Created *int64 `json:"created"`
	}
	if err := json.Unmarshal(body, &raw); err != nil || raw.ID == "" || raw.Type == "" || raw.Created == nil {
		http.Error(w, "invalid event", http.StatusBadRequest)
		return
	}
	ev := Event{ID: raw.ID, Type: raw.Type, Created: time.Unix(*raw.Created, 0).UTC(), Payload: body}
	if err := s.store.SaveEvent(r.Context(), ev); err != nil {
		log.Printf("store event %s: %v", ev.ID, err)
		http.Error(w, "storage failure", http.StatusInternalServerError)
		return
	}
	w.WriteHeader(http.StatusOK)
}
