package main

import (
	"encoding/json"
	"io"
	"log"
	"net/http"
	"time"
)

const (
	maxBody       = 1 << 20
	signTolerance = 5 * time.Minute
)

type Server struct {
	secret string
	store  Store
	now    func() time.Time
}

func NewServer(secret string, store Store) *Server {
	return &Server{secret: secret, store: store, now: time.Now}
}

func (s *Server) Handler() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("POST /webhooks", s.handleWebhook)
	mux.HandleFunc("GET /healthz", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	})
	return mux
}

func (s *Server) handleWebhook(w http.ResponseWriter, r *http.Request) {
	body, err := io.ReadAll(http.MaxBytesReader(w, r.Body, maxBody))
	if err != nil {
		http.Error(w, "cannot read body", http.StatusRequestEntityTooLarge)
		return
	}
	if err := verifySignature(s.secret, r.Header.Get("Signature"), body, s.now(), signTolerance); err != nil {
		http.Error(w, err.Error(), http.StatusUnauthorized)
		return
	}

	var p struct {
		ID      string `json:"id"`
		Type    string `json:"type"`
		Created *int64 `json:"created"`
	}
	if err := json.Unmarshal(body, &p); err != nil || p.ID == "" || p.Type == "" || p.Created == nil {
		http.Error(w, "invalid event", http.StatusBadRequest)
		return
	}

	err = s.store.Save(r.Context(), Event{
		ID:      p.ID,
		Type:    p.Type,
		Created: time.Unix(*p.Created, 0).UTC(),
		Payload: body,
	})
	if err != nil {
		log.Printf("store event %s: %v", p.ID, err)
		http.Error(w, "storage failure", http.StatusInternalServerError)
		return
	}
	w.WriteHeader(http.StatusOK)
}
