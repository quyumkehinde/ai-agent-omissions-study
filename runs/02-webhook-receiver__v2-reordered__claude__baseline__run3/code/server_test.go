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

var secret = []byte("whsec_test")

type memStore struct {
	events map[string]Event
	err    error
}

func (m *memStore) Save(_ *http.Request, e Event) error {
	if m.err != nil {
		return m.err
	}
	if _, ok := m.events[e.ID]; !ok {
		m.events[e.ID] = e
	}
	return nil
}

func sign(t int64, body string) string {
	mac := hmac.New(sha256.New, secret)
	fmt.Fprintf(mac, "%d.%s", t, body)
	return fmt.Sprintf("t=%d,v1=%s", t, hex.EncodeToString(mac.Sum(nil)))
}

const body = `{"id":"evt_0001","object":"event","type":"customer.created","created":1735689600,"data":{"object":{"id":"cus_1"}}}`

func post(s *Server, b, sig string) *httptest.ResponseRecorder {
	req := httptest.NewRequest("POST", "/webhooks", strings.NewReader(b))
	if sig != "" {
		req.Header.Set("Signature", sig)
	}
	rec := httptest.NewRecorder()
	s.ServeHTTP(rec, req)
	return rec
}

func newTestServer() (*Server, *memStore) {
	st := &memStore{events: map[string]Event{}}
	return NewServer(st, secret, DefaultTolerance), st
}

func TestValidEventStored(t *testing.T) {
	s, st := newTestServer()
	rec := post(s, body, sign(s.now().Unix(), body))
	if rec.Code != 200 {
		t.Fatalf("code %d: %s", rec.Code, rec.Body)
	}
	e := st.events["evt_0001"]
	if e.Type != "customer.created" || e.Created.Unix() != 1735689600 || string(e.Payload) != body {
		t.Fatalf("bad stored event: %+v", e)
	}
}

func TestDuplicateDeliveryOK(t *testing.T) {
	s, st := newTestServer()
	for i := 0; i < 2; i++ {
		if rec := post(s, body, sign(s.now().Unix(), body)); rec.Code != 200 {
			t.Fatalf("delivery %d: %d", i, rec.Code)
		}
	}
	if len(st.events) != 1 {
		t.Fatalf("want 1 event, got %d", len(st.events))
	}
}

func TestRejected(t *testing.T) {
	s, st := newTestServer()
	now := s.now().Unix()
	other := sign(now, `{"id":"x"}`)
	cases := map[string]string{
		"missing":        "",
		"garbage":        "hello",
		"wrong sig":      other,
		"bad hex":        fmt.Sprintf("t=%d,v1=zzzz", now),
		"no v1":          fmt.Sprintf("t=%d", now),
		"no t":           "v1=" + strings.SplitN(sign(now, body), "v1=", 2)[1],
		"stale (replay)": sign(now-3600, body),
		"future":         sign(now+3600, body),
	}
	for name, sig := range cases {
		if rec := post(s, body, sig); rec.Code != 400 {
			t.Errorf("%s: code %d, want 400", name, rec.Code)
		}
	}
	if len(st.events) != 0 {
		t.Fatal("rejected events were stored")
	}
}

func TestTamperedBody(t *testing.T) {
	s, _ := newTestServer()
	sig := sign(s.now().Unix(), body)
	if rec := post(s, strings.Replace(body, "cus_1", "cus_2", 1), sig); rec.Code != 400 {
		t.Fatalf("code %d", rec.Code)
	}
}

func TestInvalidEventBody(t *testing.T) {
	s, _ := newTestServer()
	for _, b := range []string{`not json`, `{"type":"a","created":1}`, `{"id":"a","created":1}`, `{"id":"a","type":"t"}`} {
		if rec := post(s, b, sign(s.now().Unix(), b)); rec.Code != 400 {
			t.Errorf("%q: code %d", b, rec.Code)
		}
	}
}

func TestStoreFailureNot2xx(t *testing.T) {
	s, st := newTestServer()
	st.err = errors.New("db down")
	if rec := post(s, body, sign(s.now().Unix(), body)); rec.Code != 500 {
		t.Fatalf("code %d, want 500", rec.Code)
	}
}

func TestHealthz(t *testing.T) {
	s, _ := newTestServer()
	rec := httptest.NewRecorder()
	s.ServeHTTP(rec, httptest.NewRequest("GET", "/healthz", nil))
	if rec.Code != 200 {
		t.Fatal(rec.Code)
	}
}

// Runs against a real Postgres when MIRROR_TEST_DATABASE_URL is set.
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

	s := NewServer(ps, secret, DefaultTolerance)
	b := fmt.Sprintf(`{"id":%q,"type":"customer.created","created":1735689600}`, id)
	for i := 0; i < 2; i++ {
		if rec := post(s, b, sign(s.now().Unix(), b)); rec.Code != 200 {
			t.Fatalf("code %d", rec.Code)
		}
	}
	var n int
	var typ string
	var created time.Time
	err = ps.pool.QueryRow(ctx, "SELECT count(*), max(type), max(created_at) FROM events WHERE id=$1", id).Scan(&n, &typ, &created)
	if err != nil || n != 1 || typ != "customer.created" || created.Unix() != 1735689600 {
		t.Fatalf("n=%d typ=%s created=%v err=%v", n, typ, created, err)
	}
}
