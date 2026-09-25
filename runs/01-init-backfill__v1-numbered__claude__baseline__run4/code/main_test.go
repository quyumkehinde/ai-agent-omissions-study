package main

import (
	"bytes"
	"context"
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

type fakeSink struct {
	schema bool
	cust   map[string]Customer
	subs   map[string]Subscription
}

func newFakeSink() *fakeSink {
	return &fakeSink{cust: map[string]Customer{}, subs: map[string]Subscription{}}
}
func (f *fakeSink) CreateSchema(context.Context) error { f.schema = true; return nil }
func (f *fakeSink) UpsertCustomers(_ context.Context, cs []Customer) error {
	for _, c := range cs {
		f.cust[c.ID] = c
	}
	return nil
}
func (f *fakeSink) UpsertSubscriptions(_ context.Context, ss []Subscription) error {
	for _, s := range ss {
		f.subs[s.ID] = s
	}
	return nil
}
func (f *fakeSink) Count(_ context.Context, t string) (int64, error) {
	if t == "customers" {
		return int64(len(f.cust)), nil
	}
	return int64(len(f.subs)), nil
}

// fakeAPI serves 250 customers and 120 subscriptions (every 4th canceled).
func fakeAPI(t *testing.T, limited *atomic.Int32) *httptest.Server {
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Authorization") != "Bearer good" {
			w.WriteHeader(401)
			w.Write([]byte(`{"error":{"type":"auth","message":"bad key"}}`))
			return
		}
		if r.URL.Path == "/v1/account" {
			w.Write([]byte(`{"id":"acct","object":"account","livemode":false}`))
			return
		}
		if limited != nil && limited.Add(-1) >= 0 {
			w.Header().Set("Retry-After", "2")
			w.WriteHeader(429)
			return
		}
		var all []any
		var ids []string
		switch r.URL.Path {
		case "/v1/customers":
			for i := 0; i < 250; i++ {
				id := fmt.Sprintf("cus_%04d", i)
				ids = append(ids, id)
				all = append(all, Customer{ID: id, Object: "customer", Email: id + "@x.io", Name: "N", Created: int64(i)})
			}
		case "/v1/subscriptions":
			for i := 0; i < 120; i++ {
				st := "active"
				if i%4 == 0 {
					st = "canceled"
					if r.URL.Query().Get("status") != "all" {
						continue
					}
				}
				id := fmt.Sprintf("sub_%04d", i)
				ids = append(ids, id)
				all = append(all, Subscription{ID: id, Object: "subscription", Customer: "cus_0000", Status: st, Created: int64(i)})
			}
		default:
			w.WriteHeader(404)
			return
		}
		limit, _ := strconv.Atoi(r.URL.Query().Get("limit"))
		if limit == 0 {
			limit = 10
		}
		start := 0
		if a := r.URL.Query().Get("starting_after"); a != "" {
			for i, id := range ids {
				if id == a {
					start = i + 1
				}
			}
		}
		end := min(start+limit, len(all))
		json.NewEncoder(w).Encode(map[string]any{"object": "list", "has_more": end < len(all), "data": all[start:end]})
	}))
}

func testClient(url, key string) *Client {
	c := NewClient(url, key)
	c.Sleep = func(time.Duration) {}
	return c
}

func TestInitImportsEverythingIncludingCanceled(t *testing.T) {
	srv := fakeAPI(t, nil)
	defer srv.Close()
	s := newFakeSink()
	var out bytes.Buffer
	if err := runInit(context.Background(), testClient(srv.URL, "good"), s, &out); err != nil {
		t.Fatal(err)
	}
	if !s.schema || len(s.cust) != 250 || len(s.subs) != 120 {
		t.Fatalf("schema=%v customers=%d subs=%d", s.schema, len(s.cust), len(s.subs))
	}
	if s.subs["sub_0000"].Status != "canceled" {
		t.Fatal("canceled subscription missing")
	}
	var st bytes.Buffer
	if err := runStatus(context.Background(), s, &st); err != nil {
		t.Fatal(err)
	}
	if st.String() != "customers: 250\nsubscriptions: 120\n" {
		t.Fatalf("status output %q", st.String())
	}
}

func TestRetriesOn429(t *testing.T) {
	var n atomic.Int32
	n.Store(3)
	srv := fakeAPI(t, &n)
	defer srv.Close()
	c := NewClient(srv.URL, "good")
	var slept []time.Duration
	c.Sleep = func(d time.Duration) { slept = append(slept, d) }
	s := newFakeSink()
	if err := runInit(context.Background(), c, s, &bytes.Buffer{}); err != nil {
		t.Fatal(err)
	}
	if len(s.cust) != 250 || len(slept) != 3 || slept[0] != 2*time.Second {
		t.Fatalf("customers=%d slept=%v", len(s.cust), slept)
	}
}

func TestBadKey(t *testing.T) {
	srv := fakeAPI(t, nil)
	defer srv.Close()
	_, err := testClient(srv.URL, "bad").Account(context.Background())
	if !errors.Is(err, ErrUnauthorized) {
		t.Fatalf("got %v", err)
	}
}

func TestRunMissingEnv(t *testing.T) {
	env := map[string]string{}
	get := func(k string) string { return env[k] }
	err := run(context.Background(), []string{"init"}, get, "", nil)
	if err == nil || !strings.Contains(err.Error(), "MIRROR_DATABASE_URL") || !strings.Contains(err.Error(), "MIRROR_API_KEY") {
		t.Fatalf("got %v", err)
	}
	env["MIRROR_DATABASE_URL"] = "x"
	err = run(context.Background(), []string{"status"}, get, "", nil)
	if err == nil || !strings.Contains(err.Error(), "MIRROR_API_KEY") || strings.Contains(err.Error(), "MIRROR_DATABASE_URL") {
		t.Fatalf("got %v", err)
	}
}

func TestRunRejectedKeyBeforeDB(t *testing.T) {
	srv := fakeAPI(t, nil)
	defer srv.Close()
	env := map[string]string{"MIRROR_DATABASE_URL": "postgres://invalid.invalid/x", "MIRROR_API_KEY": "bad"}
	err := run(context.Background(), []string{"init"}, func(k string) string { return env[k] }, srv.URL, nil)
	if !errors.Is(err, ErrUnauthorized) {
		t.Fatalf("got %v", err)
	}
}

func TestUsage(t *testing.T) {
	if err := run(context.Background(), []string{"bogus"}, os.Getenv, "", nil); err == nil {
		t.Fatal("expected usage error")
	}
}

// Real-Postgres test; set MIRROR_TEST_DATABASE_URL to run (it drops the mirror tables).
func TestStorePostgres(t *testing.T) {
	dsn := os.Getenv("MIRROR_TEST_DATABASE_URL")
	if dsn == "" {
		t.Skip("MIRROR_TEST_DATABASE_URL not set")
	}
	ctx := context.Background()
	st, err := OpenStore(ctx, dsn)
	if err != nil {
		t.Fatal(err)
	}
	defer st.Close()
	if _, err := st.pool.Exec(ctx, "DROP TABLE IF EXISTS customers, subscriptions"); err != nil {
		t.Fatal(err)
	}
	srv := fakeAPI(t, nil)
	defer srv.Close()
	for i := 0; i < 2; i++ { // second run proves idempotency
		if err := runInit(ctx, testClient(srv.URL, "good"), st, &bytes.Buffer{}); err != nil {
			t.Fatal(err)
		}
	}
	var out bytes.Buffer
	if err := runStatus(ctx, st, &out); err != nil {
		t.Fatal(err)
	}
	if out.String() != "customers: 250\nsubscriptions: 120\n" {
		t.Fatalf("got %q", out.String())
	}
}
