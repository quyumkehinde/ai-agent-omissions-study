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
	// Tolerance bounds how old (or future-dated) a signed timestamp may be,
	// limiting replay of captured requests.
	defaultTolerance = 5 * time.Minute
)

// Event is the webhook envelope. Raw holds the full original payload.
type Event struct {
	ID      string    `json:"id"`
	Type    string    `json:"type"`
	Created time.Time `json:"-"`
	Raw     []byte    `json:"-"`
}

// Store persists events. Save must be idempotent on event ID.
type Store interface {
	Save(r *http.Request, e Event) error
}

type Server struct {
	Secret    []byte
	Store     Store
	Tolerance time.Duration
	Now       func() time.Time
}

func (s *Server) Routes() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /healthz", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	})
	mux.HandleFunc("POST /webhooks", s.handleWebhook)
	return mux
}

var errBadSignature = errors.New("invalid signature")

// verify checks a `t=<unix>,v1=<hex>` header against the raw body.
func verify(secret []byte, header string, body []byte, now time.Time, tolerance time.Duration) error {
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
	mac := hmac.New(sha256.New, secret)
	mac.Write([]byte(ts))
	mac.Write([]byte("."))
	mac.Write(body)
	want := mac.Sum(nil)
	matched := false
	for _, s := range sigs {
		got, err := hex.DecodeString(s)
		if err == nil && hmac.Equal(got, want) {
			matched = true
		}
	}
	if !matched {
		return errBadSignature
	}
	d := now.Sub(time.Unix(t, 0))
	if d < 0 {
		d = -d
	}
	if d > tolerance {
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
	tol := s.Tolerance
	if tol == 0 {
		tol = defaultTolerance
	}
	now := time.Now
	if s.Now != nil {
		now = s.Now
	}
	if err := verify(s.Secret, r.Header.Get("Signature"), body, now(), tol); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	var env struct {
		ID      string `json:"id"`
		Type    string `json:"type"`
		Created *int64 `json:"created"`
	}
	if err := json.Unmarshal(body, &env); err != nil || env.ID == "" || env.Type == "" || env.Created == nil {
		http.Error(w, "malformed event", http.StatusBadRequest)
		return
	}
	ev := Event{ID: env.ID, Type: env.Type, Created: time.Unix(*env.Created, 0).UTC(), Raw: body}

	// Respond 2xx only after the event is durably stored; otherwise 500 so
	// the sender retries.
	if err := s.Store.Save(r, ev); err != nil {
		log.Printf("store event %s: %v", ev.ID, err)
		http.Error(w, "storage error", http.StatusInternalServerError)
		return
	}
	w.WriteHeader(http.StatusOK)
}
