package main

import (
	"bytes"
	"context"
	"database/sql"
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
)

const testKey = "sk_test_x"

// fakeAPI serves nc customers and ns subscriptions (every 3rd one canceled),
// with the documented pagination, status filtering, and auth behavior.
func fakeAPI(t *testing.T, nc, ns int, rateLimitFirst int32) *httptest.Server {
	t.Helper()
	var limited atomic.Int32
	mux := http.NewServeMux()
	writeJSON := func(w http.ResponseWriter, v any) {
		json.NewEncoder(w).Encode(v)
	}
	page := func(w http.ResponseWriter, r *http.Request, items []map[string]any) {
		if limited.Add(1) <= rateLimitFirst {
			w.Header().Set("Retry-After", "1")
			w.WriteHeader(429)
			return
		}
		limit, _ := strconv.Atoi(r.URL.Query().Get("limit"))
		if limit < 1 || limit > 100 {
			limit = 10
		}
		start := 0
		if after := r.URL.Query().Get("starting_after"); after != "" {
			for i, it := range items {
				if it["id"] == after {
					start = i + 1
				}
			}
		}
		end := min(start+limit, len(items))
		writeJSON(w, map[string]any{"object": "list", "has_more": end < len(items), "data": items[start:end]})
	}
	mux.HandleFunc("/v1/account", func(w http.ResponseWriter, r *http.Request) {
		writeJSON(w, map[string]any{"id": "acct", "object": "account"})
	})
	mux.HandleFunc("/v1/customers", func(w http.ResponseWriter, r *http.Request) {
		var items []map[string]any
		for i := 0; i < nc; i++ {
			c := map[string]any{"id": fmt.Sprintf("cus_%04d", i), "object": "customer", "email": fmt.Sprintf("c%d@x.com", i), "name": fmt.Sprintf("C %d", i), "created": 1700000000 + i}
			if i == 1 {
				c["name"] = nil
			}
			items = append(items, c)
		}
		page(w, r, items)
	})
	mux.HandleFunc("/v1/subscriptions", func(w http.ResponseWriter, r *http.Request) {
		all := r.URL.Query().Get("status") == "all"
		var items []map[string]any
		for i := 0; i < ns; i++ {
			st := "active"
			if i%3 == 2 {
				st = "canceled"
				if !all {
					continue
				}
			}
			items = append(items, map[string]any{"id": fmt.Sprintf("sub_%04d", i), "object": "subscription", "customer": "cus_0000", "status": st, "created": 1700000000 + i})
		}
		page(w, r, items)
	})
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Authorization") != "Bearer "+testKey {
			w.WriteHeader(401)
			writeJSON(w, map[string]any{"error": map[string]any{"type": "auth", "message": "bad key"}})
			return
		}
		mux.ServeHTTP(w, r)
	}))
	t.Cleanup(srv.Close)
	return srv
}

func testClient(srv *httptest.Server, key string) (*Client, *[]time.Duration) {
	c := NewClient(srv.URL, key)
	var sleeps []time.Duration
	c.Sleep = func(_ context.Context, d time.Duration) error { sleeps = append(sleeps, d); return nil }
	return c, &sleeps
}

func TestCheckAccount(t *testing.T) {
	srv := fakeAPI(t, 0, 0, 0)
	c, _ := testClient(srv, testKey)
	if err := c.CheckAccount(context.Background()); err != nil {
		t.Fatal(err)
	}
	c, _ = testClient(srv, "wrong")
	if err := c.CheckAccount(context.Background()); err != ErrUnauthorized {
		t.Fatalf("got %v, want ErrUnauthorized", err)
	}
}

func TestPaginationCanceledAndRateLimit(t *testing.T) {
	srv := fakeAPI(t, 250, 90, 2)
	c, sleeps := testClient(srv, testKey)
	ctx := context.Background()

	var cust []Customer
	if err := c.EachCustomerPage(ctx, func(p []Customer) error { cust = append(cust, p...); return nil }); err != nil {
		t.Fatal(err)
	}
	if len(cust) != 250 {
		t.Fatalf("customers: got %d, want 250", len(cust))
	}
	if len(*sleeps) != 2 || (*sleeps)[0] != time.Second {
		t.Fatalf("expected 2 sleeps honoring Retry-After, got %v", *sleeps)
	}
	if cust[1].Name != nil || cust[0].Name == nil {
		t.Fatalf("null name not preserved")
	}

	var subs []Subscription
	if err := c.EachSubscriptionPage(ctx, func(p []Subscription) error { subs = append(subs, p...); return nil }); err != nil {
		t.Fatal(err)
	}
	if len(subs) != 90 {
		t.Fatalf("subscriptions: got %d, want 90 (including canceled)", len(subs))
	}
}

func TestRateLimitGivesUp(t *testing.T) {
	srv := fakeAPI(t, 5, 0, 1000)
	c, _ := testClient(srv, testKey)
	if err := c.EachCustomerPage(context.Background(), func([]Customer) error { return nil }); err == nil {
		t.Fatal("expected error")
	}
}

func TestMissingEnv(t *testing.T) {
	cases := []struct {
		args    []string
		env     map[string]string
		want    string
		notWant string
	}{
		{[]string{"init"}, map[string]string{}, "MIRROR_DATABASE_URL, MIRROR_API_KEY", ""},
		{[]string{"init"}, map[string]string{"MIRROR_DATABASE_URL": "x"}, "MIRROR_API_KEY", "MIRROR_DATABASE_URL"},
		{[]string{"init"}, map[string]string{"MIRROR_API_KEY": "x"}, "MIRROR_DATABASE_URL", "MIRROR_API_KEY"},
		{[]string{"status"}, map[string]string{}, "MIRROR_DATABASE_URL", ""},
	}
	for _, tc := range cases {
		var out, errb bytes.Buffer
		code := run(context.Background(), tc.args, func(k string) string { return tc.env[k] }, &out, &errb)
		if code != 1 || !strings.Contains(errb.String(), tc.want) || (tc.notWant != "" && strings.Contains(errb.String(), tc.notWant)) {
			t.Errorf("%v %v: code=%d stderr=%q", tc.args, tc.env, code, errb.String())
		}
	}
}

func TestUsage(t *testing.T) {
	var out, errb bytes.Buffer
	if code := run(context.Background(), []string{"bogus"}, func(string) string { return "" }, &out, &errb); code != 2 {
		t.Fatalf("code=%d", code)
	}
}

// testDB returns a connection URL scoped to a fresh schema, dropped on cleanup.
func testDB(t *testing.T) (string, *sql.DB) {
	t.Helper()
	base := os.Getenv("MIRROR_DATABASE_URL")
	if base == "" {
		t.Skip("MIRROR_DATABASE_URL not set")
	}
	admin, err := sql.Open("postgres", base)
	if err != nil {
		t.Fatal(err)
	}
	schemaName := fmt.Sprintf("mirror_test_%d", time.Now().UnixNano())
	if _, err := admin.Exec("CREATE SCHEMA " + schemaName); err != nil {
		t.Skipf("postgres unavailable: %v", err)
	}
	t.Cleanup(func() {
		admin.Exec("DROP SCHEMA " + schemaName + " CASCADE")
		admin.Close()
	})
	u, _ := url.Parse(base)
	q := u.Query()
	q.Set("search_path", schemaName)
	u.RawQuery = q.Encode()
	db, err := sql.Open("postgres", u.String())
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { db.Close() })
	return u.String(), db
}

func TestInitAndStatusEndToEnd(t *testing.T) {
	dbURL, db := testDB(t)
	srv := fakeAPI(t, 230, 120, 0)
	env := map[string]string{"MIRROR_DATABASE_URL": dbURL, "MIRROR_API_KEY": testKey, "MIRROR_API_URL": srv.URL}
	getenv := func(k string) string { return env[k] }
	ctx := context.Background()

	var out, errb bytes.Buffer
	if code := run(ctx, []string{"status"}, getenv, &out, &errb); code != 1 || !strings.Contains(errb.String(), "mirror init") {
		t.Fatalf("status before init: code=%d stderr=%q", code, errb.String())
	}

	// Run twice: re-running must be idempotent.
	for i := 0; i < 2; i++ {
		out.Reset()
		errb.Reset()
		if code := run(ctx, []string{"init"}, getenv, &out, &errb); code != 0 {
			t.Fatalf("init: code=%d stderr=%q", code, errb.String())
		}
	}

	out.Reset()
	if code := run(ctx, []string{"status"}, getenv, &out, &errb); code != 0 {
		t.Fatalf("status: %s", errb.String())
	}
	if want := "customers: 230\nsubscriptions: 120\n"; out.String() != want {
		t.Fatalf("status output %q, want %q", out.String(), want)
	}

	var canceled int
	db.QueryRow("SELECT count(*) FROM subscriptions WHERE status='canceled'").Scan(&canceled)
	if canceled != 40 {
		t.Fatalf("canceled subscriptions: got %d, want 40", canceled)
	}
	var name sql.NullString
	var email string
	db.QueryRow("SELECT email, name FROM customers WHERE id='cus_0001'").Scan(&email, &name)
	if email != "c1@x.com" || name.Valid {
		t.Fatalf("customer row wrong: %q %v", email, name)
	}
}

func TestInitRejectedKey(t *testing.T) {
	dbURL, db := testDB(t)
	srv := fakeAPI(t, 3, 3, 0)
	env := map[string]string{"MIRROR_DATABASE_URL": dbURL, "MIRROR_API_KEY": "nope", "MIRROR_API_URL": srv.URL}
	var out, errb bytes.Buffer
	code := run(context.Background(), []string{"init"}, func(k string) string { return env[k] }, &out, &errb)
	if code != 1 || !strings.Contains(errb.String(), "rejected") {
		t.Fatalf("code=%d stderr=%q", code, errb.String())
	}
	// Nothing should have been created.
	if _, err := CountRows(context.Background(), db, "customers"); err == nil {
		t.Fatal("tables created despite rejected key")
	}
}
