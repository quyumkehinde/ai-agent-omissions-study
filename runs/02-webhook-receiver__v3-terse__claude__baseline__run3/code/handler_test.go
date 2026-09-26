package main

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"
	"time"
)

const testSecret = "whsec_test"

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
func (m *memStore) Ping(context.Context) error { return nil }

var fixedNow = time.Unix(1735689600, 0)

const body = `{"id":"evt_0001","object":"event","type":"customer.created","created":1735689600,"data":{"object":{"id":"cus_1"}}}`

func post(t *testing.T, s Store, b, sig string) *httptest.ResponseRecorder {
	t.Helper()
	srv := &Server{Secret: testSecret, Store: s, Now: func() time.Time { return fixedNow }}
	req := httptest.NewRequest("POST", "/webhooks", strings.NewReader(b))
	if sig != "" {
		req.Header.Set("Signature", sig)
	}
	rec := httptest.NewRecorder()
	srv.Routes().ServeHTTP(rec, req)
	return rec
}

func header(ts int64, b string) string {
	return fmt.Sprintf("t=%d,v1=%s", ts, sign(testSecret, ts, []byte(b)))
}

func TestValidEventStored(t *testing.T) {
	st := &memStore{}
	rec := post(t, st, body, header(fixedNow.Unix(), body))
	if rec.Code != 200 {
		t.Fatalf("code %d", rec.Code)
	}
	e := st.events["evt_0001"]
	if e.Type != "customer.created" || e.Created.Unix() != 1735689600 || string(e.Payload) != body {
		t.Fatalf("bad stored event %+v", e)
	}
}

func TestDuplicateDelivery(t *testing.T) {
	st := &memStore{}
	for i := 0; i < 2; i++ {
		if rec := post(t, st, body, header(fixedNow.Unix(), body)); rec.Code != 200 {
			t.Fatalf("delivery %d: %d", i, rec.Code)
		}
	}
	if len(st.events) != 1 {
		t.Fatalf("want 1 event, got %d", len(st.events))
	}
}

func TestRejections(t *testing.T) {
	now := fixedNow.Unix()
	tests := map[string]string{
		"missing header":  "",
		"garbage header":  "nonsense",
		"wrong signature": fmt.Sprintf("t=%d,v1=%s", now, strings.Repeat("0", 64)),
		"tampered body":   header(now, body+" "),
		"no v1":           fmt.Sprintf("t=%d", now),
		"stale timestamp": header(now-3600, body),
		"future":          header(now+3600, body),
		"wrong secret":    fmt.Sprintf("t=%d,v1=%s", now, sign("other", now, []byte(body))),
	}
	for name, sig := range tests {
		t.Run(name, func(t *testing.T) {
			st := &memStore{}
			rec := post(t, st, body, sig)
			if rec.Code != http.StatusUnauthorized {
				t.Fatalf("code %d", rec.Code)
			}
			if len(st.events) != 0 {
				t.Fatal("event stored despite bad signature")
			}
		})
	}
}

func TestStoreFailureIsNot2xx(t *testing.T) {
	rec := post(t, &memStore{err: errors.New("db down")}, body, header(fixedNow.Unix(), body))
	if rec.Code < 500 {
		t.Fatalf("code %d", rec.Code)
	}
}

func TestInvalidPayloads(t *testing.T) {
	for _, b := range []string{`not json`, `{"type":"x","created":1}`, `{"id":"e","created":1}`, `{"id":"e","type":"x"}`} {
		rec := post(t, &memStore{}, b, header(fixedNow.Unix(), b))
		if rec.Code != 400 {
			t.Errorf("%s: code %d", b, rec.Code)
		}
	}
}

func TestHealthz(t *testing.T) {
	srv := &Server{Store: &memStore{}, Now: time.Now}
	rec := httptest.NewRecorder()
	srv.Routes().ServeHTTP(rec, httptest.NewRequest("GET", "/healthz", nil))
	if rec.Code != 200 {
		t.Fatal(rec.Code)
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
	defer st.pool.Exec(ctx, `DELETE FROM events WHERE id=$1`, id)

	e := Event{ID: id, Type: "customer.created", Created: time.Unix(1735689600, 0).UTC(), Payload: []byte(`{"id":"` + id + `"}`)}
	for i := 0; i < 2; i++ {
		if err := st.Save(ctx, e); err != nil {
			t.Fatal(err)
		}
	}
	var n int
	var typ string
	var created time.Time
	if err := st.pool.QueryRow(ctx, `SELECT count(*), max(type), max(created_at) FROM events WHERE id=$1`, id).Scan(&n, &typ, &created); err != nil {
		t.Fatal(err)
	}
	if n != 1 || typ != e.Type || !created.Equal(e.Created) {
		t.Fatalf("n=%d typ=%s created=%v", n, typ, created)
	}
}
