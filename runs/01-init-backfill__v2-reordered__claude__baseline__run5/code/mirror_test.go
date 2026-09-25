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
	"sync/atomic"
	"testing"
	"time"
)

// fakeAPI serves n customers and n subscriptions (every 3rd canceled), honoring
// limit, starting_after and status like the real API.
func fakeAPI(t *testing.T, n int, key string, rateLimitFirst int32) *httptest.Server {
	var calls atomic.Int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Authorization") != "Bearer "+key {
			w.WriteHeader(401)
			w.Write([]byte(`{"error":{"type":"auth","message":"bad key"}}`))
			return
		}
		if r.URL.Path == "/v1/account" {
			w.Write([]byte(`{"id":"acct_omission","object":"account","livemode":false}`))
			return
		}
		if calls.Add(1) <= rateLimitFirst {
			w.Header().Set("Retry-After", "2")
			w.WriteHeader(429)
			return
		}
		q := r.URL.Query()
		limit, _ := strconv.Atoi(q.Get("limit"))
		if limit == 0 {
			limit = 10
		}
		var all []any
		var ids []string
		switch r.URL.Path {
		case "/v1/customers":
			for i := 0; i < n; i++ {
				id := fmt.Sprintf("cus_%04d", i)
				var name any = "N" + id
				if i == 1 {
					name = nil
				}
				all = append(all, map[string]any{"id": id, "object": "customer", "email": id + "@x.io", "name": name, "created": 1700000000 + i})
				ids = append(ids, id)
			}
		case "/v1/subscriptions":
			for i := 0; i < n; i++ {
				st := "active"
				if i%3 == 0 {
					st = "canceled"
				}
				if st == "canceled" && q.Get("status") != "all" && q.Get("status") != "canceled" {
					continue
				}
				id := fmt.Sprintf("sub_%04d", i)
				all = append(all, map[string]any{"id": id, "object": "subscription", "customer": "cus_0000", "status": st, "created": 1700000000 + i})
				ids = append(ids, id)
			}
		default:
			w.WriteHeader(404)
			return
		}
		start := 0
		if a := q.Get("starting_after"); a != "" {
			for i, id := range ids {
				if id == a {
					start = i + 1
				}
			}
		}
		end := min(start+limit, len(all))
		json.NewEncoder(w).Encode(map[string]any{"object": "list", "has_more": end < len(all), "data": all[start:end]})
	}))
	t.Cleanup(srv.Close)
	return srv
}

func testClient(url, key string) (*Client, *[]time.Duration) {
	c := NewClient(url, key)
	var sleeps []time.Duration
	c.Sleep = func(_ context.Context, d time.Duration) error { sleeps = append(sleeps, d); return nil }
	return c, &sleeps
}

func TestAccountRejectedKey(t *testing.T) {
	srv := fakeAPI(t, 1, "good", 0)
	c, _ := testClient(srv.URL, "bad")
	if _, err := c.Account(context.Background()); !errors.Is(err, ErrUnauthorized) {
		t.Fatalf("got %v, want ErrUnauthorized", err)
	}
	c, _ = testClient(srv.URL, "good")
	if a, err := c.Account(context.Background()); err != nil || a.ID != "acct_omission" {
		t.Fatalf("got %v, %v", a, err)
	}
}

func TestEachCustomerPaginatesAll(t *testing.T) {
	srv := fakeAPI(t, 250, "k", 0)
	c, _ := testClient(srv.URL, "k")
	seen := map[string]bool{}
	var nilName int
	err := c.EachCustomer(context.Background(), func(cs []Customer) error {
		for _, x := range cs {
			seen[x.ID] = true
			if x.Name == nil {
				nilName++
			}
		}
		return nil
	})
	if err != nil || len(seen) != 250 || nilName != 1 {
		t.Fatalf("err=%v seen=%d nilName=%d", err, len(seen), nilName)
	}
}

func TestEachSubscriptionIncludesCanceled(t *testing.T) {
	srv := fakeAPI(t, 30, "k", 0)
	c, _ := testClient(srv.URL, "k")
	total, canceled := 0, 0
	err := c.EachSubscription(context.Background(), func(ss []Subscription) error {
		for _, s := range ss {
			total++
			if *s.Status == "canceled" {
				canceled++
			}
		}
		return nil
	})
	if err != nil || total != 30 || canceled != 10 {
		t.Fatalf("err=%v total=%d canceled=%d", err, total, canceled)
	}
}

func TestRateLimitRetry(t *testing.T) {
	srv := fakeAPI(t, 5, "k", 2)
	c, sleeps := testClient(srv.URL, "k")
	n := 0
	err := c.EachCustomer(context.Background(), func(cs []Customer) error { n += len(cs); return nil })
	if err != nil || n != 5 {
		t.Fatalf("err=%v n=%d", err, n)
	}
	if len(*sleeps) != 2 || (*sleeps)[0] != 2*time.Second {
		t.Fatalf("sleeps=%v", *sleeps)
	}
}

func TestRateLimitGivesUp(t *testing.T) {
	srv := fakeAPI(t, 5, "k", 1000)
	c, _ := testClient(srv.URL, "k")
	if err := c.EachCustomer(context.Background(), func([]Customer) error { return nil }); err == nil {
		t.Fatal("expected error")
	}
}

func TestRunEnvErrors(t *testing.T) {
	env := func(m map[string]string) func(string) string { return func(k string) string { return m[k] } }
	cases := []struct {
		args []string
		env  map[string]string
		want string
	}{
		{[]string{"init"}, map[string]string{"MIRROR_API_KEY": "k"}, "MIRROR_DATABASE_URL"},
		{[]string{"init"}, map[string]string{"MIRROR_DATABASE_URL": "postgres://x"}, "MIRROR_API_KEY"},
		{[]string{"status"}, nil, "MIRROR_DATABASE_URL"},
		{[]string{"bogus"}, nil, "usage"},
		{nil, nil, "usage"},
	}
	for _, tc := range cases {
		err := run(context.Background(), tc.args, env(tc.env), &bytes.Buffer{})
		if err == nil || !strings.Contains(err.Error(), tc.want) {
			t.Errorf("%v: got %v, want mention of %q", tc.args, err, tc.want)
		}
	}
}

func TestInitRejectedKeyFailsBeforeDB(t *testing.T) {
	srv := fakeAPI(t, 1, "good", 0)
	c, _ := testClient(srv.URL, "bad")
	// Store has a nil DB: the key check must fail before it is touched.
	err := runInit(context.Background(), c, &Store{}, &bytes.Buffer{})
	if !errors.Is(err, ErrUnauthorized) {
		t.Fatalf("got %v", err)
	}
}

// Database tests need a real Postgres: set MIRROR_TEST_DATABASE_URL.
func TestInitAndStatusWithPostgres(t *testing.T) {
	dsn := os.Getenv("MIRROR_TEST_DATABASE_URL")
	if dsn == "" {
		t.Skip("MIRROR_TEST_DATABASE_URL not set")
	}
	db, err := sql.Open("postgres", dsn)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	ctx := context.Background()
	db.ExecContext(ctx, "DROP TABLE IF EXISTS customers, subscriptions")
	t.Cleanup(func() { db.Exec("DROP TABLE IF EXISTS customers, subscriptions") })

	srv := fakeAPI(t, 230, "k", 0)
	c, _ := testClient(srv.URL, "k")
	s := &Store{DB: db}
	var out bytes.Buffer
	for i := 0; i < 2; i++ { // second run proves init is idempotent
		if err := runInit(ctx, c, s, &out); err != nil {
			t.Fatal(err)
		}
	}
	var st bytes.Buffer
	if err := runStatus(ctx, s, &st); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(st.String(), "customers") || !strings.Contains(st.String(), "230") {
		t.Fatalf("status output: %q", st.String())
	}
	counts, _ := s.Counts(ctx)
	if counts["customers"] != 230 || counts["subscriptions"] != 230 {
		t.Fatalf("counts=%v", counts)
	}
}
