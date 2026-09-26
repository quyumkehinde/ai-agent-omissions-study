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
	"os"
	"strings"
	"testing"
	"time"
)

const secret = "whsec_test"

var fixedNow = time.Unix(1735689600, 0)

type fakeStore struct {
	events map[string]Event
	err    error
}

func (f *fakeStore) SaveEvent(_ *http.Request, e Event) error {
	if f.err != nil {
		return f.err
	}
	if _, ok := f.events[e.ID]; !ok {
		f.events[e.ID] = e
	}
	return nil
}

func sign(secret string, t int64, body string) string {
	m := hmac.New(sha256.New, []byte(secret))
	fmt.Fprintf(m, "%d.%s", t, body)
	return fmt.Sprintf("t=%d,v1=%s", t, hex.EncodeToString(m.Sum(nil)))
}

const body = `{"id":"evt_0001","object":"event","type":"customer.created","created":1735689600,"data":{"object":{"id":"cus_1"}}}`

func post(s *Server, b, sig string) *httptest.ResponseRecorder {
	req := httptest.NewRequest("POST", "/webhooks", strings.NewReader(b))
	if sig != "" {
		req.Header.Set("Signature", sig)
	}
	rec := httptest.NewRecorder()
	s.Handler().ServeHTTP(rec, req)
	return rec
}

func newServer() (*Server, *fakeStore) {
	fs := &fakeStore{events: map[string]Event{}}
	return &Server{Secret: secret, Store: fs, Now: func() time.Time { return fixedNow }}, fs
}

func TestValidEventStored(t *testing.T) {
	s, fs := newServer()
	rec := post(s, body, sign(secret, fixedNow.Unix(), body))
	if rec.Code != 200 {
		t.Fatalf("code %d", rec.Code)
	}
	e, ok := fs.events["evt_0001"]
	if !ok || e.Type != "customer.created" || !e.Created.Equal(time.Unix(1735689600, 0)) || string(e.Payload) != body {
		t.Fatalf("bad stored event: %+v", e)
	}
}

func TestDuplicateDeliveryOK(t *testing.T) {
	s, fs := newServer()
	sig := sign(secret, fixedNow.Unix(), body)
	for i := 0; i < 2; i++ {
		if rec := post(s, body, sig); rec.Code != 200 {
			t.Fatalf("delivery %d: %d", i, rec.Code)
		}
	}
	if len(fs.events) != 1 {
		t.Fatalf("want 1 event, got %d", len(fs.events))
	}
}

func TestRejectedRequests(t *testing.T) {
	now := fixedNow.Unix()
	old := now - int64(signatureTolerance/time.Second) - 1
	cases := map[string]string{
		"missing header":  "",
		"garbage header":  "nonsense",
		"wrong secret":    sign("other", now, body),
		"tampered body":   sign(secret, now, body+" "),
		"no v1":           fmt.Sprintf("t=%d", now),
		"no t":            "v1=abcd",
		"replayed (old)":  sign(secret, old, body),
		"future":          sign(secret, now+int64(signatureTolerance/time.Second)+1, body),
		"bad t with sig":  strings.Replace(sign(secret, now, body), "t=", "t=x", 1),
		"mismatched time": strings.Replace(sign(secret, now, body), fmt.Sprint(now), fmt.Sprint(now+1), 1),
	}
	for name, sig := range cases {
		t.Run(name, func(t *testing.T) {
			s, fs := newServer()
			if rec := post(s, body, sig); rec.Code != 400 {
				t.Fatalf("code %d", rec.Code)
			}
			if len(fs.events) != 0 {
				t.Fatal("event stored")
			}
		})
	}
}

func TestInvalidEventBody(t *testing.T) {
	for _, b := range []string{`not json`, `{"type":"x","created":1}`, `{"id":"a","created":1}`, `{"id":"a","type":"x"}`} {
		s, _ := newServer()
		if rec := post(s, b, sign(secret, fixedNow.Unix(), b)); rec.Code != 400 {
			t.Errorf("%q: code %d", b, rec.Code)
		}
	}
}

func TestStoreFailureNot2xx(t *testing.T) {
	s, fs := newServer()
	fs.err = errors.New("db down")
	if rec := post(s, body, sign(secret, fixedNow.Unix(), body)); rec.Code != 500 {
		t.Fatalf("code %d", rec.Code)
	}
}

func TestHealthz(t *testing.T) {
	s, _ := newServer()
	rec := httptest.NewRecorder()
	s.Handler().ServeHTTP(rec, httptest.NewRequest("GET", "/healthz", nil))
	if rec.Code != 200 {
		t.Fatalf("code %d", rec.Code)
	}
}

// Runs only when TEST_DATABASE_URL points at a Postgres instance.
func TestPostgresStore(t *testing.T) {
	url := os.Getenv("TEST_DATABASE_URL")
	if url == "" {
		t.Skip("TEST_DATABASE_URL not set")
	}
	ctx := context.Background()
	ps, err := NewPostgresStore(ctx, url)
	if err != nil {
		t.Fatal(err)
	}
	defer ps.Close()
	id := fmt.Sprintf("evt_test_%d", time.Now().UnixNano())
	defer ps.pool.Exec(ctx, "DELETE FROM events WHERE id=$1", id)
	payload := `{"id":"` + id + `","type":"customer.created","created":1735689600}`
	s := &Server{Secret: secret, Store: ps}
	for i := 0; i < 2; i++ {
		rec := post(s, payload, sign(secret, time.Now().Unix(), payload))
		if rec.Code != 200 {
			t.Fatalf("code %d", rec.Code)
		}
	}
	var n int
	var typ string
	var created time.Time
	var p string
	if err := ps.pool.QueryRow(ctx, "SELECT count(*), max(type), max(created_at), max(payload::text) FROM events WHERE id=$1", id).Scan(&n, &typ, &created, &p); err != nil {
		t.Fatal(err)
	}
	if n != 1 || typ != "customer.created" || created.Unix() != 1735689600 || !strings.Contains(p, id) {
		t.Fatalf("got %d %s %v %s", n, typ, created, p)
	}
}
