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

// SignatureTolerance bounds how old (or future-dated) a signed timestamp may
// be. The signature covers the timestamp, so without this a captured request
// could be replayed indefinitely.
const SignatureTolerance = 5 * time.Minute

const maxBodyBytes = 1 << 20

// Event is the subset of the webhook body that we index; the full raw
// payload is stored alongside.
type Event struct {
	ID      string
	Type    string
	Created time.Time
	Payload []byte
}

// Store persists events. Save must be idempotent on event ID, since the same
// event can be delivered more than once.
type Store interface {
	Save(ctx context.Context, e Event) error
}

type Server struct {
	store  Store
	secret []byte
	now    func() time.Time
	mux    *http.ServeMux
}

func NewServer(store Store, secret []byte) *Server {
	s := &Server{store: store, secret: secret, now: time.Now, mux: http.NewServeMux()}
	s.mux.HandleFunc("POST /webhooks", s.handleWebhook)
	s.mux.HandleFunc("GET /healthz", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	})
	return s
}

func (s *Server) ServeHTTP(w http.ResponseWriter, r *http.Request) { s.mux.ServeHTTP(w, r) }

var errBadSignature = errors.New("invalid signature")

// verifySignature checks a header of the form "t=<unix>,v1=<hex>".
func verifySignature(header string, body, secret []byte, now time.Time) error {
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
	secs, err := strconv.ParseInt(ts, 10, 64)
	if err != nil || len(sigs) == 0 {
		return errBadSignature
	}

	mac := hmac.New(sha256.New, secret)
	mac.Write([]byte(ts))
	mac.Write([]byte("."))
	mac.Write(body)
	expected := mac.Sum(nil)

	valid := false
	for _, sig := range sigs {
		got, err := hex.DecodeString(sig)
		if err == nil && hmac.Equal(got, expected) {
			valid = true
		}
	}
	if !valid {
		return errBadSignature
	}

	age := now.Sub(time.Unix(secs, 0))
	if age > SignatureTolerance || age < -SignatureTolerance {
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
	if err := verifySignature(r.Header.Get("Signature"), body, s.secret, s.now()); err != nil {
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
	if err := s.store.Save(r.Context(), ev); err != nil {
		log.Printf("save event %s: %v", ev.ID, err)
		http.Error(w, "storage error", http.StatusInternalServerError)
		return
	}
	w.WriteHeader(http.StatusOK)
}
