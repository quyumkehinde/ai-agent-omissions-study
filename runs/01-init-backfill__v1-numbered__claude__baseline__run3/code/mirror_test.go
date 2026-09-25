package main

import (
	"bytes"
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"strconv"
	"strings"
	"testing"
	"time"
)

// fakeAPI serves n customers and n subscriptions (every 3rd canceled).
func fakeAPI(t *testing.T, n int, throttleFirst bool) *httptest.Server {
	t.Helper()
	throttled := !throttleFirst
	h := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Authorization") != "Bearer good" {
			w.WriteHeader(401)
			fmt.Fprint(w, `{"error":{"type":"auth","message":"bad key"}}`)
			return
		}
		if r.URL.Path == "/v1/account" {
			fmt.Fprint(w, `{"id":"acct","object":"account","livemode":false}`)
			return
		}
		if !throttled && r.URL.Path == "/v1/customers" {
			throttled = true
			w.Header().Set("Retry-After", "1")
			w.WriteHeader(429)
			return
		}
		q := r.URL.Query()
		limit, _ := strconv.Atoi(q.Get("limit"))
		if limit == 0 {
			limit = 10
		}
		var items []string
		var ids []string
		switch r.URL.Path {
		case "/v1/customers":
			for i := 0; i < n; i++ {
				ids = append(ids, fmt.Sprintf("cus_%04d", i))
				items = append(items, fmt.Sprintf(`{"id":"cus_%04d","object":"customer","email":null,"name":"N%d","created":%d}`, i, i, 1000+i))
			}
		case "/v1/subscriptions":
			for i := 0; i < n; i++ {
				st := "active"
				if i%3 == 0 {
					st = "canceled"
					if q.Get("status") != "all" {
						continue
					}
				}
				ids = append(ids, fmt.Sprintf("sub_%04d", i))
				items = append(items, fmt.Sprintf(`{"id":"sub_%04d","object":"subscription","customer":"cus_0000","status":"%s","created":%d}`, i, st, 2000+i))
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
		end := min(start+limit, len(items))
		fmt.Fprintf(w, `{"object":"list","has_more":%t,"data":[%s]}`, end < len(items), strings.Join(items[start:end], ","))
	})
	return httptest.NewServer(h)
}

func testClient(url, key string) *Client {
	c := NewClient(url, key)
	c.Sleep = func(time.Duration) {}
	return c
}

func TestListPaginatesAndIncludesCanceled(t *testing.T) {
	srv := fakeAPI(t, 250, false)
	defer srv.Close()
	c := testClient(srv.URL, "good")
	var custs []Customer
	if err := c.ListCustomers(context.Background(), func(p []Customer) error { custs = append(custs, p...); return nil }); err != nil {
		t.Fatal(err)
	}
	if len(custs) != 250 || custs[0].Email != nil || *custs[7].Name != "N7" {
		t.Fatalf("customers: got %d", len(custs))
	}
	var subs []Subscription
	if err := c.ListSubscriptions(context.Background(), func(p []Subscription) error { subs = append(subs, p...); return nil }); err != nil {
		t.Fatal(err)
	}
	if len(subs) != 250 {
		t.Fatalf("subscriptions: got %d, want 250 (canceled must be included)", len(subs))
	}
}

func TestRetriesOn429(t *testing.T) {
	srv := fakeAPI(t, 5, true)
	defer srv.Close()
	var slept []time.Duration
	c := testClient(srv.URL, "good")
	c.Sleep = func(d time.Duration) { slept = append(slept, d) }
	n := 0
	if err := c.ListCustomers(context.Background(), func(p []Customer) error { n += len(p); return nil }); err != nil {
		t.Fatal(err)
	}
	if n != 5 || len(slept) != 1 || slept[0] != time.Second {
		t.Fatalf("n=%d slept=%v", n, slept)
	}
}

func TestBadKey(t *testing.T) {
	srv := fakeAPI(t, 1, false)
	defer srv.Close()
	err := testClient(srv.URL, "bad").CheckAccount(context.Background())
	if err != errUnauthorized {
		t.Fatalf("got %v", err)
	}
}

func env(m map[string]string) func(string) string { return func(k string) string { return m[k] } }

func TestMissingEnv(t *testing.T) {
	for _, tc := range []struct {
		env  map[string]string
		want []string
	}{
		{map[string]string{}, []string{"MIRROR_DATABASE_URL", "MIRROR_API_KEY"}},
		{map[string]string{"MIRROR_API_KEY": "x"}, []string{"MIRROR_DATABASE_URL"}},
		{map[string]string{"MIRROR_DATABASE_URL": "x"}, []string{"MIRROR_API_KEY"}},
	} {
		var out, errb bytes.Buffer
		if code := run(context.Background(), []string{"status"}, env(tc.env), &out, &errb); code != 1 {
			t.Fatalf("code %d", code)
		}
		for _, w := range tc.want {
			if !strings.Contains(errb.String(), w) {
				t.Errorf("stderr %q missing %s", errb.String(), w)
			}
		}
		if len(tc.want) == 1 {
			other := "MIRROR_API_KEY"
			if tc.want[0] == other {
				other = "MIRROR_DATABASE_URL"
			}
			if strings.Contains(errb.String(), other) {
				t.Errorf("stderr %q should not mention %s", errb.String(), other)
			}
		}
	}
}

func TestInitRejectedKeyFailsBeforeDatabase(t *testing.T) {
	srv := fakeAPI(t, 1, false)
	defer srv.Close()
	var out, errb bytes.Buffer
	// The DB URL is unreachable; a rejected key must be reported first.
	code := run(context.Background(), []string{"init"}, env(map[string]string{
		"MIRROR_DATABASE_URL": "postgres://127.0.0.1:1/none",
		"MIRROR_API_KEY":      "bad",
		"MIRROR_API_URL":      srv.URL,
	}), &out, &errb)
	if code != 1 || !strings.Contains(errb.String(), "API key rejected") {
		t.Fatalf("code=%d stderr=%q", code, errb.String())
	}
}

func TestBadUsage(t *testing.T) {
	var out, errb bytes.Buffer
	if code := run(context.Background(), []string{"nope"}, env(nil), &out, &errb); code != 2 {
		t.Fatalf("code %d", code)
	}
}

// TestInitAndStatusPostgres needs a real database; set MIRROR_TEST_DATABASE_URL
// (its mirror tables are dropped and recreated).
func TestInitAndStatusPostgres(t *testing.T) {
	dsn := os.Getenv("MIRROR_TEST_DATABASE_URL")
	if dsn == "" {
		t.Skip("MIRROR_TEST_DATABASE_URL not set")
	}
	ctx := context.Background()
	st, err := OpenStore(ctx, dsn)
	if err != nil {
		t.Fatal(err)
	}
	defer st.Close(ctx)
	if _, err := st.conn.Exec(ctx, "DROP TABLE IF EXISTS customers, subscriptions"); err != nil {
		t.Fatal(err)
	}
	srv := fakeAPI(t, 150, false)
	defer srv.Close()
	e := env(map[string]string{"MIRROR_DATABASE_URL": dsn, "MIRROR_API_KEY": "good", "MIRROR_API_URL": srv.URL})
	for i := 0; i < 2; i++ { // second run proves idempotence
		var out, errb bytes.Buffer
		if code := run(ctx, []string{"init"}, e, &out, &errb); code != 0 {
			t.Fatalf("init: %s", errb.String())
		}
	}
	var out, errb bytes.Buffer
	if code := run(ctx, []string{"status"}, e, &out, &errb); code != 0 {
		t.Fatal(errb.String())
	}
	if out.String() != "customers: 150\nsubscriptions: 150\n" {
		t.Fatalf("status output %q", out.String())
	}
}
