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

var testSecret = []byte("whsec_test")

var fixedNow = time.Unix(1735689600, 0)

const testBody = `{"id":"evt_0001","object":"event","type":"customer.created","created":1735689600,"data":{"object":{"id":"cus_1"}}}`

func sign(secret []byte, t int64, body string) string {
	m := hmac.New(sha256.New, secret)
	fmt.Fprintf(m, "%d.%s", t, body)
	return fmt.Sprintf("t=%d,v1=%x", t, m.Sum(nil))
}

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

func post(s *Server, body, sig string) *httptest.ResponseRecorder {
	req := httptest.NewRequest("POST", "/webhooks", strings.NewReader(body))
	if sig != "" {
		req.Header.Set("Signature", sig)
	}
	rec := httptest.NewRecorder()
	s.Handler().ServeHTTP(rec, req)
	return rec
}

func newServer(st Store) *Server {
	return &Server{Secret: testSecret, Store: st, Now: func() time.Time { return fixedNow }}
}

func TestValidEventStored(t *testing.T) {
	st := &memStore{}
	rec := post(newServer(st), testBody, sign(testSecret, fixedNow.Unix(), testBody))
	if rec.Code != 200 {
		t.Fatalf("code %d: %s", rec.Code, rec.Body)
	}
	e := st.events["evt_0001"]
	if e.Type != "customer.created" || !e.Created.Equal(time.Unix(1735689600, 0)) || string(e.Payload) != testBody {
		t.Fatalf("bad stored event: %+v", e)
	}
}

func TestRejections(t *testing.T) {
	old := fixedNow.Unix() - 3600
	cases := map[string]struct{ body, sig string }{
		"missing header": {testBody, ""},
		"garbage header": {testBody, "nope"},
		"wrong secret":   {testBody, sign([]byte("other"), fixedNow.Unix(), testBody)},
		"tampered body":  {testBody + " ", sign(testSecret, fixedNow.Unix(), testBody)},
		"stale replay":   {testBody, sign(testSecret, old, testBody)},
		"future":         {testBody, sign(testSecret, fixedNow.Unix()+3600, testBody)},
		"bad hex":        {testBody, fmt.Sprintf("t=%d,v1=zz", fixedNow.Unix())},
		"no v1":          {testBody, fmt.Sprintf("t=%d", fixedNow.Unix())},
		"tampered t":     {testBody, strings.Replace(sign(testSecret, fixedNow.Unix(), testBody), fmt.Sprint(fixedNow.Unix()), fmt.Sprint(fixedNow.Unix()+1), 1)},
	}
	for name, c := range cases {
		t.Run(name, func(t *testing.T) {
			st := &memStore{}
			rec := post(newServer(st), c.body, c.sig)
			if rec.Code != 401 {
				t.Fatalf("code %d", rec.Code)
			}
			if len(st.events) != 0 {
				t.Fatal("event stored despite rejection")
			}
		})
	}
}

func TestInvalidPayloadSigned(t *testing.T) {
	for _, body := range []string{`not json`, `{"type":"x","created":1}`, `{"id":"a","type":"x"}`} {
		rec := post(newServer(&memStore{}), body, sign(testSecret, fixedNow.Unix(), body))
		if rec.Code != 400 {
			t.Errorf("%q: code %d", body, rec.Code)
		}
	}
}

func TestStoreFailureIsNot2xx(t *testing.T) {
	rec := post(newServer(&memStore{err: errors.New("down")}), testBody, sign(testSecret, fixedNow.Unix(), testBody))
	if rec.Code != 500 {
		t.Fatalf("code %d", rec.Code)
	}
}

func TestDuplicateDelivery(t *testing.T) {
	st := &memStore{}
	s := newServer(st)
	for i := 0; i < 2; i++ {
		if rec := post(s, testBody, sign(testSecret, fixedNow.Unix(), testBody)); rec.Code != 200 {
			t.Fatalf("delivery %d: %d", i, rec.Code)
		}
	}
	if len(st.events) != 1 {
		t.Fatalf("got %d events", len(st.events))
	}
}

func TestHealthz(t *testing.T) {
	rec := httptest.NewRecorder()
	newServer(&memStore{}).Handler().ServeHTTP(rec, httptest.NewRequest("GET", "/healthz", nil))
	if rec.Code != http.StatusOK {
		t.Fatalf("code %d", rec.Code)
	}
}

// Runs against a real Postgres when MIRROR_DATABASE_URL is set and reachable.
func TestPGStore(t *testing.T) {
	url := os.Getenv("MIRROR_DATABASE_URL")
	if url == "" {
		t.Skip("MIRROR_DATABASE_URL not set")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	st, err := NewPGStore(ctx, url)
	if err != nil {
		t.Skipf("postgres unavailable: %v", err)
	}
	defer st.Close()
	id := fmt.Sprintf("evt_test_%d", time.Now().UnixNano())
	defer st.pool.Exec(ctx, `DELETE FROM events WHERE id=$1`, id)

	e := Event{ID: id, Type: "customer.created", Created: time.Unix(1735689600, 0).UTC(), Payload: []byte(`{"id":"` + id + `"}`)}
	for i := 0; i < 2; i++ { // second is a redelivery
		if err := st.Save(ctx, e); err != nil {
			t.Fatal(err)
		}
	}
	var n int
	var typ string
	var created time.Time
	var payload string
	if err := st.pool.QueryRow(ctx, `SELECT count(*) FROM events WHERE id=$1`, id).Scan(&n); err != nil || n != 1 {
		t.Fatalf("count=%d err=%v", n, err)
	}
	if err := st.pool.QueryRow(ctx, `SELECT type, created_at, payload::text FROM events WHERE id=$1`, id).Scan(&typ, &created, &payload); err != nil {
		t.Fatal(err)
	}
	if typ != e.Type || !created.Equal(e.Created) || !strings.Contains(payload, id) {
		t.Fatalf("got %s %v %s", typ, created, payload)
	}
}
