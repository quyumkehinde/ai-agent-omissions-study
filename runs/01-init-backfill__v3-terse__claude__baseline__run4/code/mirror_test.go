package main

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"strconv"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"
)

const testKey = "sk_test"

// fakeAPI serves n customers and subs subscriptions (every 4th canceled), honoring
// limit, starting_after and status, and 429s the first list request.
func fakeAPI(t *testing.T, n, subs int) (*httptest.Server, *atomic.Int32) {
	var throttled atomic.Int32
	mux := http.NewServeMux()
	auth := func(h http.HandlerFunc) http.HandlerFunc {
		return func(w http.ResponseWriter, r *http.Request) {
			if r.Header.Get("Authorization") != "Bearer "+testKey {
				w.WriteHeader(401)
				w.Write([]byte(`{"error":{"type":"auth","message":"bad key"}}`))
				return
			}
			h(w, r)
		}
	}
	list := func(total int, item func(i int) map[string]any) http.HandlerFunc {
		return func(w http.ResponseWriter, r *http.Request) {
			if throttled.Add(1) == 1 {
				w.Header().Set("Retry-After", "1")
				w.WriteHeader(429)
				return
			}
			q := r.URL.Query()
			limit, _ := strconv.Atoi(q.Get("limit"))
			if limit == 0 {
				limit = 10
			}
			var all []map[string]any
			for i := 1; i <= total; i++ {
				it := item(i)
				if it["status"] == "canceled" && q.Get("status") != "all" {
					continue
				}
				all = append(all, it)
			}
			start := 0
			if a := q.Get("starting_after"); a != "" {
				for i, it := range all {
					if it["id"] == a {
						start = i + 1
					}
				}
			}
			end := min(start+limit, len(all))
			json.NewEncoder(w).Encode(map[string]any{"object": "list", "has_more": end < len(all), "data": all[start:end]})
		}
	}
	mux.HandleFunc("/v1/account", auth(func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte(`{"id":"acct","object":"account"}`))
	}))
	mux.HandleFunc("/v1/customers", auth(list(n, func(i int) map[string]any {
		return map[string]any{"id": fmt.Sprintf("cus_%04d", i), "object": "customer",
			"email": fmt.Sprintf("u%d@example.com", i), "name": nil, "created": 1735689600 + i}
	})))
	mux.HandleFunc("/v1/subscriptions", auth(list(subs, func(i int) map[string]any {
		st := "active"
		if i%4 == 0 {
			st = "canceled"
		}
		return map[string]any{"id": fmt.Sprintf("sub_%04d", i), "object": "subscription",
			"customer": "cus_0001", "status": st, "created": 1735689600 + i}
	})))
	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)
	return srv, &throttled
}

func testClient(url, key string) (*Client, *[]time.Duration) {
	var sleeps []time.Duration
	c := NewClient(url, key)
	c.Sleep = func(_ context.Context, d time.Duration) error { sleeps = append(sleeps, d); return nil }
	return c, &sleeps
}

func TestLoadConfigMissing(t *testing.T) {
	env := func(m map[string]string) func(string) string { return func(k string) string { return m[k] } }
	cases := []struct {
		env     map[string]string
		need    bool
		want    []string
		wantNot []string
	}{
		{map[string]string{"MIRROR_API_KEY": "k"}, true, []string{"MIRROR_DATABASE_URL"}, []string{"MIRROR_API_KEY"}},
		{map[string]string{"MIRROR_DATABASE_URL": "d"}, true, []string{"MIRROR_API_KEY"}, []string{"MIRROR_DATABASE_URL"}},
		{map[string]string{}, true, []string{"MIRROR_API_KEY", "MIRROR_DATABASE_URL"}, nil},
		{map[string]string{"MIRROR_DATABASE_URL": "d"}, false, nil, nil}, // status needs no key
	}
	for i, c := range cases {
		_, err := loadConfig(env(c.env), c.need)
		if len(c.want) == 0 {
			if err != nil {
				t.Errorf("case %d: unexpected error %v", i, err)
			}
			continue
		}
		if err == nil {
			t.Fatalf("case %d: expected error", i)
		}
		for _, w := range c.want {
			if !strings.Contains(err.Error(), w) {
				t.Errorf("case %d: %q missing %s", i, err, w)
			}
		}
		for _, w := range c.wantNot {
			if strings.Contains(err.Error(), w) {
				t.Errorf("case %d: %q should not mention %s", i, err, w)
			}
		}
	}
}

func TestCheckAccountRejectsBadKey(t *testing.T) {
	srv, _ := fakeAPI(t, 0, 0)
	c, _ := testClient(srv.URL, "wrong")
	if err := c.CheckAccount(context.Background()); err != errUnauthorized {
		t.Fatalf("got %v, want errUnauthorized", err)
	}
	c, _ = testClient(srv.URL, testKey)
	if err := c.CheckAccount(context.Background()); err != nil {
		t.Fatal(err)
	}
}

func TestPaginationRetryAndCanceled(t *testing.T) {
	srv, _ := fakeAPI(t, 250, 45)
	c, sleeps := testClient(srv.URL, testKey)
	var custs []Customer
	if err := c.ListCustomers(context.Background(), func(p []Customer) error { custs = append(custs, p...); return nil }); err != nil {
		t.Fatal(err)
	}
	if len(custs) != 250 || custs[249].ID != "cus_0250" {
		t.Fatalf("got %d customers", len(custs))
	}
	if len(*sleeps) != 1 || (*sleeps)[0] != time.Second {
		t.Fatalf("expected one 1s Retry-After sleep, got %v", *sleeps)
	}
	var subs []Subscription
	if err := c.ListSubscriptions(context.Background(), func(p []Subscription) error { subs = append(subs, p...); return nil }); err != nil {
		t.Fatal(err)
	}
	if len(subs) != 45 {
		t.Fatalf("got %d subscriptions, want 45 including canceled", len(subs))
	}
}

func TestRetryGivesUp(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Retry-After", "0")
		w.WriteHeader(429)
	}))
	defer srv.Close()
	c, _ := testClient(srv.URL, testKey)
	if err := c.ListCustomers(context.Background(), func([]Customer) error { return nil }); err == nil {
		t.Fatal("expected error")
	}
}

// testDB returns a connection URL scoped to a fresh schema. Requires MIRROR_TEST_DATABASE_URL.
func testDB(t *testing.T) string {
	base := os.Getenv("MIRROR_TEST_DATABASE_URL")
	if base == "" {
		t.Skip("MIRROR_TEST_DATABASE_URL not set")
	}
	ctx := context.Background()
	schema := fmt.Sprintf("mirror_test_%d", time.Now().UnixNano())
	admin, err := pgx.Connect(ctx, base)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := admin.Exec(ctx, "CREATE SCHEMA "+schema); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		admin.Exec(ctx, "DROP SCHEMA "+schema+" CASCADE")
		admin.Close(ctx)
	})
	u, _ := url.Parse(base)
	q := u.Query()
	q.Set("search_path", schema)
	u.RawQuery = q.Encode()
	return u.String()
}

func TestInitAndStatusEndToEnd(t *testing.T) {
	dbURL := testDB(t)
	srv, _ := fakeAPI(t, 120, 30)
	env := map[string]string{"MIRROR_DATABASE_URL": dbURL, "MIRROR_API_KEY": testKey, "MIRROR_API_URL": srv.URL}
	getenv := func(k string) string { return env[k] }
	ctx := context.Background()

	var out bytes.Buffer
	if err := run(ctx, []string{"init"}, getenv, &out); err != nil {
		t.Fatal(err)
	}
	// Re-running must be idempotent.
	if err := run(ctx, []string{"init"}, getenv, &out); err != nil {
		t.Fatal(err)
	}
	out.Reset()
	if err := run(ctx, []string{"status"}, getenv, &out); err != nil {
		t.Fatal(err)
	}
	if got, want := out.String(), "customers: 120\nsubscriptions: 30\n"; got != want {
		t.Fatalf("status = %q, want %q", got, want)
	}

	conn, _ := pgx.Connect(ctx, dbURL)
	defer conn.Close(ctx)
	var n int
	conn.QueryRow(ctx, "SELECT count(*) FROM subscriptions WHERE status='canceled'").Scan(&n)
	if n != 7 {
		t.Errorf("canceled subs = %d, want 7", n)
	}
	var email *string
	var name *string
	var created time.Time
	if err := conn.QueryRow(ctx, "SELECT email, name, created FROM customers WHERE id='cus_0001'").Scan(&email, &name, &created); err != nil {
		t.Fatal(err)
	}
	if email == nil || *email != "u1@example.com" || name != nil || created.Unix() != 1735689601 {
		t.Errorf("unexpected row: %v %v %v", email, name, created)
	}
}

func TestInitBadKeyDoesNotTouchDB(t *testing.T) {
	srv, _ := fakeAPI(t, 1, 1)
	env := map[string]string{"MIRROR_DATABASE_URL": "postgres://invalid.invalid/x", "MIRROR_API_KEY": "nope", "MIRROR_API_URL": srv.URL}
	err := run(context.Background(), []string{"init"}, func(k string) string { return env[k] }, &bytes.Buffer{})
	if err == nil || !strings.Contains(err.Error(), "MIRROR_API_KEY") || !strings.Contains(err.Error(), "rejected") {
		t.Fatalf("got %v", err)
	}
}
