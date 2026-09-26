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

var fixedNow = time.Unix(1735689600, 0)

func sign(secret string, ts int64, body string) string {
	mac := hmac.New(sha256.New, []byte(secret))
	fmt.Fprintf(mac, "%d.%s", ts, body)
	return fmt.Sprintf("t=%d,v1=%s", ts, hex.EncodeToString(mac.Sum(nil)))
}

const evBody = `{"id":"evt_0001","object":"event","type":"customer.created","created":1735689600,"data":{"object":{"id":"cus_1"}}}`

func post(t *testing.T, st Store, body, sig string) *httptest.ResponseRecorder {
	t.Helper()
	s := NewServer(testSecret, st)
	s.now = func() time.Time { return fixedNow }
	req := httptest.NewRequest("POST", "/webhooks", strings.NewReader(body))
	if sig != "" {
		req.Header.Set("Signature", sig)
	}
	rr := httptest.NewRecorder()
	s.Routes().ServeHTTP(rr, req)
	return rr
}

func TestValidEventStored(t *testing.T) {
	st := &memStore{}
	rr := post(t, st, evBody, sign(testSecret, fixedNow.Unix(), evBody))
	if rr.Code != 200 {
		t.Fatalf("code %d: %s", rr.Code, rr.Body)
	}
	e, ok := st.events["evt_0001"]
	if !ok || e.Type != "customer.created" || e.Created.Unix() != 1735689600 || string(e.Payload) != evBody {
		t.Fatalf("bad stored event: %+v", e)
	}
}

func TestDuplicateDeliveryAcknowledged(t *testing.T) {
	st := &memStore{}
	for i := 0; i < 2; i++ {
		if rr := post(t, st, evBody, sign(testSecret, fixedNow.Unix(), evBody)); rr.Code != 200 {
			t.Fatalf("delivery %d: code %d", i, rr.Code)
		}
	}
	if len(st.events) != 1 {
		t.Fatalf("want 1 event, got %d", len(st.events))
	}
}

func TestRejected(t *testing.T) {
	now := fixedNow.Unix()
	good := sign(testSecret, now, evBody)
	cases := map[string]struct{ body, sig string }{
		"missing header":   {evBody, ""},
		"wrong secret":     {evBody, sign("other", now, evBody)},
		"tampered body":    {strings.Replace(evBody, "cus_1", "cus_2", 1), good},
		"garbage header":   {evBody, "hello"},
		"non-hex v1":       {evBody, fmt.Sprintf("t=%d,v1=zz", now)},
		"missing v1":       {evBody, fmt.Sprintf("t=%d", now)},
		"stale timestamp":  {evBody, sign(testSecret, now-3600, evBody)},
		"future timestamp": {evBody, sign(testSecret, now+3600, evBody)},
		"t altered":        {evBody, strings.Replace(good, fmt.Sprint(now), fmt.Sprint(now+1), 1)},
	}
	for name, c := range cases {
		t.Run(name, func(t *testing.T) {
			st := &memStore{}
			if rr := post(t, st, c.body, c.sig); rr.Code != 400 {
				t.Fatalf("code %d, want 400", rr.Code)
			}
			if len(st.events) != 0 {
				t.Fatal("event stored despite rejection")
			}
		})
	}
}

func TestMalformedSignedEvent(t *testing.T) {
	for _, body := range []string{`not json`, `{"id":"","type":"x","created":1}`, `{"id":"a","type":"x"}`} {
		if rr := post(t, &memStore{}, body, sign(testSecret, fixedNow.Unix(), body)); rr.Code != 400 {
			t.Errorf("%q: code %d, want 400", body, rr.Code)
		}
	}
}

func TestStoreFailureIsNot2xx(t *testing.T) {
	st := &memStore{err: errors.New("db down")}
	rr := post(t, st, evBody, sign(testSecret, fixedNow.Unix(), evBody))
	if rr.Code/100 == 2 {
		t.Fatalf("code %d, want non-2xx", rr.Code)
	}
}

func TestHealthz(t *testing.T) {
	rr := httptest.NewRecorder()
	NewServer(testSecret, &memStore{}).Routes().ServeHTTP(rr, httptest.NewRequest("GET", "/healthz", nil))
	if rr.Code != http.StatusOK {
		t.Fatalf("code %d", rr.Code)
	}
}

// Runs against a real Postgres when MIRROR_TEST_DATABASE_URL is set.
func TestPostgresStore(t *testing.T) {
	url := os.Getenv("MIRROR_TEST_DATABASE_URL")
	if url == "" {
		t.Skip("MIRROR_TEST_DATABASE_URL not set")
	}
	ctx := context.Background()
	s, err := NewPostgresStore(ctx, url)
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	id := fmt.Sprintf("evt_test_%d", time.Now().UnixNano())
	defer s.pool.Exec(ctx, "DELETE FROM events WHERE id=$1", id)

	e := Event{ID: id, Type: "customer.created", Created: time.Unix(1735689600, 0).UTC(), Payload: []byte(`{"id":"` + id + `"}`)}
	for i := 0; i < 2; i++ {
		if err := s.SaveEvent(ctx, e); err != nil {
			t.Fatal(err)
		}
	}
	var n int
	var typ string
	if err := s.pool.QueryRow(ctx, "SELECT count(*), max(type) FROM events WHERE id=$1", id).Scan(&n, &typ); err != nil {
		t.Fatal(err)
	}
	if n != 1 || typ != "customer.created" {
		t.Fatalf("n=%d type=%s", n, typ)
	}
}
