package main

import (
	"bytes"
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"strconv"
	"testing"
	"time"
)

const secret = "whsec_test"

type memStore struct {
	events map[string]Event
	err    error
}

func (m *memStore) Save(_ context.Context, e Event) error {
	if m.err != nil {
		return m.err
	}
	if _, ok := m.events[e.ID]; !ok {
		m.events[e.ID] = e
	}
	return nil
}

var fixedNow = time.Unix(1735689600, 0)

func sign(sec string, ts int64, body []byte) string {
	mac := hmac.New(sha256.New, []byte(sec))
	fmt.Fprintf(mac, "%d.", ts)
	mac.Write(body)
	return fmt.Sprintf("t=%d,v1=%s", ts, hex.EncodeToString(mac.Sum(nil)))
}

const body = `{"id":"evt_0001","object":"event","type":"customer.created","created":1735689600,"data":{"object":{"id":"cus_1"}}}`

func do(t *testing.T, st Store, b, sig string) *httptest.ResponseRecorder {
	t.Helper()
	s := &Server{Secret: secret, Store: st, Now: func() time.Time { return fixedNow }}
	req := httptest.NewRequest("POST", "/webhooks", bytes.NewBufferString(b))
	if sig != "" {
		req.Header.Set("Signature", sig)
	}
	rr := httptest.NewRecorder()
	s.Routes().ServeHTTP(rr, req)
	return rr
}

func TestValidStored(t *testing.T) {
	st := &memStore{events: map[string]Event{}}
	rr := do(t, st, body, sign(secret, fixedNow.Unix(), []byte(body)))
	if rr.Code != 200 {
		t.Fatalf("code %d", rr.Code)
	}
	e := st.events["evt_0001"]
	if e.Type != "customer.created" || e.Created.Unix() != 1735689600 || string(e.Payload) != body {
		t.Fatalf("bad event %+v", e)
	}
}

func TestRejected(t *testing.T) {
	b := []byte(body)
	cases := map[string]string{
		"missing":      "",
		"garbage":      "hello",
		"wrong secret": sign("other", fixedNow.Unix(), b),
		"tampered":     sign(secret, fixedNow.Unix(), []byte(body+" ")),
		"old replay":   sign(secret, fixedNow.Add(-time.Hour).Unix(), b),
		"future":       sign(secret, fixedNow.Add(time.Hour).Unix(), b),
		"bad hex":      "t=" + strconv.FormatInt(fixedNow.Unix(), 10) + ",v1=zz",
		"no timestamp": "v1=abcd",
	}
	for name, sig := range cases {
		t.Run(name, func(t *testing.T) {
			st := &memStore{events: map[string]Event{}}
			if rr := do(t, st, body, sig); rr.Code != 400 {
				t.Fatalf("code %d", rr.Code)
			}
			if len(st.events) != 0 {
				t.Fatal("stored despite rejection")
			}
		})
	}
}

func TestStoreFailureNot2xx(t *testing.T) {
	st := &memStore{err: errors.New("db down")}
	rr := do(t, st, body, sign(secret, fixedNow.Unix(), []byte(body)))
	if rr.Code/100 == 2 {
		t.Fatalf("code %d", rr.Code)
	}
}

func TestDuplicateDelivery(t *testing.T) {
	st := &memStore{events: map[string]Event{}}
	sig := sign(secret, fixedNow.Unix(), []byte(body))
	for i := 0; i < 2; i++ {
		if rr := do(t, st, body, sig); rr.Code != 200 {
			t.Fatalf("code %d", rr.Code)
		}
	}
	if len(st.events) != 1 {
		t.Fatal("duplicate stored")
	}
}

func TestInvalidJSONSigned(t *testing.T) {
	b := `{"nope`
	st := &memStore{events: map[string]Event{}}
	if rr := do(t, st, b, sign(secret, fixedNow.Unix(), []byte(b))); rr.Code != 400 {
		t.Fatalf("code %d", rr.Code)
	}
}

func TestHealthz(t *testing.T) {
	s := &Server{Secret: secret, Store: &memStore{}}
	rr := httptest.NewRecorder()
	s.Routes().ServeHTTP(rr, httptest.NewRequest("GET", "/healthz", nil))
	if rr.Code != http.StatusOK {
		t.Fatalf("code %d", rr.Code)
	}
}

// Runs only when MIRROR_TEST_DATABASE_URL points at a scratch Postgres.
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
		if err := p.Save(ctx, e); err != nil {
			t.Fatal(err)
		}
	}
	var n int
	var typ string
	if err := p.pool.QueryRow(ctx, "SELECT count(*), max(type) FROM events WHERE id=$1", id).Scan(&n, &typ); err != nil {
		t.Fatal(err)
	}
	if n != 1 || typ != e.Type {
		t.Fatalf("n=%d typ=%s", n, typ)
	}
}
