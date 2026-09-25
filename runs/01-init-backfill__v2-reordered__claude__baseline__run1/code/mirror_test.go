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
	"testing"
	"time"
)

const testKey = "sk_test_key"

// fakeAPI serves 250 customers and subscriptions (every 5th canceled), paginated,
// with one 429 on the first customers request.
func fakeAPI(t *testing.T) *httptest.Server {
	t.Helper()
	limited := false
	mux := http.NewServeMux()
	mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Authorization") != "Bearer "+testKey {
			w.WriteHeader(401)
			fmt.Fprint(w, `{"error":{"type":"auth","message":"bad key"}}`)
			return
		}
		switch r.URL.Path {
		case "/v1/account":
			fmt.Fprint(w, `{"id":"acct_x","object":"account","livemode":false}`)
		case "/v1/customers":
			if !limited {
				limited = true
				w.Header().Set("Retry-After", "2")
				w.WriteHeader(429)
				return
			}
			serveList(w, r, 250, func(i int) any {
				c := map[string]any{"id": fmt.Sprintf("cus_%03d", i), "object": "customer", "created": 1700000000 + i,
					"email": fmt.Sprintf("u%d@example.com", i), "name": fmt.Sprintf("User %d", i)}
				if i == 0 {
					c["name"], c["email"] = nil, nil
				}
				return c
			}, false)
		case "/v1/subscriptions":
			serveList(w, r, 250, func(i int) any {
				st := "active"
				if i%5 == 0 {
					st = "canceled"
				}
				return map[string]any{"id": fmt.Sprintf("sub_%03d", i), "object": "subscription",
					"customer": fmt.Sprintf("cus_%03d", i), "status": st, "created": 1700000000 + i}
			}, true)
		default:
			w.WriteHeader(404)
		}
	})
	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)
	return srv
}

func serveList(w http.ResponseWriter, r *http.Request, total int, mk func(int) any, isSubs bool) {
	q := r.URL.Query()
	limit := 10
	if v := q.Get("limit"); v != "" {
		limit, _ = strconv.Atoi(v)
	}
	start := 0
	if a := q.Get("starting_after"); a != "" {
		n, _ := strconv.Atoi(a[strings.Index(a, "_")+1:])
		start = n + 1
	}
	all := q.Get("status") == "all"
	var data []any
	i := start
	for ; i < total && len(data) < limit; i++ {
		item := mk(i)
		if isSubs && !all && item.(map[string]any)["status"] == "canceled" {
			continue
		}
		data = append(data, item)
	}
	hasMore := false
	for j := i; j < total; j++ {
		if !isSubs || all || mk(j).(map[string]any)["status"] != "canceled" {
			hasMore = true
			break
		}
	}
	json.NewEncoder(w).Encode(map[string]any{"object": "list", "has_more": hasMore, "data": data})
}

func TestRequireEnv(t *testing.T) {
	env := map[string]string{"MIRROR_API_KEY": "k"}
	err := run(context.Background(), []string{"init"}, func(k string) string { return env[k] }, &bytes.Buffer{})
	if err == nil || !strings.Contains(err.Error(), "MIRROR_DATABASE_URL") || strings.Contains(err.Error(), "MIRROR_API_KEY") {
		t.Fatalf("want error naming only MIRROR_DATABASE_URL, got %v", err)
	}
	env = map[string]string{"MIRROR_DATABASE_URL": "x"}
	err = run(context.Background(), []string{"status"}, func(k string) string { return env[k] }, &bytes.Buffer{})
	if err == nil || !strings.Contains(err.Error(), "MIRROR_API_KEY") {
		t.Fatalf("want error naming MIRROR_API_KEY, got %v", err)
	}
}

func TestUsage(t *testing.T) {
	if err := run(context.Background(), []string{"bogus"}, func(string) string { return "x" }, &bytes.Buffer{}); err == nil {
		t.Fatal("want usage error")
	}
}

func TestClientPagination(t *testing.T) {
	srv := fakeAPI(t)
	c := NewClient(srv.URL, testKey)
	var slept []time.Duration
	c.Sleep = func(d time.Duration) { slept = append(slept, d) }

	nc := 0
	if err := c.ListCustomers(context.Background(), func(cs []Customer) error { nc += len(cs); return nil }); err != nil {
		t.Fatal(err)
	}
	if nc != 250 {
		t.Errorf("customers = %d, want 250", nc)
	}
	if len(slept) != 1 || slept[0] != 2*time.Second {
		t.Errorf("slept = %v, want [2s]", slept)
	}
	ns, canceled := 0, 0
	if err := c.ListSubscriptions(context.Background(), func(ss []Subscription) error {
		for _, s := range ss {
			ns++
			if s.Status == "canceled" {
				canceled++
			}
		}
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	if ns != 250 || canceled != 50 {
		t.Errorf("subs = %d (canceled %d), want 250 (50)", ns, canceled)
	}
}

func TestInitRejectedKey(t *testing.T) {
	srv := fakeAPI(t)
	err := runInit(context.Background(), &bytes.Buffer{}, "postgres://invalid.invalid/none", NewClient(srv.URL, "wrong"))
	if err == nil || !strings.Contains(err.Error(), "rejected") {
		t.Fatalf("want rejected-key error, got %v", err)
	}
}

// testDSN returns a DSN scoped to a fresh schema, dropped on cleanup.
func testDSN(t *testing.T) string {
	t.Helper()
	base := os.Getenv("MIRROR_DATABASE_URL")
	if base == "" {
		t.Skip("MIRROR_DATABASE_URL not set")
	}
	name := fmt.Sprintf("mirror_test_%d", time.Now().UnixNano())
	db, err := sql.Open("postgres", base)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec("CREATE SCHEMA " + name); err != nil {
		t.Skipf("cannot reach Postgres: %v", err)
	}
	t.Cleanup(func() { db.Exec("DROP SCHEMA " + name + " CASCADE"); db.Close() })
	u, err := url.Parse(base)
	if err != nil {
		t.Fatal(err)
	}
	q := u.Query()
	q.Set("search_path", name)
	u.RawQuery = q.Encode()
	return u.String()
}

func TestInitAndStatus(t *testing.T) {
	dsn := testDSN(t)
	srv := fakeAPI(t)
	c := NewClient(srv.URL, testKey)
	c.Sleep = func(time.Duration) {}

	var out bytes.Buffer
	for i := 0; i < 2; i++ { // second run must be idempotent
		if err := runInit(context.Background(), &out, dsn, c); err != nil {
			t.Fatal(err)
		}
	}
	out.Reset()
	if err := runStatus(context.Background(), &out, dsn); err != nil {
		t.Fatal(err)
	}
	if got, want := out.String(), "customers: 250\nsubscriptions: 250\n"; got != want {
		t.Errorf("status = %q, want %q", got, want)
	}

	st, err := OpenStore(context.Background(), dsn)
	if err != nil {
		t.Fatal(err)
	}
	defer st.Close()
	var email, name sql.NullString
	var created int64
	if err := st.db.QueryRow("SELECT email, name, created FROM customers WHERE id='cus_000'").Scan(&email, &name, &created); err != nil {
		t.Fatal(err)
	}
	if email.Valid || name.Valid || created != 1700000000 {
		t.Errorf("cus_000 = %v %v %d", email, name, created)
	}
	var status string
	if err := st.db.QueryRow("SELECT status FROM subscriptions WHERE id='sub_005'").Scan(&status); err != nil || status != "canceled" {
		t.Errorf("sub_005 status = %q, err %v", status, err)
	}
}

func TestStatusBeforeInit(t *testing.T) {
	dsn := testDSN(t)
	err := runStatus(context.Background(), &bytes.Buffer{}, dsn)
	if err == nil || !strings.Contains(err.Error(), "mirror init") {
		t.Fatalf("got %v", err)
	}
}
