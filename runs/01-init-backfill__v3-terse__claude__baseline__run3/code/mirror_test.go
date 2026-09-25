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
	"sync/atomic"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"
)

// fakeAPI serves n customers and n subscriptions (every 3rd canceled).
func fakeAPI(t *testing.T, n int, key string) (*httptest.Server, *atomic.Int32) {
	t.Helper()
	var throttled atomic.Int32
	mux := http.NewServeMux()
	auth := func(h http.HandlerFunc) http.HandlerFunc {
		return func(w http.ResponseWriter, r *http.Request) {
			if r.Header.Get("Authorization") != "Bearer "+key {
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
	list := func(kind string, item func(i int) (id, body string, canceled bool)) http.HandlerFunc {
		return auth(func(w http.ResponseWriter, r *http.Request) {
			if kind == "customers" && throttled.Add(1) == 1 {
				w.Header().Set("Retry-After", "2")
				w.WriteHeader(429)
				return
			}
			q := r.URL.Query()
			limit, _ := strconv.Atoi(q.Get("limit"))
			if limit == 0 {
				limit = 10
			}
			started := q.Get("starting_after") == ""
			var items []string
			more := false
			for i := 1; i <= n; i++ {
				id, body, canceled := item(i)
				if !started {
					started = id == q.Get("starting_after")
					continue
				}
				if canceled && q.Get("status") != "all" {
					continue
				}
				if len(items) == limit {
					more = true
					break
				}
				items = append(items, body)
			}
			fmt.Fprintf(w, `{"object":"list","has_more":%t,"data":[%s]}`, more, strings.Join(items, ","))
		})
	}
	mux.HandleFunc("/v1/customers", list("customers", func(i int) (string, string, bool) {
		id := fmt.Sprintf("cus_%04d", i)
		name := fmt.Sprintf(`"Customer %d"`, i)
		if i == 2 {
			name = "null"
		}
		return id, fmt.Sprintf(`{"id":%q,"object":"customer","email":"u%d@example.com","name":%s,"created":%d}`, id, i, name, 1735689600+i), false
	}))
	mux.HandleFunc("/v1/subscriptions", list("subscriptions", func(i int) (string, string, bool) {
		id := fmt.Sprintf("sub_%04d", i)
		st := "active"
		if i%3 == 0 {
			st = "canceled"
		}
		return id, fmt.Sprintf(`{"id":%q,"object":"subscription","customer":"cus_0001","status":%q,"created":%d}`, id, st, 1735689600+i), st == "canceled"
	}))
	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)
	return srv, &throttled
}

func noSleep(c *Client) *[]time.Duration {
	var waits []time.Duration
	c.Sleep = func(_ context.Context, d time.Duration) error { waits = append(waits, d); return nil }
	return &waits
}

func TestLoadConfig(t *testing.T) {
	env := func(m map[string]string) func(string) string { return func(k string) string { return m[k] } }
	cases := []struct {
		name string
		env  map[string]string
		want []string // substrings; nil = success
	}{
		{"ok", map[string]string{"MIRROR_DATABASE_URL": "d", "MIRROR_API_KEY": "k"}, nil},
		{"no db", map[string]string{"MIRROR_API_KEY": "k"}, []string{"MIRROR_DATABASE_URL"}},
		{"no key", map[string]string{"MIRROR_DATABASE_URL": "d"}, []string{"MIRROR_API_KEY"}},
		{"neither", nil, []string{"MIRROR_DATABASE_URL", "MIRROR_API_KEY"}},
	}
	for _, c := range cases {
		_, err := LoadConfig(env(c.env))
		if c.want == nil {
			if err != nil {
				t.Errorf("%s: %v", c.name, err)
			}
			continue
		}
		if err == nil {
			t.Errorf("%s: expected error", c.name)
			continue
		}
		for _, w := range c.want {
			if !strings.Contains(err.Error(), w) {
				t.Errorf("%s: %q missing %q", c.name, err, w)
			}
		}
	}
	// only the missing one is named
	_, err := LoadConfig(env(map[string]string{"MIRROR_API_KEY": "k"}))
	if strings.Contains(err.Error(), "MIRROR_API_KEY") {
		t.Errorf("error names a variable that is set: %v", err)
	}
}

func TestRunInitMissingEnvNamed(t *testing.T) {
	err := run(context.Background(), []string{"init"}, func(k string) string {
		if k == "MIRROR_API_KEY" {
			return "k"
		}
		return ""
	}, &bytes.Buffer{})
	if err == nil || !strings.Contains(err.Error(), "MIRROR_DATABASE_URL") {
		t.Fatalf("got %v", err)
	}
}

func TestRunInitRejectedKey(t *testing.T) {
	srv, _ := fakeAPI(t, 1, "good")
	env := map[string]string{"MIRROR_DATABASE_URL": "postgres://unused", "MIRROR_API_KEY": "bad", "MIRROR_API_URL": srv.URL}
	err := run(context.Background(), []string{"init"}, func(k string) string { return env[k] }, &bytes.Buffer{})
	if err == nil || !strings.Contains(err.Error(), "MIRROR_API_KEY") || !strings.Contains(err.Error(), "rejected") {
		t.Fatalf("got %v", err)
	}
}

func TestUsage(t *testing.T) {
	if err := run(context.Background(), []string{"bogus"}, func(string) string { return "" }, &bytes.Buffer{}); err == nil {
		t.Fatal("expected usage error")
	}
}

func TestClientPaginationAndCanceled(t *testing.T) {
	srv, _ := fakeAPI(t, 250, "k")
	c := NewClient(srv.URL, "k")
	noSleep(c)
	ctx := context.Background()

	var cust []Customer
	if err := c.EachCustomerPage(ctx, func(p []Customer) error { cust = append(cust, p...); return nil }); err != nil {
		t.Fatal(err)
	}
	if len(cust) != 250 || cust[249].ID != "cus_0250" {
		t.Fatalf("customers: %d", len(cust))
	}
	if cust[1].Name != nil {
		t.Errorf("null name should be nil")
	}

	var subs []Subscription
	if err := c.EachSubscriptionPage(ctx, func(p []Subscription) error { subs = append(subs, p...); return nil }); err != nil {
		t.Fatal(err)
	}
	if len(subs) != 250 {
		t.Fatalf("subscriptions: got %d, want 250 (canceled must be included)", len(subs))
	}
}

func TestClientRetryAfter(t *testing.T) {
	srv, throttled := fakeAPI(t, 3, "k")
	c := NewClient(srv.URL, "k")
	waits := noSleep(c)
	n := 0
	if err := c.EachCustomerPage(context.Background(), func(p []Customer) error { n += len(p); return nil }); err != nil {
		t.Fatal(err)
	}
	if n != 3 || throttled.Load() < 2 {
		t.Fatalf("n=%d throttled=%d", n, throttled.Load())
	}
	if len(*waits) != 1 || (*waits)[0] != 2*time.Second {
		t.Fatalf("waits = %v", *waits)
	}
}

func TestClientRateLimitGivesUp(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Retry-After", "1")
		w.WriteHeader(429)
	}))
	defer srv.Close()
	c := NewClient(srv.URL, "k")
	noSleep(c)
	if err := c.EachCustomerPage(context.Background(), func([]Customer) error { return nil }); err == nil {
		t.Fatal("expected error")
	}
}

func TestClientServerError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(500)
		fmt.Fprint(w, `{"error":{"type":"api_error","message":"boom"}}`)
	}))
	defer srv.Close()
	err := NewClient(srv.URL, "k").CheckAccount(context.Background())
	if err == nil || !strings.Contains(err.Error(), "boom") {
		t.Fatalf("got %v", err)
	}
}

// --- Postgres integration (set MIRROR_TEST_DATABASE_URL to enable) ---

func testConn(t *testing.T) *pgx.Conn {
	t.Helper()
	url := os.Getenv("MIRROR_TEST_DATABASE_URL")
	if url == "" {
		t.Skip("MIRROR_TEST_DATABASE_URL not set")
	}
	ctx := context.Background()
	conn, err := pgx.Connect(ctx, url)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { conn.Close(ctx) })
	if _, err := conn.Exec(ctx, "DROP TABLE IF EXISTS customers, subscriptions"); err != nil {
		t.Fatal(err)
	}
	return conn
}

func TestInitEndToEnd(t *testing.T) {
	testConn(t)
	srv, _ := fakeAPI(t, 250, "k")
	env := map[string]string{
		"MIRROR_DATABASE_URL": os.Getenv("MIRROR_TEST_DATABASE_URL"),
		"MIRROR_API_KEY":      "k",
		"MIRROR_API_URL":      srv.URL,
	}
	getenv := func(k string) string { return env[k] }
	ctx := context.Background()

	// throttle sleep of 2s is real here; acceptable but keep it short by running once.
	var out bytes.Buffer
	if err := run(ctx, []string{"init"}, getenv, &out); err != nil {
		t.Fatal(err)
	}
	// Re-running must be idempotent.
	if err := run(ctx, []string{"init"}, getenv, &bytes.Buffer{}); err != nil {
		t.Fatal(err)
	}

	out.Reset()
	if err := run(ctx, []string{"status"}, getenv, &out); err != nil {
		t.Fatal(err)
	}
	s := out.String()
	if !strings.Contains(s, "customers") || !strings.Contains(s, "250") ||
		!regexpLine(s, "subscriptions", "250") || !regexpLine(s, "customers", "250") {
		t.Fatalf("status output:\n%s", s)
	}

	conn := testConn2(t)
	var canceled int
	conn.QueryRow(ctx, "SELECT count(*) FROM subscriptions WHERE status='canceled'").Scan(&canceled)
	if canceled != 83 {
		t.Errorf("canceled = %d, want 83", canceled)
	}
	var name *string
	var created time.Time
	if err := conn.QueryRow(ctx, "SELECT name, created FROM customers WHERE id='cus_0002'").Scan(&name, &created); err != nil {
		t.Fatal(err)
	}
	if name != nil || created.Unix() != 1735689602 {
		t.Errorf("name=%v created=%v", name, created)
	}
}

func regexpLine(s, table, n string) bool {
	for _, l := range strings.Split(s, "\n") {
		f := strings.Fields(l)
		if len(f) == 2 && f[0] == table && f[1] == n {
			return true
		}
	}
	return false
}

func testConn2(t *testing.T) *pgx.Conn {
	conn, err := pgx.Connect(context.Background(), os.Getenv("MIRROR_TEST_DATABASE_URL"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { conn.Close(context.Background()) })
	return conn
}

func TestUpsertUpdatesExisting(t *testing.T) {
	conn := testConn(t)
	ctx := context.Background()
	if err := CreateSchema(ctx, conn); err != nil {
		t.Fatal(err)
	}
	e1, e2 := "a@x.com", "b@x.com"
	UpsertCustomers(ctx, conn, []Customer{{ID: "c1", Object: "customer", Email: &e1, Created: 1}})
	UpsertCustomers(ctx, conn, []Customer{{ID: "c1", Object: "customer", Email: &e2, Created: 1}})
	UpsertSubscriptions(ctx, conn, []Subscription{{ID: "s1", Object: "subscription", Customer: "c1", Status: "active", Created: 1}})
	UpsertSubscriptions(ctx, conn, []Subscription{{ID: "s1", Object: "subscription", Customer: "c1", Status: "canceled", Created: 1}})
	var email, status string
	conn.QueryRow(ctx, "SELECT email FROM customers WHERE id='c1'").Scan(&email)
	conn.QueryRow(ctx, "SELECT status FROM subscriptions WHERE id='s1'").Scan(&status)
	if email != "b@x.com" || status != "canceled" {
		t.Fatalf("email=%s status=%s", email, status)
	}
	counts, err := Counts(ctx, conn)
	if err != nil || counts["customers"] != 1 || counts["subscriptions"] != 1 {
		t.Fatalf("%v %v", counts, err)
	}
}

func TestStatusWithoutTables(t *testing.T) {
	testConn(t)
	env := map[string]string{"MIRROR_DATABASE_URL": os.Getenv("MIRROR_TEST_DATABASE_URL")}
	err := run(context.Background(), []string{"status"}, func(k string) string { return env[k] }, &bytes.Buffer{})
	if err == nil || !strings.Contains(err.Error(), "mirror init") {
		t.Fatalf("got %v", err)
	}
}
