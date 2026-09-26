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
	// Signatures cover the timestamp, so a captured request stays valid
	// forever unless we bound its age.
	timestampTolerance = 5 * time.Minute
)

// Event is the webhook envelope. Payload holds the raw request body.
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
	mux.HandleFunc("POST /webhooks", s.handleWebhook)
	mux.HandleFunc("GET /healthz", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	})
	return mux
}

func parseSignature(h string) (t int64, sig []byte, err error) {
	var haveT, haveV bool
	for _, part := range strings.Split(h, ",") {
		k, v, ok := strings.Cut(strings.TrimSpace(part), "=")
		if !ok {
			continue
		}
		switch k {
		case "t":
			t, err = strconv.ParseInt(v, 10, 64)
			if err != nil {
				return 0, nil, err
			}
			haveT = true
		case "v1":
			if !haveV {
				sig, err = hex.DecodeString(v)
				if err != nil {
					return 0, nil, err
				}
				haveV = true
			}
		}
	}
	if !haveT || !haveV {
		return 0, nil, errors.New("malformed signature header")
	}
	return t, sig, nil
}

func (s *Server) verify(header string, body []byte) bool {
	t, sig, err := parseSignature(header)
	if err != nil {
		return false
	}
	mac := hmac.New(sha256.New, []byte(s.Secret))
	mac.Write([]byte(strconv.FormatInt(t, 10) + "."))
	mac.Write(body)
	if !hmac.Equal(mac.Sum(nil), sig) {
		return false
	}
	age := s.Now().Sub(time.Unix(t, 0))
	return age <= timestampTolerance && age >= -timestampTolerance
}

func (s *Server) handleWebhook(w http.ResponseWriter, r *http.Request) {
	body, err := io.ReadAll(http.MaxBytesReader(w, r.Body, maxBodyBytes))
	if err != nil {
		http.Error(w, "cannot read body", http.StatusBadRequest)
		return
	}
	if !s.verify(r.Header.Get("Signature"), body) {
		http.Error(w, "invalid signature", http.StatusBadRequest)
		return
	}
	var env struct {
		ID      string `json:"id"`
		Type    string `json:"type"`
		Created int64  `json:"created"`
	}
	if err := json.Unmarshal(body, &env); err != nil || env.ID == "" || env.Type == "" {
		http.Error(w, "invalid event", http.StatusBadRequest)
		return
	}
	ev := Event{ID: env.ID, Type: env.Type, Created: time.Unix(env.Created, 0).UTC(), Payload: body}
	if err := s.Store.Save(r.Context(), ev); err != nil {
		log.Printf("store event %s: %v", ev.ID, err)
		http.Error(w, "storage error", http.StatusInternalServerError) // non-2xx => API retries
		return
	}
	w.WriteHeader(http.StatusOK)
}
