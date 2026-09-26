package main

import (
	"encoding/json"
	"io"
	"log"
	"net/http"
	"time"
)

const maxBodyBytes = 1 << 20

type Server struct {
	Secret string
	Store  Store
	Now    func() time.Time
}

func (s *Server) Routes() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("POST /webhooks", s.handleWebhook)
	mux.HandleFunc("GET /healthz", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	})
	return mux
}

func (s *Server) handleWebhook(w http.ResponseWriter, r *http.Request) {
	body, err := io.ReadAll(http.MaxBytesReader(w, r.Body, maxBodyBytes))
	if err != nil {
		http.Error(w, "cannot read body", http.StatusRequestEntityTooLarge)
		return
	}
	if err := verifySignature(s.Secret, r.Header.Get("Signature"), body, s.Now()); err != nil {
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

	err = s.Store.Save(r.Context(), Event{
		ID:      ev.ID,
		Type:    ev.Type,
		Created: time.Unix(*ev.Created, 0).UTC(),
		Payload: body,
	})
	if err != nil {
		log.Printf("store event %s: %v", ev.ID, err)
		http.Error(w, "storage failure", http.StatusInternalServerError) // non-2xx => sender retries
		return
	}
	w.WriteHeader(http.StatusOK)
}
