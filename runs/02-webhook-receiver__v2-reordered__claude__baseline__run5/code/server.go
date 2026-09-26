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

// DefaultTolerance bounds how old (or future-dated) a signature timestamp may
// be. The signature covers the timestamp, so without this a captured request
// could be replayed forever.
const DefaultTolerance = 5 * time.Minute

const maxBody = 1 << 20

// Event is the parsed subset of a webhook event we index on.
type Event struct {
	ID      string
	Type    string
	Created time.Time
	Payload []byte // full raw JSON body
}

// Store persists events. SaveEvent must be idempotent per event ID and must
// only return nil once the event is durably stored.
type Store interface {
	SaveEvent(ctx context.Context, e Event) error
}

type Server struct {
	secret    []byte
	store     Store
	tolerance time.Duration
	now       func() time.Time
	mux       *http.ServeMux
}

func NewServer(secret string, store Store, tolerance time.Duration) *Server {
	s := &Server{secret: []byte(secret), store: store, tolerance: tolerance, now: time.Now, mux: http.NewServeMux()}
	s.mux.HandleFunc("POST /webhooks", s.handleWebhook)
	s.mux.HandleFunc("GET /healthz", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	})
	return s
}

func (s *Server) ServeHTTP(w http.ResponseWriter, r *http.Request) { s.mux.ServeHTTP(w, r) }

var errBadSignature = errors.New("invalid signature")

// verify checks the Signature header ("t=<unix>,v1=<hex>") against body.
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
	if ts == "" || len(sigs) == 0 {
		return errBadSignature
	}
	t, err := strconv.ParseInt(ts, 10, 64)
	if err != nil {
		return errBadSignature
	}

	mac := hmac.New(sha256.New, s.secret)
	mac.Write([]byte(ts + "."))
	mac.Write(body)
	expected := mac.Sum(nil)

	matched := false
	for _, sig := range sigs {
		got, err := hex.DecodeString(sig)
		if err == nil && hmac.Equal(got, expected) {
			matched = true
		}
	}
	if !matched {
		return errBadSignature
	}

	age := s.now().Sub(time.Unix(t, 0))
	if age > s.tolerance || age < -s.tolerance {
		return errors.New("timestamp outside tolerance")
	}
	return nil
}

func (s *Server) handleWebhook(w http.ResponseWriter, r *http.Request) {
	body, err := io.ReadAll(http.MaxBytesReader(w, r.Body, maxBody))
	if err != nil {
		http.Error(w, "cannot read body", http.StatusBadRequest)
		return
	}
	if err := s.verify(r.Header.Get("Signature"), body); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	var in struct {
		ID      string `json:"id"`
		Type    string `json:"type"`
		Created *int64 `json:"created"`
	}
	if err := json.Unmarshal(body, &in); err != nil || in.ID == "" || in.Type == "" || in.Created == nil {
		http.Error(w, "malformed event", http.StatusBadRequest)
		return
	}

	ev := Event{ID: in.ID, Type: in.Type, Created: time.Unix(*in.Created, 0).UTC(), Payload: body}
	if err := s.store.SaveEvent(r.Context(), ev); err != nil {
		log.Printf("store event %s: %v", ev.ID, err)
		http.Error(w, "storage error", http.StatusInternalServerError)
		return
	}
	w.WriteHeader(http.StatusOK)
}
