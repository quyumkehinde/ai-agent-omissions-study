package main

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"strconv"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"
)

const testKey = "sk_test_key"

// fakeAPI serves nCust customers and subscriptions (every 4th canceled), paginated like the real API.
func fakeAPI(t *testing.T, nCust, nSub int, rateLimitFirst int32) *httptest.Server {
	t.Helper()
	var limited atomic.Int32
	mux := http.NewServeMux()
	writeJSON := func(w http.ResponseWriter, v any) {
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(v)
	}
	page := func(w http.ResponseWriter, r *http.Request, ids []string, item func(i int) any) {
		if limited.Add(1) <= rateLimitFirst {
			w.Header().Set("Retry-After", "2")
			w.WriteHeader(429)
			return
		}
		limit := 10
		if s := r.URL.Query().Get("limit"); s != "" {
			limit, _ = strconv.Atoi(s)
		}
		start := 0
		if a := r.URL.Query().Get("starting_after"); a != "" {
			for i, id := range ids {
				if id == a {
					start = i + 1
				}
			}
		}
		end := min(start+limit, len(ids))
		data := []any{}
		for i := start; i < end; i++ {
			data = append(data, item(i))
		}
		writeJSON(w, map[string]any{"object": "list", "has_more": end < len(ids), "data": data})
	}
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
	mux.HandleFunc("/v1/account", auth(func(w http.ResponseWriter, r *http.Request) {
		writeJSON(w, Account{ID: "acct_1", Object: "account"})
	}))
	var cids, sids []string
	for i := 0; i < nCust; i++ {
		cids = append(cids, fmt.Sprintf("cus_%04d", i))
	}
	for i := 0; i < nSub; i++ {
		sids = append(sids, fmt.Sprintf("sub_%04d", i))
	}
	mux.HandleFunc("/v1/customers", auth(func(w http.ResponseWriter, r *http.Request) {
		page(w, r, cids, func(i int) any {
			c := Customer{ID: cids[i], Object: "customer", Created: 1735689600 + int64(i)}
			if i%5 != 0 { // every 5th has null email/name
				e, n := fmt.Sprintf("u%d@example.com", i), fmt.Sprintf("C%d", i)
				c.Email, c.Name = &e, &n
			}
			return c
		})
	}))
	mux.HandleFunc("/v1/subscriptions", auth(func(w http.ResponseWriter, r *http.Request) {
		all := r.URL.Query().Get("status") == "all"
		var ids []string
		for i, id := range sids {
			if all || i%4 != 3 {
				ids = append(ids, id)
			}
		}
		page(w, r, ids, func(i int) any {
			n, _ := strconv.Atoi(strings.TrimPrefix(ids[i], "sub_"))
			st := "active"
			if n%4 == 3 {
				st = "canceled"
			}
			return Subscription{ID: ids[i], Object: "subscription", Customer: cids[n%nCust], Status: st, Created: 1735689600 + int64(n)}
		})
	}))
	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)
	return srv
}

func testConn(t *testing.T) *pgx.Conn {
	t.Helper()
	url := os.Getenv("MIRROR_DATABASE_URL")
	if url == "" {
		t.Skip("MIRROR_DATABASE_URL not set")
	}
	ctx := context.Background()
	conn, err := pgx.Connect(ctx, url)
	if err != nil {
		t.Fatal(err)
	}
	// Isolate in a throwaway schema so real data is untouched.
	name := fmt.Sprintf("mirror_test_%d", time.Now().UnixNano())
	if _, err := conn.Exec(ctx, "CREATE SCHEMA "+name); err != nil {
		t.Fatal(err)
	}
	if _, err := conn.Exec(ctx, "SET search_path TO "+name); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		conn.Exec(ctx, "DROP SCHEMA "+name+" CASCADE")
		conn.Close(ctx)
	})
	return conn
}

func TestClientAccount(t *testing.T) {
	srv := fakeAPI(t, 1, 1, 0)
	if _, err := NewClient(srv.URL, testKey).Account(context.Background()); err != nil {
		t.Fatal(err)
	}
	_, err := NewClient(srv.URL, "wrong").Account(context.Background())
	if err != ErrUnauthorized {
		t.Fatalf("got %v, want ErrUnauthorized", err)
	}
}

func TestPaginationAndCanceled(t *testing.T) {
	srv := fakeAPI(t, 250, 43, 0)
	c := NewClient(srv.URL, testKey)
	var nc, ns, canceled int
	if err := c.EachCustomerPage(context.Background(), func(p []Customer) error { nc += len(p); return nil }); err != nil {
		t.Fatal(err)
	}
	err := c.EachSubscriptionPage(context.Background(), func(p []Subscription) error {
		ns += len(p)
		for _, s := range p {
			if s.Status == "canceled" {
				canceled++
			}
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	if nc != 250 || ns != 43 || canceled != 10 {
		t.Fatalf("customers=%d subs=%d canceled=%d", nc, ns, canceled)
	}
}

func TestRateLimitRetry(t *testing.T) {
	srv := fakeAPI(t, 3, 1, 2)
	c := NewClient(srv.URL, testKey)
	var slept []time.Duration
	c.Sleep = func(_ context.Context, d time.Duration) error { slept = append(slept, d); return nil }
	n := 0
	if err := c.EachCustomerPage(context.Background(), func(p []Customer) error { n += len(p); return nil }); err != nil {
		t.Fatal(err)
	}
	if n != 3 || len(slept) != 2 || slept[0] != 2*time.Second {
		t.Fatalf("n=%d slept=%v", n, slept)
	}
}

func TestRateLimitGivesUp(t *testing.T) {
	srv := fakeAPI(t, 3, 1, 1000)
	c := NewClient(srv.URL, testKey)
	c.Sleep = func(context.Context, time.Duration) error { return nil }
	err := c.EachCustomerPage(context.Background(), func([]Customer) error { return nil })
	if err == nil || !strings.Contains(err.Error(), "rate limited") {
		t.Fatalf("got %v", err)
	}
}

func TestBackfillAndStatus(t *testing.T) {
	conn := testConn(t)
	ctx := context.Background()
	srv := fakeAPI(t, 120, 30, 0)
	c := NewClient(srv.URL, testKey)
	if err := CreateSchema(ctx, conn); err != nil {
		t.Fatal(err)
	}
	for i := 0; i < 2; i++ { // second run proves idempotence
		res, err := Backfill(ctx, c, conn)
		if err != nil {
			t.Fatal(err)
		}
		if res.Customers != 120 || res.Subscriptions != 30 {
			t.Fatalf("res=%+v", res)
		}
	}
	if n, _ := CountRows(ctx, conn, "customers"); n != 120 {
		t.Fatalf("customers=%d", n)
	}
	if n, _ := CountRows(ctx, conn, "subscriptions"); n != 30 {
		t.Fatalf("subscriptions=%d", n)
	}
	var canceled int
	conn.QueryRow(ctx, "SELECT count(*) FROM subscriptions WHERE status='canceled'").Scan(&canceled)
	if canceled != 7 {
		t.Fatalf("canceled=%d", canceled)
	}
	var email, name *string
	var created int64
	if err := conn.QueryRow(ctx, "SELECT email, name, extract(epoch FROM created)::bigint FROM customers WHERE id='cus_0000'").Scan(&email, &name, &created); err != nil {
		t.Fatal(err)
	}
	if email != nil || name != nil || created != 1735689600 {
		t.Fatalf("email=%v name=%v created=%d", email, name, created)
	}
}

func TestRunInitEnvErrors(t *testing.T) {
	cases := []struct {
		env  map[string]string
		want string
	}{
		{map[string]string{"MIRROR_API_KEY": "k"}, "MIRROR_DATABASE_URL"},
		{map[string]string{"MIRROR_DATABASE_URL": "postgres://x"}, "MIRROR_API_KEY"},
	}
	for _, tc := range cases {
		var out, errb bytes.Buffer
		code := run(context.Background(), []string{"init"}, func(k string) string { return tc.env[k] }, &out, &errb)
		if code == 0 || !strings.Contains(errb.String(), tc.want) {
			t.Errorf("env=%v code=%d stderr=%q", tc.env, code, errb.String())
		}
		other := "MIRROR_API_KEY"
		if tc.want == other {
			other = "MIRROR_DATABASE_URL"
		}
		if strings.Contains(errb.String(), other) {
			t.Errorf("error names %s too: %q", other, errb.String())
		}
	}
}

func TestRunInitRejectedKey(t *testing.T) {
	srv := fakeAPI(t, 1, 1, 0)
	env := map[string]string{"MIRROR_DATABASE_URL": "postgres://unreachable.invalid/x", "MIRROR_API_KEY": "bad", "MIRROR_API_URL": srv.URL}
	var out, errb bytes.Buffer
	code := run(context.Background(), []string{"init"}, func(k string) string { return env[k] }, &out, &errb)
	if code == 0 || !strings.Contains(errb.String(), "MIRROR_API_KEY is invalid") {
		t.Fatalf("code=%d stderr=%q", code, errb.String())
	}
}

func TestRunInitAndStatusEndToEnd(t *testing.T) {
	url := os.Getenv("MIRROR_DATABASE_URL")
	if url == "" {
		t.Skip("MIRROR_DATABASE_URL not set")
	}
	// Use a scratch schema via the connection's search_path option.
	ctx := context.Background()
	admin, err := pgx.Connect(ctx, url)
	if err != nil {
		t.Fatal(err)
	}
	defer admin.Close(ctx)
	name := fmt.Sprintf("mirror_e2e_%d", time.Now().UnixNano())
	admin.Exec(ctx, "CREATE SCHEMA "+name)
	defer admin.Exec(ctx, "DROP SCHEMA "+name+" CASCADE")
	sep := "?"
	if strings.Contains(url, "?") {
		sep = "&"
	}
	scoped := url + sep + "search_path=" + name

	srv := fakeAPI(t, 25, 12, 0)
	env := map[string]string{"MIRROR_DATABASE_URL": scoped, "MIRROR_API_KEY": testKey, "MIRROR_API_URL": srv.URL}
	getenv := func(k string) string { return env[k] }

	var out, errb bytes.Buffer
	if code := run(ctx, []string{"init"}, getenv, &out, &errb); code != 0 {
		t.Fatalf("init code=%d stderr=%s", code, errb.String())
	}
	out.Reset()
	if code := run(ctx, []string{"status"}, getenv, &out, &errb); code != 0 {
		t.Fatalf("status code=%d stderr=%s", code, errb.String())
	}
	if got := out.String(); got != "customers: 25\nsubscriptions: 12\n" {
		t.Fatalf("status output %q", got)
	}
}
