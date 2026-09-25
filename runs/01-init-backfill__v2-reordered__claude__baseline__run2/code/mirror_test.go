package main

import (
	"bytes"
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"
	"time"
)

// fakeAPI serves paginated customers and subscriptions; canceled subs only with status=all.
func fakeAPI(t *testing.T, nCust, nSub int, throttleFirst bool) *httptest.Server {
	t.Helper()
	throttled := !throttleFirst
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Authorization") != "Bearer good" {
			w.WriteHeader(401)
			w.Write([]byte(`{"error":{"type":"authentication_error","message":"bad key"}}`))
			return
		}
		if r.URL.Path == "/v1/account" {
			w.Write([]byte(`{"id":"acct_x","object":"account"}`))
			return
		}
		if !throttled && r.URL.Path == "/v1/customers" {
			throttled = true
			w.Header().Set("Retry-After", "1")
			w.WriteHeader(429)
			return
		}
		var ids []map[string]any
		switch r.URL.Path {
		case "/v1/customers":
			for i := 0; i < nCust; i++ {
				ids = append(ids, map[string]any{"id": fmt.Sprintf("cus_%03d", i), "object": "customer", "email": nil, "name": "n", "created": 1})
			}
		case "/v1/subscriptions":
			for i := 0; i < nSub; i++ {
				st := "active"
				if i%2 == 1 {
					st = "canceled"
				}
				if st == "canceled" && r.URL.Query().Get("status") != "all" {
					continue
				}
				ids = append(ids, map[string]any{"id": fmt.Sprintf("sub_%03d", i), "object": "subscription", "customer": "cus_000", "status": st, "created": 2})
			}
		}
		if a := r.URL.Query().Get("starting_after"); a != "" {
			for i, m := range ids {
				if m["id"] == a {
					ids = ids[i+1:]
					break
				}
			}
		}
		limit := 10
		fmt.Sscan(r.URL.Query().Get("limit"), &limit)
		more := len(ids) > limit
		if more {
			ids = ids[:limit]
		}
		json.NewEncoder(w).Encode(map[string]any{"object": "list", "has_more": more, "data": ids})
	}))
}

func newTestClient(url, key string) (*Client, *[]time.Duration) {
	c := NewClient(url, key)
	var sleeps []time.Duration
	c.sleep = func(d time.Duration) { sleeps = append(sleeps, d) }
	return c, &sleeps
}

func TestCheckAccount(t *testing.T) {
	srv := fakeAPI(t, 0, 0, false)
	defer srv.Close()
	c, _ := newTestClient(srv.URL, "good")
	if err := c.CheckAccount(context.Background()); err != nil {
		t.Fatal(err)
	}
	c, _ = newTestClient(srv.URL, "bad")
	if err := c.CheckAccount(context.Background()); err != ErrUnauthorized {
		t.Fatalf("got %v", err)
	}
}

func TestPaginationAndRetry(t *testing.T) {
	srv := fakeAPI(t, 250, 0, true)
	defer srv.Close()
	c, sleeps := newTestClient(srv.URL, "good")
	n := 0
	err := c.EachCustomerPage(context.Background(), func(p []Customer) error { n += len(p); return nil })
	if err != nil || n != 250 {
		t.Fatalf("n=%d err=%v", n, err)
	}
	if len(*sleeps) != 1 || (*sleeps)[0] != time.Second {
		t.Fatalf("sleeps=%v", *sleeps)
	}
}

func TestSubscriptionsIncludeCanceled(t *testing.T) {
	srv := fakeAPI(t, 0, 30, false)
	defer srv.Close()
	c, _ := newTestClient(srv.URL, "good")
	n, canceled := 0, 0
	c.EachSubscriptionPage(context.Background(), func(p []Subscription) error {
		for _, s := range p {
			n++
			if *s.Status == "canceled" {
				canceled++
			}
		}
		return nil
	})
	if n != 30 || canceled != 15 {
		t.Fatalf("n=%d canceled=%d", n, canceled)
	}
}

func env(m map[string]string) func(string) string { return func(k string) string { return m[k] } }

func TestMissingEnv(t *testing.T) {
	var out, errb bytes.Buffer
	if code := run(context.Background(), []string{"status"}, env(map[string]string{"MIRROR_API_KEY": "k"}), &out, &errb); code != 1 ||
		!strings.Contains(errb.String(), "MIRROR_DATABASE_URL") || strings.Contains(errb.String(), "MIRROR_API_KEY") {
		t.Fatalf("code=%d err=%q", code, errb.String())
	}
	errb.Reset()
	run(context.Background(), []string{"init"}, env(map[string]string{"MIRROR_DATABASE_URL": "x"}), &out, &errb)
	if !strings.Contains(errb.String(), "MIRROR_API_KEY") {
		t.Fatalf("err=%q", errb.String())
	}
}

func TestInitRejectedKey(t *testing.T) {
	srv := fakeAPI(t, 0, 0, false)
	defer srv.Close()
	var out, errb bytes.Buffer
	code := run(context.Background(), []string{"init"}, env(map[string]string{
		"MIRROR_DATABASE_URL": "postgres://unused", "MIRROR_API_KEY": "bad", "MIRROR_API_URL": srv.URL}), &out, &errb)
	if code != 1 || !strings.Contains(errb.String(), "rejected") {
		t.Fatalf("code=%d err=%q", code, errb.String())
	}
}

// TestEndToEnd needs a real Postgres: set MIRROR_TEST_DATABASE_URL (its mirror tables are dropped).
func TestEndToEnd(t *testing.T) {
	dbURL := os.Getenv("MIRROR_TEST_DATABASE_URL")
	if dbURL == "" {
		t.Skip("MIRROR_TEST_DATABASE_URL not set")
	}
	db, err := sql.Open("pgx", dbURL)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	db.Exec("DROP TABLE IF EXISTS customers, subscriptions")

	srv := fakeAPI(t, 250, 30, false)
	defer srv.Close()
	e := env(map[string]string{"MIRROR_DATABASE_URL": dbURL, "MIRROR_API_KEY": "good", "MIRROR_API_URL": srv.URL})
	for i := 0; i < 2; i++ { // second run proves idempotence
		var out, errb bytes.Buffer
		if code := run(context.Background(), []string{"init"}, e, &out, &errb); code != 0 {
			t.Fatalf("init: %s", errb.String())
		}
	}
	var out, errb bytes.Buffer
	if code := run(context.Background(), []string{"status"}, e, &out, &errb); code != 0 {
		t.Fatal(errb.String())
	}
	if out.String() != "customers: 250\nsubscriptions: 30\n" {
		t.Fatalf("status=%q", out.String())
	}
}
