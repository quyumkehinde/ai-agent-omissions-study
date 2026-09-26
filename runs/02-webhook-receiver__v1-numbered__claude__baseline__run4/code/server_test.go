package main

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"strconv"
	"testing"
	"time"
)

const testSecret = "whsec_test"

type memStore struct {
	events map[string]Event
	err    error
}

func (m *memStore) SaveEvent(_ context.Context, e Event) error {
	if m.err != nil {
		return m.err
	}
	if _, ok := m.events[e.ID]; !ok {
		m.events[e.ID] = e
	}
	return nil
}

var fixedNow = time.Unix(1735689600, 0)

func newTest(t *testing.T) (*Server, *memStore) {
	st := &memStore{events: map[string]Event{}}
	s := NewServer(testSecret, st)
	s.now = func() time.Time { return fixedNow }
	return s, st
}

const body = `{"id":"evt_0001","object":"event","type":"customer.created","created":1735689600,"data":{"object":{"id":"cus_1"}}}`

func post(s *Server, payload, header string) *httptest.ResponseRecorder {
	req := httptest.NewRequest("POST", "/webhooks", bytes.NewBufferString(payload))
	if header != "" {
		req.Header.Set("Signature", header)
	}
	rec := httptest.NewRecorder()
	s.Handler().ServeHTTP(rec, req)
	return rec
}

func header(t time.Time, payload string) string {
	ts := strconv.FormatInt(t.Unix(), 10)
	return fmt.Sprintf("t=%s,v1=%s", ts, sign([]byte(testSecret), ts, []byte(payload)))
}

func TestValidEventStored(t *testing.T) {
	s, st := newTest(t)
	if rec := post(s, body, header(fixedNow, body)); rec.Code != 200 {
		t.Fatalf("code %d", rec.Code)
	}
	e, ok := st.events["evt_0001"]
	if !ok || e.Type != "customer.created" || e.Created.Unix() != 1735689600 || string(e.Payload) != body {
		t.Fatalf("bad stored event: %+v", e)
	}
}

func TestDuplicateDeliveryOK(t *testing.T) {
	s, st := newTest(t)
	for i := 0; i < 2; i++ {
		if rec := post(s, body, header(fixedNow, body)); rec.Code != 200 {
			t.Fatalf("code %d", rec.Code)
		}
	}
	if len(st.events) != 1 {
		t.Fatal("expected one event")
	}
}

func TestRejected(t *testing.T) {
	other := `{"id":"evt_0002","type":"x","created":1}`
	cases := map[string]string{
		"missing":       "",
		"garbage":       "nonsense",
		"no v1":         "t=1735689600",
		"no t":          "v1=abcd",
		"wrong sig":     "t=1735689600,v1=" + fmt.Sprintf("%064d", 0),
		"tampered body": header(fixedNow, other),
		"old replay":    header(fixedNow.Add(-time.Hour), body),
		"future":        header(fixedNow.Add(time.Hour), body),
		"bad t":         "t=abc,v1=00",
	}
	for name, h := range cases {
		t.Run(name, func(t *testing.T) {
			s, st := newTest(t)
			if rec := post(s, body, h); rec.Code != 400 {
				t.Fatalf("code %d", rec.Code)
			}
			if len(st.events) != 0 {
				t.Fatal("event stored")
			}
		})
	}
}

func TestWrongSecretRejected(t *testing.T) {
	s, _ := newTest(t)
	ts := strconv.FormatInt(fixedNow.Unix(), 10)
	h := "t=" + ts + ",v1=" + sign([]byte("other"), ts, []byte(body))
	if rec := post(s, body, h); rec.Code != 400 {
		t.Fatalf("code %d", rec.Code)
	}
}

func TestInvalidEventBody(t *testing.T) {
	s, _ := newTest(t)
	for _, b := range []string{`not json`, `{"type":"a","created":1}`, `{"id":"e","created":1}`, `{"id":"e","type":"a"}`} {
		if rec := post(s, b, header(fixedNow, b)); rec.Code != 400 {
			t.Fatalf("%s: code %d", b, rec.Code)
		}
	}
}

func TestStoreFailureNot2xx(t *testing.T) {
	s, st := newTest(t)
	st.err = errors.New("db down")
	if rec := post(s, body, header(fixedNow, body)); rec.Code != 500 {
		t.Fatalf("code %d", rec.Code)
	}
}

func TestHealthz(t *testing.T) {
	s, _ := newTest(t)
	rec := httptest.NewRecorder()
	s.Handler().ServeHTTP(rec, httptest.NewRequest("GET", "/healthz", nil))
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
	p, err := NewPGStore(ctx, url)
	if err != nil {
		t.Fatal(err)
	}
	defer p.Close()
	id := fmt.Sprintf("evt_test_%d", time.Now().UnixNano())
	defer p.pool.Exec(ctx, "DELETE FROM events WHERE id=$1", id)
	e := Event{ID: id, Type: "customer.created", Created: time.Unix(1735689600, 0).UTC(), Payload: []byte(`{"a":1}`)}
	for i := 0; i < 2; i++ {
		if err := p.SaveEvent(ctx, e); err != nil {
			t.Fatal(err)
		}
	}
	var n int
	var typ string
	var created time.Time
	if err := p.pool.QueryRow(ctx, "SELECT count(*), max(type), max(created_at) FROM events WHERE id=$1", id).Scan(&n, &typ, &created); err != nil {
		t.Fatal(err)
	}
	if n != 1 || typ != e.Type || !created.Equal(e.Created) {
		t.Fatalf("got %d %s %v", n, typ, created)
	}
}
