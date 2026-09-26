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

const testSecret = "whsec_test"

var fixedNow = time.Unix(1735689600, 0)

const body = `{"id":"evt_0001","object":"event","type":"customer.created","created":1735689600,"data":{"object":{"id":"cus_1"}}}`

func sign(secret string, t int64, b string) string {
	m := hmac.New(sha256.New, []byte(secret))
	fmt.Fprintf(m, "%d.%s", t, b)
	return fmt.Sprintf("t=%d,v1=%s", t, hex.EncodeToString(m.Sum(nil)))
}

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

func post(t *testing.T, st Store, b, sig string) *httptest.ResponseRecorder {
	t.Helper()
	s := &Server{Secret: testSecret, Store: st, Now: func() time.Time { return fixedNow }}
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
	rec := post(t, st, body, sign(testSecret, fixedNow.Unix(), body))
	if rec.Code != 200 {
		t.Fatalf("code %d", rec.Code)
	}
	e, ok := st.events["evt_0001"]
	if !ok || e.Type != "customer.created" || !e.Created.Equal(time.Unix(1735689600, 0)) || string(e.Payload) != body {
		t.Fatalf("bad stored event: %+v", e)
	}
}

func TestDuplicateDeliveryAcknowledged(t *testing.T) {
	st := &memStore{events: map[string]Event{}}
	sig := sign(testSecret, fixedNow.Unix(), body)
	for i := 0; i < 2; i++ {
		if rec := post(t, st, body, sig); rec.Code != 200 {
			t.Fatalf("delivery %d: code %d", i, rec.Code)
		}
	}
	if len(st.events) != 1 {
		t.Fatalf("want 1 event, got %d", len(st.events))
	}
}

func TestRejections(t *testing.T) {
	now := fixedNow.Unix()
	cases := map[string]string{
		"missing":       "",
		"garbage":       "nonsense",
		"wrong secret":  sign("other", now, body),
		"tampered body": sign(testSecret, now, body+" "),
		"stale":         sign(testSecret, now-3600, body),
		"future":        sign(testSecret, now+3600, body),
		"no v1":         fmt.Sprintf("t=%d", now),
		"bad hex":       fmt.Sprintf("t=%d,v1=zz", now),
		"t swapped":     strings.Replace(sign(testSecret, now, body), fmt.Sprint(now), fmt.Sprint(now-1), 1),
	}
	for name, sig := range cases {
		t.Run(name, func(t *testing.T) {
			st := &memStore{events: map[string]Event{}}
			if rec := post(t, st, body, sig); rec.Code != 401 {
				t.Fatalf("code %d, want 401", rec.Code)
			}
			if len(st.events) != 0 {
				t.Fatal("event stored despite bad signature")
			}
		})
	}
}

func TestInvalidPayloadsSigned(t *testing.T) {
	for name, b := range map[string]string{
		"not json":   "hello",
		"no id":      `{"type":"x","created":1}`,
		"no type":    `{"id":"e","created":1}`,
		"no created": `{"id":"e","type":"x"}`,
	} {
		t.Run(name, func(t *testing.T) {
			st := &memStore{events: map[string]Event{}}
			if rec := post(t, st, b, sign(testSecret, fixedNow.Unix(), b)); rec.Code != 400 {
				t.Fatalf("code %d, want 400", rec.Code)
			}
		})
	}
}

func TestStoreFailureIsNot2xx(t *testing.T) {
	st := &memStore{events: map[string]Event{}, err: errors.New("db down")}
	if rec := post(t, st, body, sign(testSecret, fixedNow.Unix(), body)); rec.Code != 500 {
		t.Fatalf("code %d, want 500", rec.Code)
	}
}

func TestBodyTooLarge(t *testing.T) {
	b := strings.Repeat("a", maxBodyBytes+1)
	st := &memStore{events: map[string]Event{}}
	if rec := post(t, st, b, sign(testSecret, fixedNow.Unix(), b)); rec.Code != 413 {
		t.Fatalf("code %d, want 413", rec.Code)
	}
}

func TestHealthz(t *testing.T) {
	s := &Server{Secret: testSecret, Store: &memStore{}, Now: time.Now}
	rec := httptest.NewRecorder()
	s.Routes().ServeHTTP(rec, httptest.NewRequest("GET", "/healthz", nil))
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
	st, err := NewPGStore(ctx, url)
	if err != nil {
		t.Fatal(err)
	}
	defer st.Close()
	id := fmt.Sprintf("evt_test_%d", time.Now().UnixNano())
	defer st.pool.Exec(ctx, `DELETE FROM events WHERE id=$1`, id)

	e := Event{ID: id, Type: "customer.created", Created: time.Unix(1735689600, 0).UTC(), Payload: []byte(body)}
	if err := st.Save(ctx, e); err != nil {
		t.Fatal(err)
	}
	e2 := e
	e2.Type = "changed"
	if err := st.Save(ctx, e2); err != nil {
		t.Fatalf("duplicate save: %v", err)
	}
	var typ string
	var created time.Time
	var payload string
	err = st.pool.QueryRow(ctx, `SELECT type, created_at, payload::text FROM events WHERE id=$1`, id).Scan(&typ, &created, &payload)
	if err != nil {
		t.Fatal(err)
	}
	if typ != "customer.created" || !created.Equal(e.Created) || !strings.Contains(payload, `"cus_1"`) {
		t.Fatalf("got %s %v %s", typ, created, payload)
	}
}
