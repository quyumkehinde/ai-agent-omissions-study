package main

import (
	"bytes"
	"context"
	"database/sql"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"strconv"
	"strings"
	"sync/atomic"
	"testing"
	"time"
)

const testKey = "sk_test_omission"

// fakeAPI serves n customers and n subscriptions (every 3rd canceled).
func fakeAPI(t *testing.T, n int, rateLimitFirst int32) *httptest.Server {
	var limited atomic.Int32
	mux := http.NewServeMux()
	auth := func(h http.HandlerFunc) http.HandlerFunc {
		return func(w http.ResponseWriter, r *http.Request) {
			if r.Header.Get("Authorization") != "Bearer "+testKey {
				w.WriteHeader(401)
				fmt.Fprint(w, `{"error":{"type":"auth","message":"bad key"}}`)
				return
			}
			h(w, r)
		}
	}
	list := func(kind string, item func(i int) string, skip func(i int, q map[string][]string) bool) http.HandlerFunc {
		return auth(func(w http.ResponseWriter, r *http.Request) {
			if limited.Add(1) <= rateLimitFirst {
				w.Header().Set("Retry-After", "1")
				w.WriteHeader(429)
				return
			}
			q := r.URL.Query()
			limit, _ := strconv.Atoi(q.Get("limit"))
			start := 0
			if a := q.Get("starting_after"); a != "" {
				fmt.Sscanf(a, kind+"_%d", &start)
			}
			var items []string
			more := false
			for i := start + 1; i <= n; i++ {
				if skip(i, q) {
					continue
				}
				if len(items) == limit {
					more = true
					break
				}
				items = append(items, item(i))
			}
			fmt.Fprintf(w, `{"object":"list","has_more":%v,"data":[%s]}`, more, strings.Join(items, ","))
		})
	}
	mux.HandleFunc("/v1/account", auth(func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprint(w, `{"id":"acct_omission","object":"account","livemode":false}`)
	}))
	mux.HandleFunc("/v1/customers", list("cus",
		func(i int) string {
			return fmt.Sprintf(`{"id":"cus_%d","object":"customer","email":"u%d@x.com","name":null,"created":%d}`, i, i, 1735689600+i)
		},
		func(int, map[string][]string) bool { return false }))
	mux.HandleFunc("/v1/subscriptions", list("sub",
		func(i int) string {
			st := "active"
			if i%3 == 0 {
				st = "canceled"
			}
			return fmt.Sprintf(`{"id":"sub_%d","object":"subscription","customer":"cus_1","status":"%s","created":%d}`, i, st, 1735689600+i)
		},
		func(i int, q map[string][]string) bool {
			return i%3 == 0 && (len(q["status"]) == 0 || q["status"][0] != "all")
		}))
	s := httptest.NewServer(mux)
	t.Cleanup(s.Close)
	return s
}

func noSleep(slept *[]time.Duration) func(context.Context, time.Duration) error {
	return func(_ context.Context, d time.Duration) error { *slept = append(*slept, d); return nil }
}

func TestPaginationAndCanceled(t *testing.T) {
	srv := fakeAPI(t, 250, 0)
	c := NewClient(srv.URL, testKey)
	var cn, sn int
	if err := c.EachCustomerPage(context.Background(), func(p []Customer) error { cn += len(p); return nil }); err != nil {
		t.Fatal(err)
	}
	seen := map[string]bool{}
	canceled := 0
	err := c.EachSubscriptionPage(context.Background(), func(p []Subscription) error {
		for _, s := range p {
			seen[s.ID] = true
			if s.Status == "canceled" {
				canceled++
			}
		}
		sn += len(p)
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	if cn != 250 || sn != 250 || len(seen) != 250 || canceled != 83 {
		t.Fatalf("customers=%d subs=%d unique=%d canceled=%d", cn, sn, len(seen), canceled)
	}
}

func TestNullableFields(t *testing.T) {
	srv := fakeAPI(t, 1, 0)
	c := NewClient(srv.URL, testKey)
	var got Customer
	c.EachCustomerPage(context.Background(), func(p []Customer) error { got = p[0]; return nil })
	if got.Name != nil || got.Email == nil || *got.Email != "u1@x.com" {
		t.Fatalf("%+v", got)
	}
}

func TestRateLimitRetry(t *testing.T) {
	srv := fakeAPI(t, 5, 2)
	c := NewClient(srv.URL, testKey)
	var slept []time.Duration
	c.Sleep = noSleep(&slept)
	n := 0
	if err := c.EachCustomerPage(context.Background(), func(p []Customer) error { n += len(p); return nil }); err != nil {
		t.Fatal(err)
	}
	if n != 5 || len(slept) != 2 || slept[0] != time.Second {
		t.Fatalf("n=%d slept=%v", n, slept)
	}
}

func TestRateLimitGivesUp(t *testing.T) {
	srv := fakeAPI(t, 5, 1000)
	c := NewClient(srv.URL, testKey)
	var slept []time.Duration
	c.Sleep = noSleep(&slept)
	err := c.EachCustomerPage(context.Background(), func([]Customer) error { return nil })
	if err == nil || !strings.Contains(err.Error(), "rate limited") {
		t.Fatalf("err=%v", err)
	}
}

func TestAccountUnauthorized(t *testing.T) {
	srv := fakeAPI(t, 1, 0)
	if err := NewClient(srv.URL, "wrong").Account(context.Background()); err != ErrUnauthorized {
		t.Fatalf("err=%v", err)
	}
	if err := NewClient(srv.URL, testKey).Account(context.Background()); err != nil {
		t.Fatal(err)
	}
}

func env(m map[string]string) func(string) string { return func(k string) string { return m[k] } }

func TestMissingEnv(t *testing.T) {
	cases := []struct {
		env  map[string]string
		want string
	}{
		{map[string]string{"MIRROR_API_KEY": "k"}, "MIRROR_DATABASE_URL"},
		{map[string]string{"MIRROR_DATABASE_URL": "postgres://x"}, "MIRROR_API_KEY"},
	}
	for _, c := range cases {
		err := run(context.Background(), []string{"init"}, env(c.env), &bytes.Buffer{})
		if err == nil || !strings.Contains(err.Error(), c.want) {
			t.Errorf("env %v: err=%v, want mention of %s", c.env, err, c.want)
		}
	}
	err := run(context.Background(), []string{"status"}, env(nil), &bytes.Buffer{})
	if err == nil || !strings.Contains(err.Error(), "MIRROR_DATABASE_URL") {
		t.Errorf("status err=%v", err)
	}
	err = run(context.Background(), []string{"init"}, env(nil), &bytes.Buffer{})
	if err == nil || !strings.Contains(err.Error(), "MIRROR_DATABASE_URL") || !strings.Contains(err.Error(), "MIRROR_API_KEY") {
		t.Errorf("both missing err=%v", err)
	}
}

func TestInitRejectedKey(t *testing.T) {
	srv := fakeAPI(t, 1, 0)
	err := run(context.Background(), []string{"init"}, env(map[string]string{
		"MIRROR_DATABASE_URL": "postgres://unused", "MIRROR_API_KEY": "nope", "MIRROR_API_URL": srv.URL,
	}), &bytes.Buffer{})
	if err == nil || !strings.Contains(err.Error(), "MIRROR_API_KEY was rejected") {
		t.Fatalf("err=%v", err)
	}
}

func TestUnknownCommand(t *testing.T) {
	if err := run(context.Background(), []string{"bogus"}, env(nil), &bytes.Buffer{}); err == nil {
		t.Fatal("expected error")
	}
}

// Postgres-backed test; runs only when MIRROR_TEST_DATABASE_URL is set.
// WARNING: it drops the customers and subscriptions tables in that database.
func TestInitAndStatusPostgres(t *testing.T) {
	url := os.Getenv("MIRROR_TEST_DATABASE_URL")
	if url == "" {
		t.Skip("MIRROR_TEST_DATABASE_URL not set")
	}
	ctx := context.Background()
	db, err := sql.Open("pgx", url)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	db.Exec("DROP TABLE IF EXISTS customers, subscriptions")

	srv := fakeAPI(t, 230, 0)
	e := env(map[string]string{"MIRROR_DATABASE_URL": url, "MIRROR_API_KEY": testKey, "MIRROR_API_URL": srv.URL})
	for i := 0; i < 2; i++ { // second run proves idempotency
		var out bytes.Buffer
		if err := run(ctx, []string{"init"}, e, &out); err != nil {
			t.Fatal(err, out.String())
		}
	}
	var out bytes.Buffer
	if err := run(ctx, []string{"status"}, e, &out); err != nil {
		t.Fatal(err)
	}
	s := out.String()
	if !strings.Contains(s, "customers") || !strings.Contains(s, "230") {
		t.Fatalf("status output: %q", s)
	}
	var canceled int
	db.QueryRow("SELECT count(*) FROM subscriptions WHERE status='canceled'").Scan(&canceled)
	if canceled != 76 {
		t.Fatalf("canceled=%d", canceled)
	}
	var name sql.NullString
	db.QueryRow("SELECT name FROM customers WHERE id='cus_1'").Scan(&name)
	if name.Valid {
		t.Fatal("expected NULL name")
	}
}
