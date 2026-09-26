package main

import (
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"
	"time"
)

var secret = []byte("whsec_test")
var fixedNow = time.Unix(1735689600, 0)

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

func sign(body string, t int64) string {
	mac := hmac.New(sha256.New, secret)
	fmt.Fprintf(mac, "%d.%s", t, body)
	return fmt.Sprintf("t=%d,v1=%s", t, hex.EncodeToString(mac.Sum(nil)))
}

const body = `{"id":"evt_0001","object":"event","type":"customer.created","created":1735689600,"data":{"object":{"id":"cus_1"}}}`

func do(t *testing.T, st Store, b, sig string) *httptest.ResponseRecorder {
	t.Helper()
	s := &Server{Secret: secret, Store: st, Now: func() time.Time { return fixedNow }}
	req := httptest.NewRequest("POST", "/webhooks", strings.NewReader(b))
	if sig != "" {
		req.Header.Set("Signature", sig)
	}
	rec := httptest.NewRecorder()
	s.Routes().ServeHTTP(rec, req)
	return rec
}

func TestValidEventStored(t *testing.T) {
	st := &memStore{events: map[string]Event{}}
	rec := do(t, st, body, sign(body, fixedNow.Unix()))
	if rec.Code != 200 {
		t.Fatalf("code %d", rec.Code)
	}
	e := st.events["evt_0001"]
	if e.Type != "customer.created" || e.Created.Unix() != 1735689600 || string(e.Raw) != body {
		t.Fatalf("bad event %+v", e)
	}
}

func TestInvalidSignatures(t *testing.T) {
	st := &memStore{events: map[string]Event{}}
	old := fixedNow.Add(-time.Hour).Unix()
	cases := map[string]string{
		"missing":       "",
		"garbage":       "nope",
		"wrong secret":  "t=1735689600,v1=" + strings.Repeat("00", 32),
		"non-hex":       "t=1735689600,v1=zz",
		"tampered body": sign(body+" ", fixedNow.Unix()),
		"no v1":         "t=1735689600",
		"stale replay":  sign(body, old),
		"future":        sign(body, fixedNow.Add(time.Hour).Unix()),
	}
	for name, sig := range cases {
		if rec := do(t, st, body, sig); rec.Code != 400 {
			t.Errorf("%s: code %d", name, rec.Code)
		}
	}
	if len(st.events) != 0 {
		t.Fatal("invalid request stored")
	}
}

func TestMalformedEvent(t *testing.T) {
	b := `{"id":"x"}`
	if rec := do(t, &memStore{events: map[string]Event{}}, b, sign(b, fixedNow.Unix())); rec.Code != 400 {
		t.Fatalf("code %d", rec.Code)
	}
}

func TestStoreFailureNot2xx(t *testing.T) {
	st := &memStore{err: fmt.Errorf("db down")}
	if rec := do(t, st, body, sign(body, fixedNow.Unix())); rec.Code != 500 {
		t.Fatalf("code %d", rec.Code)
	}
}

func TestDuplicateDelivery(t *testing.T) {
	st := &memStore{events: map[string]Event{}}
	for i := 0; i < 2; i++ {
		if rec := do(t, st, body, sign(body, fixedNow.Unix())); rec.Code != 200 {
			t.Fatalf("code %d", rec.Code)
		}
	}
	if len(st.events) != 1 {
		t.Fatal("duplicate stored")
	}
}

func TestHealthz(t *testing.T) {
	s := &Server{}
	rec := httptest.NewRecorder()
	s.Routes().ServeHTTP(rec, httptest.NewRequest("GET", "/healthz", nil))
	if rec.Code != 200 {
		t.Fatal(rec.Code)
	}
}

// Runs only when MIRROR_TEST_DATABASE_URL points at a Postgres instance.
func TestPGStore(t *testing.T) {
	url := os.Getenv("MIRROR_TEST_DATABASE_URL")
	if url == "" {
		t.Skip("MIRROR_TEST_DATABASE_URL not set")
	}
	ctx := context.Background()
	ps, err := NewPGStore(ctx, url)
	if err != nil {
		t.Fatal(err)
	}
	defer ps.pool.Close()
	id := fmt.Sprintf("evt_test_%d", time.Now().UnixNano())
	b := fmt.Sprintf(`{"id":%q,"type":"customer.created","created":1735689600}`, id)
	req := httptest.NewRequest("POST", "/webhooks", nil)
	ev := Event{ID: id, Type: "customer.created", Created: time.Unix(1735689600, 0).UTC(), Raw: []byte(b)}
	for i := 0; i < 2; i++ {
		if err := ps.Save(req, ev); err != nil {
			t.Fatal(err)
		}
	}
	var n int
	var typ, payload string
	err = ps.pool.QueryRow(ctx, `SELECT count(*), max(type), max(payload::text) FROM events WHERE id=$1`, id).Scan(&n, &typ, &payload)
	if err != nil || n != 1 || typ != "customer.created" || !strings.Contains(payload, id) {
		t.Fatalf("n=%d typ=%s err=%v", n, typ, err)
	}
	ps.pool.Exec(ctx, `DELETE FROM events WHERE id=$1`, id)
}
