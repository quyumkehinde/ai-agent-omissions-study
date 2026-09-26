package main

import (
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

var testSecret = []byte("whsec_test")

type memStore struct {
	events map[string]Event
	err    error
}

func (m *memStore) Save(_ context.Context, e Event) error {
	if m.err != nil {
		return m.err
	}
	if m.events == nil {
		m.events = map[string]Event{}
	}
	if _, ok := m.events[e.ID]; !ok {
		m.events[e.ID] = e
	}
	return nil
}

func sign(secret []byte, t time.Time, body string) string {
	ts := fmt.Sprint(t.Unix())
	mac := hmac.New(sha256.New, secret)
	mac.Write([]byte(ts + "." + body))
	return "t=" + ts + ",v1=" + hex.EncodeToString(mac.Sum(nil))
}

const goodBody = `{"id":"evt_0001","object":"event","type":"customer.created","created":1735689600,"data":{"object":{"id":"cus_1"}}}`

func post(s *Server, body, sig string) *httptest.ResponseRecorder {
	req := httptest.NewRequest("POST", "/webhooks", strings.NewReader(body))
	if sig != "" {
		req.Header.Set("Signature", sig)
	}
	rec := httptest.NewRecorder()
	s.ServeHTTP(rec, req)
	return rec
}

func TestValidEventStored(t *testing.T) {
	st := &memStore{}
	s := NewServer(st, testSecret)
	rec := post(s, goodBody, sign(testSecret, time.Now(), goodBody))
	if rec.Code != 200 {
		t.Fatalf("code %d: %s", rec.Code, rec.Body)
	}
	e, ok := st.events["evt_0001"]
	if !ok || e.Type != "customer.created" || e.Created.Unix() != 1735689600 || string(e.Payload) != goodBody {
		t.Fatalf("bad stored event: %+v", e)
	}
}

func TestRejections(t *testing.T) {
	now := time.Now()
	cases := map[string]string{
		"missing header":   "",
		"garbage header":   "nonsense",
		"wrong secret":     sign([]byte("other"), now, goodBody),
		"tampered body":    sign(testSecret, now, goodBody+" "),
		"no v1":            fmt.Sprintf("t=%d", now.Unix()),
		"non-hex v1":       fmt.Sprintf("t=%d,v1=zz", now.Unix()),
		"stale timestamp":  sign(testSecret, now.Add(-time.Hour), goodBody),
		"future timestamp": sign(testSecret, now.Add(time.Hour), goodBody),
	}
	for name, sig := range cases {
		t.Run(name, func(t *testing.T) {
			st := &memStore{}
			rec := post(NewServer(st, testSecret), goodBody, sig)
			if rec.Code != 400 {
				t.Fatalf("code %d, want 400", rec.Code)
			}
			if len(st.events) != 0 {
				t.Fatal("event stored despite rejection")
			}
		})
	}
}

func TestTimestampSwapRejected(t *testing.T) {
	// Valid v1 for an old t, header claims a fresh t.
	old := time.Now().Add(-time.Hour)
	sig := sign(testSecret, old, goodBody)
	v1 := sig[strings.Index(sig, "v1="):]
	rec := post(NewServer(&memStore{}, testSecret), goodBody, fmt.Sprintf("t=%d,%s", time.Now().Unix(), v1))
	if rec.Code != 400 {
		t.Fatalf("code %d", rec.Code)
	}
}

func TestMalformedEvent(t *testing.T) {
	for _, body := range []string{`not json`, `{"type":"x","created":1}`, `{"id":"a","created":1}`, `{"id":"a","type":"x"}`} {
		rec := post(NewServer(&memStore{}, testSecret), body, sign(testSecret, time.Now(), body))
		if rec.Code != 400 {
			t.Errorf("%q: code %d", body, rec.Code)
		}
	}
}

func TestStoreFailureIsNot2xx(t *testing.T) {
	s := NewServer(&memStore{err: errors.New("db down")}, testSecret)
	rec := post(s, goodBody, sign(testSecret, time.Now(), goodBody))
	if rec.Code < 500 {
		t.Fatalf("code %d, want 5xx so the event is retried", rec.Code)
	}
}

func TestRedeliveryIsIdempotent(t *testing.T) {
	st := &memStore{}
	s := NewServer(st, testSecret)
	for i := 0; i < 2; i++ {
		if rec := post(s, goodBody, sign(testSecret, time.Now(), goodBody)); rec.Code != 200 {
			t.Fatalf("delivery %d: code %d", i, rec.Code)
		}
	}
	if len(st.events) != 1 {
		t.Fatalf("got %d events", len(st.events))
	}
}

func TestHealthz(t *testing.T) {
	rec := httptest.NewRecorder()
	NewServer(&memStore{}, testSecret).ServeHTTP(rec, httptest.NewRequest("GET", "/healthz", nil))
	if rec.Code != http.StatusOK {
		t.Fatalf("code %d", rec.Code)
	}
}
