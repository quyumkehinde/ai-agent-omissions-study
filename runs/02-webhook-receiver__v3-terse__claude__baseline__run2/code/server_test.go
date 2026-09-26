package main

import (
	"context"
	"crypto/hmac"
	"crypto/sha256"
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

var fixedNow = time.Unix(1735689600, 0)

type fakeStore struct {
	events map[string]Event
	saves  int
	err    error
}

func (f *fakeStore) Save(_ context.Context, e Event) error {
	if f.err != nil {
		return f.err
	}
	f.saves++
	if _, ok := f.events[e.ID]; !ok {
		f.events[e.ID] = e
	}
	return nil
}

func sign(secret string, ts int64, body string) string {
	m := hmac.New(sha256.New, []byte(secret))
	fmt.Fprintf(m, "%d.%s", ts, body)
	return fmt.Sprintf("t=%d,v1=%x", ts, m.Sum(nil))
}

const body = `{"id":"evt_0001","object":"event","type":"customer.created","created":1735689600,"data":{"object":{"id":"cus_1"}}}`

func post(t *testing.T, s *Server, b, sig string) *httptest.ResponseRecorder {
	t.Helper()
	req := httptest.NewRequest("POST", "/webhooks", strings.NewReader(b))
	if sig != "" {
		req.Header.Set("Signature", sig)
	}
	rec := httptest.NewRecorder()
	s.Handler().ServeHTTP(rec, req)
	return rec
}

func newTestServer() (*Server, *fakeStore) {
	fs := &fakeStore{events: map[string]Event{}}
	s := NewServer(fs, testSecret)
	s.now = func() time.Time { return fixedNow }
	return s, fs
}

func TestValidEventStored(t *testing.T) {
	s, fs := newTestServer()
	rec := post(t, s, body, sign(testSecret, fixedNow.Unix(), body))
	if rec.Code != 200 {
		t.Fatalf("code = %d", rec.Code)
	}
	e := fs.events["evt_0001"]
	if e.Type != "customer.created" || e.Created.Unix() != 1735689600 || string(e.Payload) != body {
		t.Fatalf("bad stored event: %+v", e)
	}
}

func TestRejected(t *testing.T) {
	now := fixedNow.Unix()
	cases := map[string]struct {
		body, sig string
		code      int
	}{
		"missing header":  {body, "", 401},
		"wrong secret":    {body, sign("other", now, body), 401},
		"tampered body":   {body + " ", sign(testSecret, now, body), 401},
		"garbage header":  {body, "nonsense", 401},
		"bad hex":         {body, fmt.Sprintf("t=%d,v1=zz", now), 401},
		"stale (replay)":  {body, sign(testSecret, now-3600, body), 401},
		"future":          {body, sign(testSecret, now+3600, body), 401},
		"not json":        {"nope", sign(testSecret, now, "nope"), 400},
		"missing id":      {`{"type":"x","created":1}`, sign(testSecret, now, `{"type":"x","created":1}`), 400},
		"missing created": {`{"id":"a","type":"x"}`, sign(testSecret, now, `{"id":"a","type":"x"}`), 400},
	}
	for name, c := range cases {
		t.Run(name, func(t *testing.T) {
			s, fs := newTestServer()
			if rec := post(t, s, c.body, c.sig); rec.Code != c.code {
				t.Fatalf("code = %d, want %d", rec.Code, c.code)
			}
			if fs.saves != 0 {
				t.Fatal("event stored despite rejection")
			}
		})
	}
}

func TestStoreFailureIsNot2xx(t *testing.T) {
	s, fs := newTestServer()
	fs.err = errors.New("db down")
	rec := post(t, s, body, sign(testSecret, fixedNow.Unix(), body))
	if rec.Code != 500 {
		t.Fatalf("code = %d, want 500", rec.Code)
	}
}

func TestDuplicateDeliveryAcked(t *testing.T) {
	s, fs := newTestServer()
	sig := sign(testSecret, fixedNow.Unix(), body)
	for i := 0; i < 2; i++ {
		if rec := post(t, s, body, sig); rec.Code != 200 {
			t.Fatalf("delivery %d: code = %d", i, rec.Code)
		}
	}
	if len(fs.events) != 1 {
		t.Fatalf("events = %d", len(fs.events))
	}
}

func TestHealthz(t *testing.T) {
	s, _ := newTestServer()
	rec := httptest.NewRecorder()
	s.Handler().ServeHTTP(rec, httptest.NewRequest("GET", "/healthz", nil))
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
	defer st.pool.Exec(ctx, `DELETE FROM events WHERE id=$1`, id)

	e := Event{ID: id, Type: "customer.created", Created: time.Unix(1735689600, 0).UTC(), Payload: []byte(`{"id":"` + id + `"}`)}
	for i := 0; i < 2; i++ { // second save is a duplicate delivery
		if err := st.Save(ctx, e); err != nil {
			t.Fatalf("save %d: %v", i, err)
		}
	}
	var n int
	var typ string
	var created time.Time
	var payload string
	if err := st.pool.QueryRow(ctx, `SELECT count(*) FROM events WHERE id=$1`, id).Scan(&n); err != nil || n != 1 {
		t.Fatalf("count = %d, err = %v", n, err)
	}
	if err := st.pool.QueryRow(ctx, `SELECT type, created_at, payload::text FROM events WHERE id=$1`, id).Scan(&typ, &created, &payload); err != nil {
		t.Fatal(err)
	}
	if typ != e.Type || !created.Equal(e.Created) || !strings.Contains(payload, id) {
		t.Fatalf("got %s %v %s", typ, created, payload)
	}
}
