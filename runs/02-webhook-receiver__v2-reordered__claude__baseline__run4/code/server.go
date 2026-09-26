package main

import (
	"crypto/hmac"
	"crypto/sha256"
	"crypto/subtle"
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
	// A signature covers its timestamp, so a captured request stays valid
	// forever unless we bound its age.
	signatureTolerance = 5 * time.Minute
)

// Event is a webhook event. Payload holds the raw request body.
type Event struct {
	ID      string
	Type    string
	Created time.Time
	Payload []byte
}

// Store persists events. Saving an already-stored ID must succeed without
// changing the stored row (deliveries can repeat).
type Store interface {
	SaveEvent(r *http.Request, e Event) error
}

type Server struct {
	Secret string
	Store  Store
	Now    func() time.Time
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
	sec, err := strconv.ParseInt(ts, 10, 64)
	if err != nil || len(sigs) == 0 {
		return errBadSignature
	}
	mac := hmac.New(sha256.New, []byte(secret))
	mac.Write([]byte(ts + "."))
	mac.Write(body)
	want := []byte(hex.EncodeToString(mac.Sum(nil)))
	valid := 0
	for _, got := range sigs {
		valid |= subtle.ConstantTimeCompare(want, []byte(got))
	}
	if valid != 1 {
		return errBadSignature
	}
	age := now.Sub(time.Unix(sec, 0))
	if age > signatureTolerance || age < -signatureTolerance {
		return errBadSignature
	}
	return nil
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
	var in struct {
		ID      string `json:"id"`
		Type    string `json:"type"`
		Created *int64 `json:"created"`
	}
	if err := json.Unmarshal(body, &in); err != nil || in.ID == "" || in.Type == "" || in.Created == nil {
		http.Error(w, "invalid event", http.StatusBadRequest)
		return
	}
	ev := Event{ID: in.ID, Type: in.Type, Created: time.Unix(*in.Created, 0).UTC(), Payload: body}
	if err := s.Store.SaveEvent(r, ev); err != nil {
		log.Printf("store event %s: %v", ev.ID, err)
		http.Error(w, "storage failure", http.StatusInternalServerError)
		return
	}
	w.WriteHeader(http.StatusOK)
}
