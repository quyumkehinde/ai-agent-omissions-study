package main

import (
	"bytes"
	"context"
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

	"github.com/jackc/pgx/v5"
)

const testKey = "sk_test"

// fakeAPI serves nCust customers and nSub subscriptions (every 3rd canceled),
// hiding canceled ones unless status=all, and rate-limits the first list call.
func fakeAPI(t *testing.T, nCust, nSub int) *httptest.Server {
	t.Helper()
	var limited atomic.Bool
	page := func(w http.ResponseWriter, r *http.Request, ids []string, obj func(i int) string) {
		limit, _ := strconv.Atoi(r.URL.Query().Get("limit"))
		if limit < 1 || limit > 100 {
			limit = 10
		}
		start := 0
		if after := r.URL.Query().Get("starting_after"); after != "" {
			for i, id := range ids {
				if id == after {
					start = i + 1
				}
			}
		}
		end := min(start+limit, len(ids))
		var items []string
		for i := start; i < end; i++ {
			items = append(items, obj(i))
		}
		fmt.Fprintf(w, `{"object":"list","has_more":%v,"data":[%s]}`, end < len(ids), strings.Join(items, ","))
	}
	mux := http.NewServeMux()
	auth := func(h http.HandlerFunc) http.HandlerFunc {
		return func(w http.ResponseWriter, r *http.Request) {
			if r.Header.Get("Authorization") != "Bearer "+testKey {
				w.WriteHeader(401)
				fmt.Fprint(w, `{"error":{"type":"authentication_error","message":"Invalid API key provided."}}`)
				return
			}
			h(w, r)
		}
	}
	mux.HandleFunc("/v1/account", auth(func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprint(w, `{"id":"acct_x","object":"account","livemode":false}`)
	}))
	mux.HandleFunc("/v1/customers", auth(func(w http.ResponseWriter, r *http.Request) {
		if limited.CompareAndSwap(false, true) {
			w.Header().Set("Retry-After", "1")
			w.WriteHeader(429)
			return
		}
		var ids []string
		for i := 0; i < nCust; i++ {
			ids = append(ids, fmt.Sprintf("cus_%04d", i))
		}
		page(w, r, ids, func(i int) string {
			if i%2 == 0 { // null name/email must round-trip
				return fmt.Sprintf(`{"id":%q,"object":"customer","email":null,"name":null,"created":%d}`, ids[i], 1700000000+i)
			}
			return fmt.Sprintf(`{"id":%q,"object":"customer","email":"a%d@x.io","name":"N%d","created":%d}`, ids[i], i, i, 1700000000+i)
		})
	}))
	mux.HandleFunc("/v1/subscriptions", auth(func(w http.ResponseWriter, r *http.Request) {
		all := r.URL.Query().Get("status") == "all"
		var ids []string
		var status []string
		for i := 0; i < nSub; i++ {
			st := "active"
			if i%3 == 2 {
				st = "canceled"
				if !all {
					continue
				}
			}
			ids = append(ids, fmt.Sprintf("sub_%04d", i))
			status = append(status, st)
		}
		page(w, r, ids, func(i int) string {
			return fmt.Sprintf(`{"id":%q,"object":"subscription","customer":"cus_0000","status":%q,"created":%d}`, ids[i], status[i], 1700000000+i)
		})
	}))
	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)
	return srv
}

func testClient(url, key string) *Client {
	c := NewClient(url, key)
	c.Sleep = func(context.Context, time.Duration) error { return nil }
	return c
}

func TestCheckAccount(t *testing.T) {
	srv := fakeAPI(t, 0, 0)
	if err := testClient(srv.URL, testKey).CheckAccount(context.Background()); err != nil {
		t.Fatal(err)
	}
	err := testClient(srv.URL, "bad").CheckAccount(context.Background())
	if err == nil || !strings.Contains(err.Error(), "rejected") {
		t.Fatalf("want rejected error, got %v", err)
	}
}

func TestListPaginatesRetriesAndIncludesCanceled(t *testing.T) {
	srv := fakeAPI(t, 250, 30)
	c := testClient(srv.URL, testKey)
	var custs, subs, canceled int
	if err := c.ListCustomers(context.Background(), func(p []Customer) error { custs += len(p); return nil }); err != nil {
		t.Fatal(err)
	}
	if err := c.ListSubscriptions(context.Background(), func(p []Subscription) error {
		for _, s := range p {
			subs++
			if s.Status == "canceled" {
				canceled++
			}
		}
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	if custs != 250 || subs != 30 || canceled != 10 {
		t.Fatalf("customers=%d subs=%d canceled=%d", custs, subs, canceled)
	}
}

func TestRateLimitGivesUp(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Retry-After", "0")
		w.WriteHeader(429)
	}))
	defer srv.Close()
	err := testClient(srv.URL, testKey).ListCustomers(context.Background(), func([]Customer) error { return nil })
	if err == nil || !strings.Contains(err.Error(), "rate limited") {
		t.Fatalf("got %v", err)
	}
}

func TestMissingEnv(t *testing.T) {
	for _, tc := range []struct {
		args    []string
		env     map[string]string
		wantErr string
	}{
		{[]string{"init"}, map[string]string{"MIRROR_API_KEY": "k"}, "MIRROR_DATABASE_URL"},
		{[]string{"init"}, map[string]string{"MIRROR_DATABASE_URL": "u"}, "MIRROR_API_KEY"},
		{[]string{"status"}, nil, "MIRROR_DATABASE_URL"},
		{[]string{"bogus"}, nil, "unknown command"},
		{nil, nil, "usage"},
	} {
		err := run(context.Background(), tc.args, func(k string) string { return tc.env[k] }, &bytes.Buffer{})
		if err == nil || !strings.Contains(err.Error(), tc.wantErr) {
			t.Errorf("%v: want %q, got %v", tc.args, tc.wantErr, err)
		}
	}
}

func TestInitRejectedKeyDoesNotTouchDB(t *testing.T) {
	srv := fakeAPI(t, 0, 0)
	// Unreachable DB: if init connected first, the error would be about the database.
	err := runInit(context.Background(), testClient(srv.URL, "bad"), "postgres://nobody@127.0.0.1:1/x", &bytes.Buffer{})
	if err == nil || !strings.Contains(err.Error(), "MIRROR_API_KEY") {
		t.Fatalf("got %v", err)
	}
}

// testDBURL creates an isolated schema and returns a URL whose search_path points at it.
func testDBURL(t *testing.T) string {
	base := os.Getenv("MIRROR_DATABASE_URL")
	if base == "" {
		t.Skip("MIRROR_DATABASE_URL not set")
	}
	ctx := context.Background()
	db, err := pgx.Connect(ctx, base)
	if err != nil {
		t.Skipf("postgres unavailable: %v", err)
	}
	schemaName := fmt.Sprintf("mirror_test_%d", time.Now().UnixNano())
	if _, err := db.Exec(ctx, "CREATE SCHEMA "+schemaName); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		db.Exec(ctx, "DROP SCHEMA "+schemaName+" CASCADE")
		db.Close(ctx)
	})
	u, _ := url.Parse(base)
	q := u.Query()
	q.Set("search_path", schemaName)
	u.RawQuery = q.Encode()
	return u.String()
}

func TestInitAndStatusEndToEnd(t *testing.T) {
	dbURL := testDBURL(t)
	srv := fakeAPI(t, 120, 30)
	ctx := context.Background()

	// status before init: helpful error
	if err := runStatus(ctx, dbURL, &bytes.Buffer{}); err == nil || !strings.Contains(err.Error(), "mirror init") {
		t.Fatalf("want init hint, got %v", err)
	}

	var out bytes.Buffer
	for i := 0; i < 2; i++ { // second run proves idempotence
		out.Reset()
		if err := runInit(ctx, testClient(srv.URL, testKey), dbURL, &out); err != nil {
			t.Fatal(err)
		}
	}
	out.Reset()
	if err := runStatus(ctx, dbURL, &out); err != nil {
		t.Fatal(err)
	}
	got := strings.Fields(out.String())
	want := []string{"customers", "120", "subscriptions", "30"}
	if strings.Join(got, " ") != strings.Join(want, " ") {
		t.Fatalf("status output %q", out.String())
	}

	db, _ := pgx.Connect(ctx, dbURL)
	defer db.Close(ctx)
	var n int
	db.QueryRow(ctx, "SELECT count(*) FROM subscriptions WHERE status='canceled'").Scan(&n)
	if n != 10 {
		t.Errorf("canceled subscriptions = %d, want 10", n)
	}
	db.QueryRow(ctx, "SELECT count(*) FROM customers WHERE email IS NULL AND name IS NULL").Scan(&n)
	if n != 60 {
		t.Errorf("null customers = %d, want 60", n)
	}
}
