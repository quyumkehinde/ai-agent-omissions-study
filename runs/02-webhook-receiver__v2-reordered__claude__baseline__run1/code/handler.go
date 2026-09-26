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
	// Signatures older (or newer) than this are rejected to limit replays.
	timestampTolerance = 5 * time.Minute
)

// Event is a webhook event as stored.
type Event struct {
	ID      string
	Type    string
	Created time.Time
	Payload []byte
}

// Store persists events. Save must be idempotent on event ID.
type Store interface {
	Save(ctx context.Context, e Event) error
}

type Server struct {
	Secret string
	Store  Store
	Now    func() time.Time
}

func (s *Server) Routes() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /healthz", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	})
	mux.HandleFunc("POST /webhooks", s.handleWebhook)
	return mux
}

// verifySignature checks a header of the form "t=<unix>,v1=<hex>".
func verifySignature(secret, header string, body []byte, now time.Time) error {
	var ts string
	var sigs []string
	for _, part := range strings.Split(header, ",") {
		k, v, ok := strings.Cut(strings.TrimSpace(part), "=")
		if !ok {
			continue
		}
		switch k {
		case "t":
			ts = v
		case "v1":
			sigs = append(sigs, v)
		}
	}
	if ts == "" || len(sigs) == 0 {
		return errors.New("malformed signature header")
	}
	sec, err := strconv.ParseInt(ts, 10, 64)
	if err != nil {
		return errors.New("bad timestamp")
	}
	if d := now.Sub(time.Unix(sec, 0)); d > timestampTolerance || d < -timestampTolerance {
		return errors.New("timestamp outside tolerance")
	}
	mac := hmac.New(sha256.New, []byte(secret))
	mac.Write([]byte(ts + "."))
	mac.Write(body)
	want := mac.Sum(nil)
	for _, s := range sigs {
		got, err := hex.DecodeString(s)
		if err == nil && hmac.Equal(got, want) {
			return nil
		}
	}
	return errors.New("signature mismatch")
}

func (s *Server) handleWebhook(w http.ResponseWriter, r *http.Request) {
	body, err := io.ReadAll(http.MaxBytesReader(w, r.Body, maxBodyBytes))
	if err != nil {
		http.Error(w, "cannot read body", http.StatusBadRequest)
		return
	}
	now := time.Now
	if s.Now != nil {
		now = s.Now
	}
	if err := verifySignature(s.Secret, r.Header.Get("Signature"), body, now()); err != nil {
		http.Error(w, "invalid signature", http.StatusBadRequest)
		return
	}
	var ev struct {
		ID      string `json:"id"`
		Type    string `json:"type"`
		Created int64  `json:"created"`
	}
	if err := json.Unmarshal(body, &ev); err != nil || ev.ID == "" || ev.Type == "" {
		http.Error(w, "invalid event", http.StatusBadRequest)
		return
	}
	err = s.Store.Save(r.Context(), Event{
		ID: ev.ID, Type: ev.Type, Created: time.Unix(ev.Created, 0).UTC(), Payload: body,
	})
	if err != nil {
		log.Printf("store event %s: %v", ev.ID, err)
		http.Error(w, "storage failure", http.StatusInternalServerError)
		return
	}
	w.WriteHeader(http.StatusOK)
}
