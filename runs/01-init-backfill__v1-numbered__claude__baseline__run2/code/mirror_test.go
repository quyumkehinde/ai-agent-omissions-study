package main

import (
	"bytes"
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"strconv"
	"strings"
	"testing"
	"time"
)

func TestLoadConfigMissing(t *testing.T) {
	cases := []struct {
		env  map[string]string
		want []string
	}{
		{map[string]string{}, []string{"MIRROR_DATABASE_URL", "MIRROR_API_KEY"}},
		{map[string]string{"MIRROR_API_KEY": "k"}, []string{"MIRROR_DATABASE_URL"}},
		{map[string]string{"MIRROR_DATABASE_URL": "u"}, []string{"MIRROR_API_KEY"}},
	}
	for _, c := range cases {
		_, err := loadConfig(func(k string) string { return c.env[k] })
		if err == nil {
			t.Fatalf("expected error for %v", c.env)
		}
		for _, w := range c.want {
			if !strings.Contains(err.Error(), w) {
				t.Errorf("error %q should name %s", err, w)
			}
		}
		if len(c.want) == 1 {
			other := "MIRROR_API_KEY"
			if c.want[0] == other {
				other = "MIRROR_DATABASE_URL"
			}
			if strings.Contains(err.Error(), other) {
				t.Errorf("error %q should not name %s", err, other)
			}
		}
	}
}

func TestRunMissingEnv(t *testing.T) {
	err := run(context.Background(), []string{"status"}, func(string) string { return "" }, &bytes.Buffer{})
	if err == nil || !strings.Contains(err.Error(), "MIRROR_API_KEY") {
		t.Fatalf("got %v", err)
	}
}

func TestRunUsage(t *testing.T) {
	if err := run(context.Background(), []string{"bogus"}, os.Getenv, &bytes.Buffer{}); err == nil {
		t.Fatal("expected usage error")
	}
}

// fakeAPI serves n customers and subscriptions (every 3rd canceled).
func fakeAPI(t *testing.T, nCust, nSub int, onList func(w http.ResponseWriter, r *http.Request) bool) *httptest.Server {
	t.Helper()
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Authorization") != "Bearer good" {
			w.WriteHeader(401)
			w.Write([]byte(`{"error":{"type":"auth","message":"bad key"}}`))
			return
		}
		if r.URL.Path == "/v1/account" {
			w.Write([]byte(`{"id":"acct_x","object":"account","livemode":false}`))
			return
		}
		if onList != nil && onList(w, r) {
			return
		}
		var items []any
		switch r.URL.Path {
		case "/v1/customers":
			for i := 1; i <= nCust; i++ {
				items = append(items, Customer{ID: fmt.Sprintf("cus_%03d", i), Object: "customer", Created: int64(i)})
			}
		case "/v1/subscriptions":
			all := r.URL.Query().Get("status") == "all"
			for i := 1; i <= nSub; i++ {
				st := "active"
				if i%3 == 0 {
					st = "canceled"
					if !all {
						continue
					}
				}
				items = append(items, Subscription{ID: fmt.Sprintf("sub_%03d", i), Object: "subscription", Customer: "cus_001", Status: st, Created: int64(i)})
			}
		default:
			w.WriteHeader(404)
			return
		}
		limit, _ := strconv.Atoi(r.URL.Query().Get("limit"))
		if limit == 0 {
			limit = 10
		}
		start := 0
		if a := r.URL.Query().Get("starting_after"); a != "" {
			for i, it := range items {
				b, _ := json.Marshal(it)
				var m map[string]any
				json.Unmarshal(b, &m)
				if m["id"] == a {
					start = i + 1
				}
			}
		}
		end := min(start+limit, len(items))
		json.NewEncoder(w).Encode(map[string]any{"object": "list", "has_more": end < len(items), "data": items[start:end]})
	}))
}

func TestCheckAccountRejected(t *testing.T) {
	srv := fakeAPI(t, 0, 0, nil)
	defer srv.Close()
	_, err := NewClient(srv.URL, "wrong").CheckAccount(context.Background())
	if !errors.Is(err, ErrUnauthorized) {
		t.Fatalf("got %v", err)
	}
	id, err := NewClient(srv.URL, "good").CheckAccount(context.Background())
	if err != nil || id != "acct_x" {
		t.Fatalf("got %q, %v", id, err)
	}
}

func TestPaginationAndCanceled(t *testing.T) {
	srv := fakeAPI(t, 250, 30, nil)
	defer srv.Close()
	c := NewClient(srv.URL, "good")

	var custs []Customer
	if err := c.EachCustomer(context.Background(), func(p []Customer) error { custs = append(custs, p...); return nil }); err != nil {
		t.Fatal(err)
	}
	if len(custs) != 250 || custs[249].ID != "cus_250" {
		t.Fatalf("got %d customers", len(custs))
	}

	var subs []Subscription
	if err := c.EachSubscription(context.Background(), func(p []Subscription) error { subs = append(subs, p...); return nil }); err != nil {
		t.Fatal(err)
	}
	if len(subs) != 30 {
		t.Fatalf("got %d subscriptions, want 30 (canceled must be included)", len(subs))
	}
}

func TestRetryAfter(t *testing.T) {
	calls := 0
	srv := fakeAPI(t, 3, 0, func(w http.ResponseWriter, r *http.Request) bool {
		calls++
		if calls <= 2 {
			w.Header().Set("Retry-After", "2")
			w.WriteHeader(429)
			return true
		}
		return false
	})
	defer srv.Close()
	c := NewClient(srv.URL, "good")
	var slept []time.Duration
	c.sleep = func(_ context.Context, d time.Duration) error { slept = append(slept, d); return nil }
	n := 0
	if err := c.EachCustomer(context.Background(), func(p []Customer) error { n += len(p); return nil }); err != nil {
		t.Fatal(err)
	}
	if n != 3 || len(slept) != 2 || slept[0] != 2*time.Second {
		t.Fatalf("n=%d slept=%v", n, slept)
	}
}

func TestRateLimitGivesUp(t *testing.T) {
	srv := fakeAPI(t, 3, 0, func(w http.ResponseWriter, r *http.Request) bool {
		w.WriteHeader(429)
		return true
	})
	defer srv.Close()
	c := NewClient(srv.URL, "good")
	c.maxRetries = 2
	c.sleep = func(context.Context, time.Duration) error { return nil }
	if err := c.EachCustomer(context.Background(), func([]Customer) error { return nil }); err == nil {
		t.Fatal("expected error")
	}
}

func TestInitRejectedKeyDoesNotTouchDB(t *testing.T) {
	srv := fakeAPI(t, 1, 1, nil)
	defer srv.Close()
	// Invalid DSN would fail on connect; the key check must fail first.
	db, _ := sql.Open("postgres", "postgres://nobody@127.0.0.1:1/x?sslmode=disable")
	defer db.Close()
	err := runInit(context.Background(), NewClient(srv.URL, "wrong"), db, &bytes.Buffer{})
	if !errors.Is(err, ErrUnauthorized) || !strings.Contains(err.Error(), "MIRROR_API_KEY") {
		t.Fatalf("got %v", err)
	}
}

// Database tests need a real Postgres: set MIRROR_TEST_DATABASE_URL.
func testDB(t *testing.T) *sql.DB {
	t.Helper()
	dsn := os.Getenv("MIRROR_TEST_DATABASE_URL")
	if dsn == "" {
		t.Skip("MIRROR_TEST_DATABASE_URL not set")
	}
	db, err := sql.Open("postgres", dsn)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { db.Close() })
	if _, err := db.Exec("DROP TABLE IF EXISTS customers, subscriptions"); err != nil {
		t.Fatal(err)
	}
	return db
}

func TestInitAndStatusDB(t *testing.T) {
	db := testDB(t)
	srv := fakeAPI(t, 250, 30, nil)
	defer srv.Close()
	ctx := context.Background()
	c := NewClient(srv.URL, "good")

	for i := 0; i < 2; i++ { // second run proves idempotency
		var out bytes.Buffer
		if err := runInit(ctx, c, db, &out); err != nil {
			t.Fatal(err)
		}
	}
	var out bytes.Buffer
	if err := runStatus(ctx, db, &out); err != nil {
		t.Fatal(err)
	}
	if got := out.String(); got != "customers: 250\nsubscriptions: 30\n" {
		t.Fatalf("status output %q", got)
	}
	var status string
	if err := db.QueryRow("SELECT status FROM subscriptions WHERE id='sub_003'").Scan(&status); err != nil || status != "canceled" {
		t.Fatalf("canceled sub: %q %v", status, err)
	}
}
