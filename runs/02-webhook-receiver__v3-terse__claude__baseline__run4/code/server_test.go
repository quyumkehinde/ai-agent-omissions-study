package main

import (
	"context"
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

var fixedNow = time.Unix(1735689600, 0)

type memStore struct {
	mu     sync.Mutex
	events map[string]Event
	err    error
}

func (m *memStore) Save(_ context.Context, e Event) error {
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
func (m *memStore) Ping(context.Context) error { return nil }

const body = `{"id":"evt_0001","object":"event","type":"customer.created","created":1735689600,"data":{"object":{"id":"cus_1"}}}`

func post(t *testing.T, s *Server, payload, header string) *httptest.ResponseRecorder {
	t.Helper()
	req := httptest.NewRequest("POST", "/webhooks", strings.NewReader(payload))
	if header != "" {
		req.Header.Set("Signature", header)
	}
	rec := httptest.NewRecorder()
	s.Handler().ServeHTTP(rec, req)
	return rec
}

func hdr(secret string, ts int64, payload string) string {
	return fmt.Sprintf("t=%d,v1=%s", ts, sign(secret, ts, []byte(payload)))
}

func newTest(st Store) *Server {
	s := NewServer(testSecret, st)
	s.now = func() time.Time { return fixedNow }
	return s
}

func TestValidEventStored(t *testing.T) {
	st := &memStore{}
	rec := post(t, newTest(st), body, hdr(testSecret, fixedNow.Unix(), body))
	if rec.Code != 200 {
		t.Fatalf("code %d: %s", rec.Code, rec.Body)
	}
	e := st.events["evt_0001"]
	if e.Type != "customer.created" || !e.Created.Equal(time.Unix(1735689600, 0)) || string(e.Payload) != body {
		t.Fatalf("bad stored event: %+v", e)
	}
}

func TestDuplicateDelivery(t *testing.T) {
	st := &memStore{}
	s := newTest(st)
	for i := 0; i < 2; i++ {
		if rec := post(t, s, body, hdr(testSecret, fixedNow.Unix(), body)); rec.Code != 200 {
			t.Fatalf("delivery %d: %d", i, rec.Code)
		}
	}
	if len(st.events) != 1 {
		t.Fatalf("want 1 event, got %d", len(st.events))
	}
}

func TestRejected(t *testing.T) {
	now := fixedNow.Unix()
	cases := map[string]string{
		"missing":       "",
		"garbage":       "nope",
		"wrong secret":  hdr("other", now, body),
		"tampered body": hdr(testSecret, now, body+" "),
		"no v1":         fmt.Sprintf("t=%d", now),
		"no t":          "v1=" + sign(testSecret, now, []byte(body)),
		"old replay":    hdr(testSecret, now-3600, body),
		"future":        hdr(testSecret, now+3600, body),
	}
	for name, h := range cases {
		t.Run(name, func(t *testing.T) {
			st := &memStore{}
			rec := post(t, newTest(st), body, h)
			if rec.Code != 401 {
				t.Fatalf("code %d", rec.Code)
			}
			if len(st.events) != 0 {
				t.Fatal("event stored despite rejection")
			}
		})
	}
}

func TestInvalidEvent(t *testing.T) {
	for _, p := range []string{`not json`, `{}`, `{"id":"e","type":"x"}`, `{"id":"","type":"x","created":1}`} {
		if rec := post(t, newTest(&memStore{}), p, hdr(testSecret, fixedNow.Unix(), p)); rec.Code != 400 {
			t.Errorf("%q: code %d", p, rec.Code)
		}
	}
}

func TestStoreFailureIsNot2xx(t *testing.T) {
	st := &memStore{err: errors.New("db down")}
	rec := post(t, newTest(st), body, hdr(testSecret, fixedNow.Unix(), body))
	if rec.Code != 500 {
		t.Fatalf("code %d", rec.Code)
	}
}

func TestHealthz(t *testing.T) {
	rec := httptest.NewRecorder()
	newTest(&memStore{}).Handler().ServeHTTP(rec, httptest.NewRequest("GET", "/healthz", nil))
	if rec.Code != http.StatusOK {
		t.Fatalf("code %d", rec.Code)
	}
}

// Runs against a real Postgres when MIRROR_TEST_DATABASE_URL is set.
func TestPGStore(t *testing.T) {
	url := os.Getenv("MIRROR_TEST_DATABASE_URL")
	if url == "" {
		t.Skip("MIRROR_TEST_DATABASE_URL not set")
	}
	ctx := context.Background()
	st, err := NewPGStore(ctx, url)
	if err != nil {
		t.Fatal(err)
	}
	defer st.Close()
	id := fmt.Sprintf("evt_test_%d", time.Now().UnixNano())
	defer st.pool.Exec(ctx, "DELETE FROM events WHERE id=$1", id)

	e := Event{ID: id, Type: "customer.created", Created: time.Unix(1735689600, 0).UTC(), Payload: []byte(`{"a":1}`)}
	for i := 0; i < 2; i++ {
		if err := st.Save(ctx, e); err != nil {
			t.Fatal(err)
		}
	}
	var n int
	var typ string
	var payload string
	err = st.pool.QueryRow(ctx, "SELECT count(*), max(type), max(payload::text) FROM events WHERE id=$1", id).Scan(&n, &typ, &payload)
	if err != nil || n != 1 || typ != e.Type || payload != `{"a": 1}` {
		t.Fatalf("n=%d typ=%s payload=%s err=%v", n, typ, payload, err)
	}
}
