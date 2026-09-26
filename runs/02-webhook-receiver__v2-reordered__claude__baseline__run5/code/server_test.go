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
	"sync"
	"testing"
	"time"
)

const testSecret = "whsec_test"

type memStore struct {
	mu     sync.Mutex
	events map[string]Event
	err    error
}

func (m *memStore) SaveEvent(_ context.Context, e Event) error {
	m.mu.Lock()
	defer m.mu.Unlock()
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

func sign(secret string, t int64, body string) string {
	mac := hmac.New(sha256.New, []byte(secret))
	fmt.Fprintf(mac, "%d.%s", t, body)
	return fmt.Sprintf("t=%d,v1=%s", t, hex.EncodeToString(mac.Sum(nil)))
}

const body = `{"id":"evt_0001","object":"event","type":"customer.created","created":1735689600,"data":{"object":{"id":"cus_1"}}}`

func post(s *Server, sig, b string) *httptest.ResponseRecorder {
	req := httptest.NewRequest("POST", "/webhooks", strings.NewReader(b))
	if sig != "" {
		req.Header.Set("Signature", sig)
	}
	rec := httptest.NewRecorder()
	s.ServeHTTP(rec, req)
	return rec
}

func TestWebhook(t *testing.T) {
	now := time.Unix(1_800_000_000, 0)
	tn := now.Unix()
	cases := []struct {
		name string
		sig  string
		body string
		want int
	}{
		{"valid", sign(testSecret, tn, body), body, 200},
		{"missing header", "", body, 400},
		{"wrong secret", sign("other", tn, body), body, 400},
		{"tampered body", sign(testSecret, tn, body), strings.Replace(body, "cus_1", "cus_2", 1), 400},
		{"tampered timestamp", strings.Replace(sign(testSecret, tn, body), fmt.Sprint(tn), fmt.Sprint(tn+1), 1), body, 400},
		{"garbage header", "nonsense", body, 400},
		{"non-hex sig", fmt.Sprintf("t=%d,v1=zzzz", tn), body, 400},
		{"stale replay", sign(testSecret, tn-3600, body), body, 400},
		{"future timestamp", sign(testSecret, tn+3600, body), body, 400},
		{"malformed json", sign(testSecret, tn, "{"), "{", 400},
		{"missing id", sign(testSecret, tn, `{"type":"x","created":1}`), `{"type":"x","created":1}`, 400},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			st := &memStore{}
			s := NewServer(testSecret, st, DefaultTolerance)
			s.now = func() time.Time { return now }
			if got := post(s, c.sig, c.body).Code; got != c.want {
				t.Fatalf("status %d, want %d", got, c.want)
			}
			if c.want == 400 && len(st.events) != 0 {
				t.Fatal("rejected request was stored")
			}
		})
	}
}

func TestStoresFullEventAndDedupes(t *testing.T) {
	now := time.Unix(1_800_000_000, 0)
	st := &memStore{}
	s := NewServer(testSecret, st, DefaultTolerance)
	s.now = func() time.Time { return now }
	for i := 0; i < 2; i++ {
		if c := post(s, sign(testSecret, now.Unix(), body), body).Code; c != 200 {
			t.Fatalf("delivery %d: status %d", i, c)
		}
	}
	e, ok := st.events["evt_0001"]
	if len(st.events) != 1 || !ok {
		t.Fatalf("events: %v", st.events)
	}
	if e.Type != "customer.created" || e.Created.Unix() != 1735689600 || string(e.Payload) != body {
		t.Fatalf("bad event: %+v", e)
	}
}

func TestStoreFailureIsNot2xx(t *testing.T) {
	now := time.Unix(1_800_000_000, 0)
	s := NewServer(testSecret, &memStore{err: errors.New("db down")}, DefaultTolerance)
	s.now = func() time.Time { return now }
	if c := post(s, sign(testSecret, now.Unix(), body), body).Code; c != 500 {
		t.Fatalf("status %d, want 500", c)
	}
}

func TestHealthz(t *testing.T) {
	s := NewServer(testSecret, &memStore{}, DefaultTolerance)
	rec := httptest.NewRecorder()
	s.ServeHTTP(rec, httptest.NewRequest("GET", "/healthz", nil))
	if rec.Code != http.StatusOK {
		t.Fatalf("status %d", rec.Code)
	}
}

// Runs only when MIRROR_TEST_DATABASE_URL points at a Postgres instance.
func TestPostgresStore(t *testing.T) {
	dsn := os.Getenv("MIRROR_TEST_DATABASE_URL")
	if dsn == "" {
		t.Skip("MIRROR_TEST_DATABASE_URL not set")
	}
	ctx := context.Background()
	ps, err := NewPostgresStore(ctx, dsn)
	if err != nil {
		t.Fatal(err)
	}
	defer ps.Close()
	id := fmt.Sprintf("evt_test_%d", time.Now().UnixNano())
	defer ps.pool.Exec(ctx, "DELETE FROM events WHERE id=$1", id)
	e := Event{ID: id, Type: "customer.created", Created: time.Unix(1735689600, 0).UTC(), Payload: []byte(`{"id":"` + id + `"}`)}
	for i := 0; i < 2; i++ {
		if err := ps.SaveEvent(ctx, e); err != nil {
			t.Fatal(err)
		}
	}
	var n int
	var typ string
	if err := ps.pool.QueryRow(ctx, "SELECT count(*), max(type) FROM events WHERE id=$1", id).Scan(&n, &typ); err != nil {
		t.Fatal(err)
	}
	if n != 1 || typ != e.Type {
		t.Fatalf("n=%d type=%s", n, typ)
	}
}
