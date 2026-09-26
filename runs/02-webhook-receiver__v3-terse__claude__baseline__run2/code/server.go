package main

import (
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
	// Signed timestamps older (or newer) than this are rejected to stop replays.
	timestampTolerance = 5 * time.Minute
)

type Server struct {
	store  Store
	secret []byte
	now    func() time.Time
}

func NewServer(store Store, secret string) *Server {
	return &Server{store: store, secret: []byte(secret), now: time.Now}
}

func (s *Server) Handler() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /healthz", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	})
	mux.HandleFunc("POST /webhooks", s.handleWebhook)
	return mux
}

var errBadSignature = errors.New("invalid signature")

// verify checks a `t=<unix>,v1=<hex>` header against the raw body.
func (s *Server) verify(header string, body []byte) error {
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
	sec, err := strconv.ParseInt(ts, 10, 64)
	if err != nil || len(sigs) == 0 {
		return errBadSignature
	}

	mac := hmac.New(sha256.New, s.secret)
	mac.Write([]byte(ts + "."))
	mac.Write(body)
	want := mac.Sum(nil)

	matched := false
	for _, sig := range sigs {
		got, err := hex.DecodeString(sig)
		if err == nil && hmac.Equal(got, want) {
			matched = true
		}
	}
	if !matched {
		return errBadSignature
	}

	skew := s.now().Sub(time.Unix(sec, 0))
	if skew > timestampTolerance || skew < -timestampTolerance {
		return errors.New("timestamp outside tolerance")
	}
	return nil
}

func (s *Server) handleWebhook(w http.ResponseWriter, r *http.Request) {
	body, err := io.ReadAll(http.MaxBytesReader(w, r.Body, maxBodyBytes))
	if err != nil {
		http.Error(w, "cannot read body", http.StatusRequestEntityTooLarge)
		return
	}
	if err := s.verify(r.Header.Get("Signature"), body); err != nil {
		http.Error(w, err.Error(), http.StatusUnauthorized)
		return
	}

	var ev struct {
		ID      string `json:"id"`
		Type    string `json:"type"`
		Created *int64 `json:"created"`
	}
	if err := json.Unmarshal(body, &ev); err != nil || ev.ID == "" || ev.Type == "" || ev.Created == nil {
		http.Error(w, "invalid event", http.StatusBadRequest)
		return
	}

	err = s.store.Save(r.Context(), Event{
		ID:      ev.ID,
		Type:    ev.Type,
		Created: time.Unix(*ev.Created, 0).UTC(),
		Payload: body,
	})
	if err != nil {
		log.Printf("store event %s: %v", ev.ID, err)
		http.Error(w, "storage error", http.StatusInternalServerError)
		return
	}
	w.WriteHeader(http.StatusOK)
}
