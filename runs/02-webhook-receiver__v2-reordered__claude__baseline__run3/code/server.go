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

// DefaultTolerance is how far a signed timestamp may be from now. The
// signature covers the timestamp, so without a bound a captured request
// could be replayed forever.
const DefaultTolerance = 5 * time.Minute

const maxBodyBytes = 1 << 20

// Event is a webhook event as stored.
type Event struct {
	ID      string
	Type    string
	Created time.Time
	Payload []byte // full raw JSON body
}

// Store persists events. Save must be idempotent per event ID.
type Store interface {
	Save(r *http.Request, e Event) error
}

type Server struct {
	store     Store
	secret    []byte
	tolerance time.Duration
	now       func() time.Time
	mux       *http.ServeMux
}

func NewServer(store Store, secret []byte, tolerance time.Duration) *Server {
	s := &Server{store: store, secret: secret, tolerance: tolerance, now: time.Now, mux: http.NewServeMux()}
	s.mux.HandleFunc("GET /healthz", func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
	})
	s.mux.HandleFunc("POST /webhooks", s.handleWebhook)
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
	t, err := strconv.ParseInt(ts, 10, 64)
	if err != nil || len(sigs) == 0 {
		return errBadSignature
	}

	mac := hmac.New(sha256.New, s.secret)
	mac.Write([]byte(ts))
	mac.Write([]byte("."))
	mac.Write(body)
	want := mac.Sum(nil)

	ok := false
	for _, sig := range sigs {
		got, err := hex.DecodeString(sig)
		if err == nil && hmac.Equal(got, want) {
			ok = true
		}
	}
	if !ok {
		return errBadSignature
	}

	age := s.now().Sub(time.Unix(t, 0))
	if age > s.tolerance || age < -s.tolerance {
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

	var ev struct {
		ID      string `json:"id"`
		Type    string `json:"type"`
		Created *int64 `json:"created"`
	}
	if err := json.Unmarshal(body, &ev); err != nil || ev.ID == "" || ev.Type == "" || ev.Created == nil {
		http.Error(w, "invalid event", http.StatusBadRequest)
		return
	}

	err = s.store.Save(r, Event{ID: ev.ID, Type: ev.Type, Created: time.Unix(*ev.Created, 0).UTC(), Payload: body})
	if err != nil {
		// Non-2xx makes the sender retry.
		log.Printf("store event %s: %v", ev.ID, err)
		http.Error(w, "storage failure", http.StatusInternalServerError)
		return
	}
	w.WriteHeader(http.StatusOK)
}
