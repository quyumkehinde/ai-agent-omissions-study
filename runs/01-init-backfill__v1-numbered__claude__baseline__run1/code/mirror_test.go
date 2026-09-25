package main

import (
	"context"
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
	env := map[string]string{}
	get := func(k string) string { return env[k] }
	_, err := loadConfig(get)
	if err == nil || !strings.Contains(err.Error(), "MIRROR_DATABASE_URL") || !strings.Contains(err.Error(), "MIRROR_API_KEY") {
		t.Fatalf("want both names in error, got %v", err)
	}
	env["MIRROR_DATABASE_URL"] = "x"
	_, err = loadConfig(get)
	if err == nil || !strings.Contains(err.Error(), "MIRROR_API_KEY") || strings.Contains(err.Error(), "MIRROR_DATABASE_URL") {
		t.Fatalf("want only API key named, got %v", err)
	}
	env["MIRROR_API_KEY"] = "k"
	cfg, err := loadConfig(get)
	if err != nil || cfg.BaseURL != DefaultBaseURL {
		t.Fatalf("got %+v %v", cfg, err)
	}
}

func TestRunUsage(t *testing.T) {
	if err := run(context.Background(), []string{"bogus"}, func(string) string { return "" }, nil); err == nil {
		t.Fatal("expected usage error")
	}
}

// fakeAPI serves n customers and subscriptions (every 3rd canceled), paginated,
// with a one-time 429 on the first customers request.
func fakeAPI(t *testing.T, n int) *httptest.Server {
	limited := false
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Authorization") != "Bearer good" {
			w.WriteHeader(401)
			fmt.Fprint(w, `{"error":{"type":"auth","message":"bad key"}}`)
			return
		}
		q := r.URL.Query()
		limit, _ := strconv.Atoi(q.Get("limit"))
		if limit == 0 {
			limit = 10
		}
		switch r.URL.Path {
		case "/v1/account":
			fmt.Fprint(w, `{"id":"acct_x","object":"account","livemode":false}`)
		case "/v1/customers", "/v1/subscriptions":
			if r.URL.Path == "/v1/customers" && !limited {
				limited = true
				w.Header().Set("Retry-After", "2")
				w.WriteHeader(429)
				return
			}
			isSub := r.URL.Path == "/v1/subscriptions"
			var ids []string
			var items []string
			for i := 0; i < n; i++ {
				if isSub {
					st := "active"
					if i%3 == 0 {
						st = "canceled"
					}
					if st == "canceled" && q.Get("status") != "all" {
						continue
					}
					ids = append(ids, fmt.Sprintf("sub_%03d", i))
					items = append(items, fmt.Sprintf(`{"id":"sub_%03d","object":"subscription","customer":"cus_%03d","status":"%s","created":%d}`, i, i, st, 1000+i))
				} else {
					ids = append(ids, fmt.Sprintf("cus_%03d", i))
					name := `"N"`
					if i == 1 {
						name = "null"
					}
					items = append(items, fmt.Sprintf(`{"id":"cus_%03d","object":"customer","email":"e%d@x.com","name":%s,"created":%d}`, i, i, name, 1000+i))
				}
			}
			start := 0
			if a := q.Get("starting_after"); a != "" {
				for i, id := range ids {
					if id == a {
						start = i + 1
					}
				}
			}
			end := start + limit
			more := end < len(items)
			if end > len(items) {
				end = len(items)
			}
			fmt.Fprintf(w, `{"object":"list","has_more":%t,"data":[%s]}`, more, strings.Join(items[start:end], ","))
		}
	}))
}

func TestAccount(t *testing.T) {
	srv := fakeAPI(t, 1)
	defer srv.Close()
	if _, err := NewClient(srv.URL, "good").Account(context.Background()); err != nil {
		t.Fatal(err)
	}
	_, err := NewClient(srv.URL, "bad").Account(context.Background())
	if !errors.Is(err, ErrUnauthorized) {
		t.Fatalf("want ErrUnauthorized, got %v", err)
	}
}

func TestListPaginationRetryAndCanceled(t *testing.T) {
	srv := fakeAPI(t, 250)
	defer srv.Close()
	c := NewClient(srv.URL, "good")
	var slept []time.Duration
	c.Sleep = func(d time.Duration) { slept = append(slept, d) }

	var cs []Customer
	if err := c.ListCustomers(context.Background(), func(p []Customer) error { cs = append(cs, p...); return nil }); err != nil {
		t.Fatal(err)
	}
	if len(cs) != 250 {
		t.Fatalf("customers: got %d", len(cs))
	}
	if len(slept) != 1 || slept[0] != 2*time.Second {
		t.Fatalf("Retry-After not honored: %v", slept)
	}
	if cs[1].Name != nil {
		t.Fatal("null name should be nil")
	}

	var ss []Subscription
	if err := c.ListSubscriptions(context.Background(), func(p []Subscription) error { ss = append(ss, p...); return nil }); err != nil {
		t.Fatal(err)
	}
	if len(ss) != 250 {
		t.Fatalf("subscriptions (incl. canceled): got %d", len(ss))
	}
}

func TestInitBadKeyFailsBeforeDB(t *testing.T) {
	srv := fakeAPI(t, 1)
	defer srv.Close()
	env := map[string]string{"MIRROR_DATABASE_URL": "postgres://invalid.invalid/x", "MIRROR_API_KEY": "bad", "MIRROR_API_URL": srv.URL}
	err := run(context.Background(), []string{"init"}, func(k string) string { return env[k] }, &strings.Builder{})
	if !errors.Is(err, ErrUnauthorized) {
		t.Fatalf("want ErrUnauthorized, got %v", err)
	}
}

// Requires a Postgres database: set MIRROR_TEST_DATABASE_URL (tables are dropped and recreated).
func TestInitAndStatusPostgres(t *testing.T) {
	dsn := os.Getenv("MIRROR_TEST_DATABASE_URL")
	if dsn == "" {
		t.Skip("MIRROR_TEST_DATABASE_URL not set")
	}
	ctx := context.Background()
	srv := fakeAPI(t, 250)
	defer srv.Close()
	st, err := OpenStore(ctx, dsn)
	if err != nil {
		t.Fatal(err)
	}
	defer st.Close()
	if _, err := st.pool.Exec(ctx, "DROP TABLE IF EXISTS customers, subscriptions"); err != nil {
		t.Fatal(err)
	}
	env := map[string]string{"MIRROR_DATABASE_URL": dsn, "MIRROR_API_KEY": "good", "MIRROR_API_URL": srv.URL}
	get := func(k string) string { return env[k] }
	for i := 0; i < 2; i++ { // second run checks idempotency
		if err := run(ctx, []string{"init"}, get, &strings.Builder{}); err != nil {
			t.Fatal(err)
		}
	}
	var out strings.Builder
	if err := run(ctx, []string{"status"}, get, &out); err != nil {
		t.Fatal(err)
	}
	if out.String() != "customers: 250\nsubscriptions: 250\n" {
		t.Fatalf("status output: %q", out.String())
	}
}
