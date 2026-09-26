package main

import (
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"errors"
	"fmt"
	"net/http/httptest"
	"os"
	"strings"
	"testing"
	"time"
)

const secret = "whsec_test"

var now = time.Unix(1735689600, 0)

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

func sign(t int64, body, key string) string {
	mac := hmac.New(sha256.New, []byte(key))
	fmt.Fprintf(mac, "%d.%s", t, body)
	return fmt.Sprintf("t=%d,v1=%x", t, mac.Sum(nil))
}

const body = `{"id":"evt_0001","object":"event","type":"customer.created","created":1735689600,"data":{"object":{"id":"cus_1"}}}`

func do(t *testing.T, st Store, method, path, b, sig string) *httptest.ResponseRecorder {
	t.Helper()
	s := &Server{Secret: secret, Store: st, Now: func() time.Time { return now }}
	req := httptest.NewRequest(method, path, strings.NewReader(b))
	if sig != "" {
		req.Header.Set("Signature", sig)
	}
	rec := httptest.NewRecorder()
	s.Routes().ServeHTTP(rec, req)
	return rec
}

func TestValidEventStored(t *testing.T) {
	st := &memStore{}
	rec := do(t, st, "POST", "/webhooks", body, sign(now.Unix(), body, secret))
	if rec.Code != 200 {
		t.Fatalf("code %d", rec.Code)
	}
	e := st.events["evt_0001"]
	if e.Type != "customer.created" || e.Created.Unix() != 1735689600 || string(e.Payload) != body {
		t.Fatalf("bad stored event %+v", e)
	}
}

func TestInvalidSignatures(t *testing.T) {
	old := now.Add(-time.Hour).Unix()
	cases := map[string]string{
		"missing":       "",
		"garbage":       "nonsense",
		"wrong secret":  sign(now.Unix(), body, "other"),
		"tampered body": sign(now.Unix(), body+" ", secret),
		"bad hex":       fmt.Sprintf("t=%d,v1=zz", now.Unix()),
		"no v1":         fmt.Sprintf("t=%d", now.Unix()),
		"replayed old":  sign(old, body, secret),
		"future":        sign(now.Add(time.Hour).Unix(), body, secret),
	}
	for name, sig := range cases {
		t.Run(name, func(t *testing.T) {
			st := &memStore{}
			if rec := do(t, st, "POST", "/webhooks", body, sig); rec.Code != 400 {
				t.Fatalf("code %d", rec.Code)
			}
			if len(st.events) != 0 {
				t.Fatal("event stored despite invalid signature")
			}
		})
	}
}

func TestMalformedEventRejected(t *testing.T) {
	b := `{"type":"x"}`
	if rec := do(t, &memStore{}, "POST", "/webhooks", b, sign(now.Unix(), b, secret)); rec.Code != 400 {
		t.Fatalf("code %d", rec.Code)
	}
}

func TestStoreFailureNot2xx(t *testing.T) {
	st := &memStore{err: errors.New("db down")}
	rec := do(t, st, "POST", "/webhooks", body, sign(now.Unix(), body, secret))
	if rec.Code/100 == 2 {
		t.Fatalf("got %d, want non-2xx", rec.Code)
	}
}

func TestDuplicateDelivery(t *testing.T) {
	st := &memStore{}
	for i := 0; i < 2; i++ {
		if rec := do(t, st, "POST", "/webhooks", body, sign(now.Unix(), body, secret)); rec.Code != 200 {
			t.Fatalf("delivery %d: code %d", i, rec.Code)
		}
	}
	if len(st.events) != 1 {
		t.Fatalf("got %d events", len(st.events))
	}
}

func TestHealthz(t *testing.T) {
	if rec := do(t, &memStore{}, "GET", "/healthz", "", ""); rec.Code != 200 {
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
	st.pool.Exec(ctx, "DELETE FROM events WHERE id = 'evt_pgtest'")
	e := Event{ID: "evt_pgtest", Type: "customer.created", Created: now.UTC(), Payload: []byte(`{"id":"evt_pgtest"}`)}
	for i := 0; i < 2; i++ { // second save is a redelivery
		if err := st.Save(ctx, e); err != nil {
			t.Fatal(err)
		}
	}
	var n int
	var typ string
	var created time.Time
	if err := st.pool.QueryRow(ctx, "SELECT count(*), max(type), max(created_at) FROM events WHERE id='evt_pgtest'").Scan(&n, &typ, &created); err != nil {
		t.Fatal(err)
	}
	if n != 1 || typ != e.Type || !created.Equal(e.Created) {
		t.Fatalf("n=%d typ=%s created=%v", n, typ, created)
	}
}
