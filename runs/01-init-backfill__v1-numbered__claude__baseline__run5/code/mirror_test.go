package main

import (
	"bytes"
	"context"
	"errors"
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

// fakeAPI serves 250 customers and 230 subscriptions (30 canceled).
func fakeAPI(t *testing.T) *httptest.Server {
	t.Helper()
	var limited atomic.Bool
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
	mux.HandleFunc("/v1/account", auth(func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprint(w, `{"id":"acct_x","object":"account","livemode":false}`)
	}))
	list := func(total int, item func(i int, q url.Values) (string, bool)) http.HandlerFunc {
		return auth(func(w http.ResponseWriter, r *http.Request) {
			q := r.URL.Query()
			if limited.CompareAndSwap(false, true) {
				w.Header().Set("Retry-After", "1")
				w.WriteHeader(429)
				return
			}
			limit, _ := strconv.Atoi(q.Get("limit"))
			if limit == 0 {
				limit = 10
			}
			var items []string
			started := q.Get("starting_after") == ""
			more := false
			for i := 1; i <= total; i++ {
				js, ok := item(i, q)
				if !ok {
					continue
				}
				id := strings.Split(js, `"`)[3]
				if !started {
					started = id == q.Get("starting_after")
					continue
				}
				if len(items) == limit {
					more = true
					break
				}
				items = append(items, js)
			}
			fmt.Fprintf(w, `{"object":"list","has_more":%v,"data":[%s]}`, more, strings.Join(items, ","))
		})
	}
	mux.HandleFunc("/v1/customers", list(250, func(i int, _ url.Values) (string, bool) {
		if i%2 == 0 {
			return fmt.Sprintf(`{"id":"cus_%04d","object":"customer","email":null,"name":null,"created":%d}`, i, i), true
		}
		return fmt.Sprintf(`{"id":"cus_%04d","object":"customer","email":"a%d@x.io","name":"N%d","created":%d}`, i, i, i, i), true
	}))
	mux.HandleFunc("/v1/subscriptions", list(230, func(i int, q url.Values) (string, bool) {
		status := "active"
		if i%8 == 0 {
			status = "canceled"
		}
		if status == "canceled" && q.Get("status") != "all" && q.Get("status") != "canceled" {
			return "", false
		}
		return fmt.Sprintf(`{"id":"sub_%04d","object":"subscription","customer":"cus_0001","status":"%s","created":%d}`, i, status, i), true
	}))
	s := httptest.NewServer(mux)
	t.Cleanup(s.Close)
	return s
}

func testClient(url, key string) (*Client, *[]time.Duration) {
	var sleeps []time.Duration
	c := NewClient(url, key)
	c.Sleep = func(_ context.Context, d time.Duration) error { sleeps = append(sleeps, d); return nil }
	return c, &sleeps
}

func TestCheckAccount(t *testing.T) {
	srv := fakeAPI(t)
	c, _ := testClient(srv.URL, testKey)
	id, err := c.CheckAccount(context.Background())
	if err != nil || id != "acct_x" {
		t.Fatalf("got %q, %v", id, err)
	}
	bad, _ := testClient(srv.URL, "wrong")
	if _, err := bad.CheckAccount(context.Background()); !errors.Is(err, ErrUnauthorized) {
		t.Fatalf("want ErrUnauthorized, got %v", err)
	}
}

func TestPaginationRateLimitAndCanceled(t *testing.T) {
	srv := fakeAPI(t)
	c, sleeps := testClient(srv.URL, testKey)
	var custs []Customer
	if err := c.EachCustomerPage(context.Background(), func(p []Customer) error { custs = append(custs, p...); return nil }); err != nil {
		t.Fatal(err)
	}
	if len(custs) != 250 {
		t.Fatalf("customers: %d", len(custs))
	}
	if len(*sleeps) != 1 || (*sleeps)[0] != time.Second {
		t.Fatalf("expected one 1s Retry-After sleep, got %v", *sleeps)
	}
	if custs[1].Email != nil || custs[0].Email == nil {
		t.Fatalf("null handling wrong: %+v %+v", custs[0], custs[1])
	}
	var subs []Subscription
	if err := c.EachSubscriptionPage(context.Background(), func(p []Subscription) error { subs = append(subs, p...); return nil }); err != nil {
		t.Fatal(err)
	}
	if len(subs) != 230 {
		t.Fatalf("subscriptions (must include canceled): %d", len(subs))
	}
}

func TestRateLimitGivesUp(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Retry-After", "0")
		w.WriteHeader(429)
	}))
	defer srv.Close()
	c, _ := testClient(srv.URL, testKey)
	c.MaxRetries = 2
	if _, err := c.CheckAccount(context.Background()); err == nil || !strings.Contains(err.Error(), "rate limited") {
		t.Fatalf("got %v", err)
	}
}

func TestMissingEnv(t *testing.T) {
	for _, tc := range []struct {
		env  map[string]string
		want string
	}{
		{map[string]string{"MIRROR_API_KEY": "k"}, "MIRROR_DATABASE_URL"},
		{map[string]string{"MIRROR_DATABASE_URL": "u"}, "MIRROR_API_KEY"},
	} {
		err := run(context.Background(), []string{"init"}, func(k string) string { return tc.env[k] }, &bytes.Buffer{})
		if err == nil || !strings.Contains(err.Error(), tc.want) {
			t.Errorf("want error naming %s, got %v", tc.want, err)
		}
		other := "MIRROR_API_KEY"
		if tc.want == other {
			other = "MIRROR_DATABASE_URL"
		}
		if err != nil && strings.Contains(err.Error(), other) {
			t.Errorf("error should not name %s: %v", other, err)
		}
	}
}

// testStore opens a Store bound to a throwaway schema in MIRROR_DATABASE_URL.
func testStore(t *testing.T) (*Store, string) {
	t.Helper()
	base := os.Getenv("MIRROR_DATABASE_URL")
	if base == "" {
		t.Skip("MIRROR_DATABASE_URL not set")
	}
	schema := fmt.Sprintf("mirror_test_%d", time.Now().UnixNano())
	admin, err := OpenStore(base)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := admin.DB.Exec("CREATE SCHEMA " + schema); err != nil {
		t.Skipf("database unavailable: %v", err)
	}
	t.Cleanup(func() {
		admin.DB.Exec("DROP SCHEMA " + schema + " CASCADE")
		admin.Close()
	})
	u, _ := url.Parse(base)
	q := u.Query()
	q.Set("search_path", schema)
	u.RawQuery = q.Encode()
	s, err := OpenStore(u.String())
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { s.Close() })
	return s, u.String()
}

func TestInitAndStatus(t *testing.T) {
	srv := fakeAPI(t)
	store, dbURL := testStore(t)
	ctx := context.Background()
	c, _ := testClient(srv.URL, testKey)

	var out bytes.Buffer
	if err := runInit(ctx, c, store, &out); err != nil {
		t.Fatal(err)
	}
	// Re-running is idempotent.
	if err := runInit(ctx, c, store, &out); err != nil {
		t.Fatal(err)
	}
	var st bytes.Buffer
	if err := runStatus(ctx, store, &st); err != nil {
		t.Fatal(err)
	}
	if st.String() != "customers: 250\nsubscriptions: 230\n" {
		t.Fatalf("status: %q", st.String())
	}
	var email, name *string
	if err := store.DB.QueryRow(`SELECT email, name FROM customers WHERE id='cus_0002'`).Scan(&email, &name); err != nil || email != nil || name != nil {
		t.Fatalf("null row: %v %v %v", email, name, err)
	}
	var status string
	if err := store.DB.QueryRow(`SELECT status FROM subscriptions WHERE id='sub_0008'`).Scan(&status); err != nil || status != "canceled" {
		t.Fatalf("canceled sub: %q %v", status, err)
	}

	// Full path through run() using env.
	env := map[string]string{"MIRROR_DATABASE_URL": dbURL, "MIRROR_API_KEY": testKey, "MIRROR_API_URL": srv.URL}
	var o2 bytes.Buffer
	if err := run(ctx, []string{"status"}, func(k string) string { return env[k] }, &o2); err != nil || !strings.Contains(o2.String(), "customers: 250") {
		t.Fatalf("run status: %q %v", o2.String(), err)
	}
}

func TestInitRejectedKeyCreatesNothing(t *testing.T) {
	srv := fakeAPI(t)
	store, _ := testStore(t)
	c, _ := testClient(srv.URL, "wrong")
	err := runInit(context.Background(), c, store, &bytes.Buffer{})
	if !errors.Is(err, ErrUnauthorized) {
		t.Fatalf("got %v", err)
	}
	if _, err := store.Count(context.Background(), "customers"); err == nil {
		t.Fatal("tables should not exist after rejected key")
	}
}
