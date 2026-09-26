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

var testNow = time.Unix(1735689600, 0)

type fakeStore struct {
	events map[string]Event
	err    error
}

func (f *fakeStore) SaveEvent(_ context.Context, e Event) error {
	if f.err != nil {
		return f.err
	}
	if _, ok := f.events[e.ID]; !ok {
		f.events[e.ID] = e
	}
	return nil
}

const body = `{"id":"evt_0001","object":"event","type":"customer.created","created":1735689600,"data":{"object":{"id":"cus_1"}}}`

func post(t *testing.T, s *Server, b, sig string) *httptest.ResponseRecorder {
	t.Helper()
	req := httptest.NewRequest("POST", "/webhooks", strings.NewReader(b))
	if sig != "" {
		req.Header.Set("Signature", sig)
	}
	rec := httptest.NewRecorder()
	s.Routes().ServeHTTP(rec, req)
	return rec
}

func sign(ts int64, b string) string {
	return fmt.Sprintf("t=%d,v1=%s", ts, computeSignature(testSecret, ts, []byte(b)))
}

func newServer() (*Server, *fakeStore) {
	fs := &fakeStore{events: map[string]Event{}}
	return &Server{secret: testSecret, store: fs, now: func() time.Time { return testNow }}, fs
}

func TestValidEventStored(t *testing.T) {
	s, fs := newServer()
	rec := post(t, s, body, sign(testNow.Unix(), body))
	if rec.Code != 200 {
		t.Fatalf("code = %d", rec.Code)
	}
	e, ok := fs.events["evt_0001"]
	if !ok || e.Type != "customer.created" || !e.Created.Equal(testNow) || string(e.Payload) != body {
		t.Fatalf("bad stored event: %+v", e)
	}
}

func TestDuplicateDeliveryAcknowledged(t *testing.T) {
	s, fs := newServer()
	for i := 0; i < 2; i++ {
		if rec := post(t, s, body, sign(testNow.Unix(), body)); rec.Code != 200 {
			t.Fatalf("delivery %d code = %d", i, rec.Code)
		}
	}
	if len(fs.events) != 1 {
		t.Fatalf("events = %d", len(fs.events))
	}
}

func TestRejectedRequests(t *testing.T) {
	old := testNow.Add(-10 * time.Minute).Unix()
	cases := map[string]struct{ body, sig string }{
		"missing header":  {body, ""},
		"garbage header":  {body, "nonsense"},
		"wrong signature": {body, fmt.Sprintf("t=%d,v1=deadbeef", testNow.Unix())},
		"tampered body":   {body + " ", sign(testNow.Unix(), body)},
		"wrong secret":    {body, fmt.Sprintf("t=%d,v1=%s", testNow.Unix(), computeSignature("other", testNow.Unix(), []byte(body)))},
		"replayed (old)":  {body, sign(old, body)},
		"tampered t":      {body, fmt.Sprintf("t=%d,v1=%s", testNow.Unix()+1, computeSignature(testSecret, testNow.Unix(), []byte(body)))},
	}
	for name, c := range cases {
		t.Run(name, func(t *testing.T) {
			s, fs := newServer()
			if rec := post(t, s, c.body, c.sig); rec.Code != 400 {
				t.Fatalf("code = %d", rec.Code)
			}
			if len(fs.events) != 0 {
				t.Fatal("event stored")
			}
		})
	}
}

func TestMalformedSignedBody(t *testing.T) {
	s, _ := newServer()
	for _, b := range []string{"not json", `{"type":"x"}`} {
		if rec := post(t, s, b, sign(testNow.Unix(), b)); rec.Code != 400 {
			t.Fatalf("%q: code = %d", b, rec.Code)
		}
	}
}

func TestStoreFailureNot2xx(t *testing.T) {
	s, fs := newServer()
	fs.err = errors.New("db down")
	if rec := post(t, s, body, sign(testNow.Unix(), body)); rec.Code != 500 {
		t.Fatalf("code = %d", rec.Code)
	}
}

func TestHealthz(t *testing.T) {
	s, _ := newServer()
	rec := httptest.NewRecorder()
	s.Routes().ServeHTTP(rec, httptest.NewRequest("GET", "/healthz", nil))
	if rec.Code != http.StatusOK {
		t.Fatalf("code = %d", rec.Code)
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

	e := Event{ID: id, Type: "customer.created", Created: testNow.UTC(), Payload: []byte(body)}
	for i := 0; i < 2; i++ { // second call exercises redelivery
		if err := st.SaveEvent(ctx, e); err != nil {
			t.Fatal(err)
		}
	}
	var n int
	var typ string
	var created time.Time
	var payload string
	if err := st.pool.QueryRow(ctx, "SELECT count(*), max(type), max(created_at), max(payload::text) FROM events WHERE id=$1", id).Scan(&n, &typ, &created, &payload); err != nil {
		t.Fatal(err)
	}
	if n != 1 || typ != e.Type || !created.Equal(e.Created) || !strings.Contains(payload, "evt_0001") {
		t.Fatalf("n=%d typ=%s created=%v payload=%s", n, typ, created, payload)
	}
}
