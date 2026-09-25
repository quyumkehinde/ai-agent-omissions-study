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

const testKey = "sk_test"

type fakeAPI struct {
	customers []Customer
	subs      []Subscription // includes canceled
	limited   atomic.Int32   // remaining 429s to serve on list endpoints
	srv       *httptest.Server
}

func newFakeAPI(t *testing.T, nCust, nSub int) *fakeAPI {
	f := &fakeAPI{}
	for i := 1; i <= nCust; i++ {
		name := fmt.Sprintf("Customer %d", i)
		email := fmt.Sprintf("u%d@example.com", i)
		c := Customer{ID: fmt.Sprintf("cus_%04d", i), Object: "customer", Email: &email, Name: &name, Created: int64(1000 + i)}
		if i == 2 { // nullable fields
			c.Name, c.Email = nil, nil
		}
		f.customers = append(f.customers, c)
	}
	statuses := []string{"active", "canceled", "past_due", "trialing"}
	for i := 1; i <= nSub; i++ {
		f.subs = append(f.subs, Subscription{ID: fmt.Sprintf("sub_%04d", i), Object: "subscription",
			Customer: "cus_0001", Status: statuses[i%4], Created: int64(2000 + i)})
	}
	f.srv = httptest.NewServer(http.HandlerFunc(f.handle))
	t.Cleanup(f.srv.Close)
	return f
}

func (f *fakeAPI) handle(w http.ResponseWriter, r *http.Request) {
	if r.Header.Get("Authorization") != "Bearer "+testKey {
		w.WriteHeader(401)
		w.Write([]byte(`{"error":{"type":"auth","message":"bad key"}}`))
		return
	}
	if r.URL.Path == "/v1/account" {
		w.Write([]byte(`{"id":"acct","object":"account","livemode":false}`))
		return
	}
	if f.limited.Add(-1) >= 0 {
		w.Header().Set("Retry-After", "1")
		w.WriteHeader(429)
		return
	}
	q := r.URL.Query()
	limit := 10
	if s := q.Get("limit"); s != "" {
		limit, _ = strconv.Atoi(s)
	}
	after := q.Get("starting_after")
	var items []any
	switch r.URL.Path {
	case "/v1/customers":
		for _, c := range f.customers {
			items = append(items, c)
		}
	case "/v1/subscriptions":
		st := q.Get("status")
		for _, s := range f.subs {
			if (st == "" && s.Status == "canceled") || (st != "" && st != "all" && st != s.Status) {
				continue
			}
			items = append(items, s)
		}
	default:
		w.WriteHeader(404)
		return
	}
	start := 0
	if after != "" {
		for i, it := range items {
			b, _ := json.Marshal(it)
			if strings.Contains(string(b), `"id":"`+after+`"`) {
				start = i + 1
			}
		}
	}
	end := min(start+limit, len(items))
	json.NewEncoder(w).Encode(map[string]any{"object": "list", "has_more": end < len(items), "data": items[start:end]})
}

func testClient(f *fakeAPI) *Client {
	c := NewClient(f.srv.URL, testKey)
	c.Sleep = func(context.Context, time.Duration) error { return nil }
	return c
}

// testDB returns a URL pointing at a fresh schema in the test Postgres.
func testDB(t *testing.T) (string, *sql.DB) {
	t.Helper()
	base := os.Getenv("MIRROR_DATABASE_URL")
	if base == "" {
		t.Skip("MIRROR_DATABASE_URL not set")
	}
	schema := fmt.Sprintf("mirror_test_%d", time.Now().UnixNano())
	admin, err := sql.Open("postgres", base)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := admin.Exec("CREATE SCHEMA " + schema); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		admin.Exec("DROP SCHEMA " + schema + " CASCADE")
		admin.Close()
	})
	u, _ := url.Parse(base)
	q := u.Query()
	q.Set("search_path", schema)
	u.RawQuery = q.Encode()
	db, err := sql.Open("postgres", u.String())
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { db.Close() })
	return u.String(), db
}

func env(m map[string]string) func(string) string { return func(k string) string { return m[k] } }

func TestMissingEnv(t *testing.T) {
	cases := []struct {
		env  map[string]string
		want string
	}{
		{map[string]string{"MIRROR_API_KEY": "k"}, "MIRROR_DATABASE_URL"},
		{map[string]string{"MIRROR_DATABASE_URL": "postgres://x"}, "MIRROR_API_KEY"},
		{map[string]string{}, "MIRROR_DATABASE_URL and MIRROR_API_KEY"},
	}
	for _, c := range cases {
		err := run(context.Background(), []string{"init"}, env(c.env), &bytes.Buffer{})
		if err == nil || !strings.Contains(err.Error(), c.want) {
			t.Errorf("env %v: got %v, want mention of %q", c.env, err, c.want)
		}
	}
	err := run(context.Background(), []string{"status"}, env(nil), &bytes.Buffer{})
	if err == nil || !strings.Contains(err.Error(), "MIRROR_DATABASE_URL") {
		t.Errorf("status: got %v", err)
	}
}

func TestInitRejectedKey(t *testing.T) {
	f := newFakeAPI(t, 1, 1)
	err := run(context.Background(), []string{"init"}, env(map[string]string{
		"MIRROR_DATABASE_URL": "postgres://nobody@127.0.0.1:1/x", // must not be reached
		"MIRROR_API_KEY":      "wrong",
		"MIRROR_API_URL":      f.srv.URL,
	}), &bytes.Buffer{})
	if err == nil || !strings.Contains(err.Error(), "MIRROR_API_KEY was rejected") {
		t.Fatalf("got %v", err)
	}
}

func TestPaginationAndRetry(t *testing.T) {
	f := newFakeAPI(t, 250, 0)
	f.limited.Store(3)
	var got []Customer
	err := testClient(f).ListCustomers(context.Background(), func(p []Customer) error {
		got = append(got, p...)
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 250 || got[249].ID != "cus_0250" {
		t.Fatalf("got %d customers", len(got))
	}
}

func TestSubscriptionsIncludeCanceled(t *testing.T) {
	f := newFakeAPI(t, 1, 40)
	var got []Subscription
	err := testClient(f).ListSubscriptions(context.Background(), func(p []Subscription) error {
		got = append(got, p...)
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 40 {
		t.Fatalf("got %d subscriptions, want 40", len(got))
	}
}

func TestInitAndStatusEndToEnd(t *testing.T) {
	f := newFakeAPI(t, 123, 57)
	dbURL, db := testDB(t)
	e := env(map[string]string{"MIRROR_DATABASE_URL": dbURL, "MIRROR_API_KEY": testKey, "MIRROR_API_URL": f.srv.URL})

	var out bytes.Buffer
	for i := 0; i < 2; i++ { // second run proves idempotence
		out.Reset()
		if err := run(context.Background(), []string{"init"}, e, &out); err != nil {
			t.Fatal(err)
		}
	}
	if !strings.Contains(out.String(), "backfilled 123 customers, 57 subscriptions") {
		t.Errorf("output: %q", out.String())
	}

	out.Reset()
	if err := run(context.Background(), []string{"status"}, e, &out); err != nil {
		t.Fatal(err)
	}
	fields := strings.Fields(out.String())
	if strings.Join(fields, " ") != "customers 123 subscriptions 57" {
		t.Errorf("status output: %q", out.String())
	}

	var canceled int
	db.QueryRow("SELECT count(*) FROM subscriptions WHERE status='canceled'").Scan(&canceled)
	if canceled == 0 {
		t.Error("no canceled subscriptions mirrored")
	}
	var name, email sql.NullString
	var created int64
	var object string
	if err := db.QueryRow("SELECT object, email, name, created FROM customers WHERE id='cus_0002'").Scan(&object, &email, &name, &created); err != nil {
		t.Fatal(err)
	}
	if object != "customer" || email.Valid || name.Valid || created != 1002 {
		t.Errorf("customer row: %v %v %v %v", object, email, name, created)
	}
	var cust string
	if err := db.QueryRow("SELECT customer FROM subscriptions WHERE id='sub_0001'").Scan(&cust); err != nil || cust != "cus_0001" {
		t.Errorf("subscription customer: %q %v", cust, err)
	}
}

func TestStatusWithoutInit(t *testing.T) {
	dbURL, _ := testDB(t)
	err := run(context.Background(), []string{"status"}, env(map[string]string{"MIRROR_DATABASE_URL": dbURL}), &bytes.Buffer{})
	if err == nil || !strings.Contains(err.Error(), "mirror init") {
		t.Fatalf("got %v", err)
	}
}
