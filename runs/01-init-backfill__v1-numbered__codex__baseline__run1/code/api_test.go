package main

import (
	"context"
	"net/http"
	"net/http/httptest"
	"net/url"
	"testing"
	"time"
)

func TestSubscriptionsRequestsAllStatusesAndPaginates(t *testing.T) {
	var calls int
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if got := r.Header.Get("Authorization"); got != "Bearer test-key" {
			t.Errorf("authorization = %q", got)
		}
		if r.URL.Query().Get("status") != "all" {
			t.Errorf("status = %q", r.URL.Query().Get("status"))
		}
		if r.URL.Query().Get("limit") != "100" {
			t.Errorf("limit = %q", r.URL.Query().Get("limit"))
		}
		calls++
		if calls == 1 {
			_, _ = w.Write([]byte(`{"has_more":true,"data":[{"id":"sub_1","object":"subscription","customer":"cus_1","status":"active","created":1}]}`))
			return
		}
		if r.URL.Query().Get("starting_after") != "sub_1" {
			t.Errorf("starting_after = %q", r.URL.Query().Get("starting_after"))
		}
		_, _ = w.Write([]byte(`{"has_more":false,"data":[{"id":"sub_2","object":"subscription","customer":"cus_1","status":"canceled","created":2}]}`))
	}))
	defer srv.Close()
	got, err := (&apiClient{baseURL: srv.URL, key: "test-key", client: srv.Client()}).subscriptions(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 2 || got[1].Status != "canceled" {
		t.Fatalf("subscriptions = %#v", got)
	}
}

func TestRateLimitRetriesUsingRetryAfter(t *testing.T) {
	old := sleep
	defer func() { sleep = old }()
	var waited time.Duration
	sleep = func(d time.Duration) { waited = d }
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Query().Get("retry") == "" {
			http.Error(w, "", http.StatusTooManyRequests)
			return
		}
		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()
	// Use a transport that turns the second request into success, while retaining
	// the real HTTP server for URL/request construction.
	n := 0
	c := &http.Client{Transport: roundTripFunc(func(r *http.Request) (*http.Response, error) {
		n++
		if n == 1 {
			return &http.Response{StatusCode: 429, Header: http.Header{"Retry-After": []string{"3"}}, Body: http.NoBody}, nil
		}
		return http.DefaultTransport.RoundTrip(r)
	})}
	resp, err := (&apiClient{baseURL: server.URL, key: "k", client: c}).get(context.Background(), "/", url.Values{"retry": {"yes"}})
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = resp.Body.Close() }()
	if waited != 3*time.Second {
		t.Fatalf("waited %v", waited)
	}
}

type roundTripFunc func(*http.Request) (*http.Response, error)

func (f roundTripFunc) RoundTrip(r *http.Request) (*http.Response, error) { return f(r) }

func TestMissingInitEnvironmentIsNamed(t *testing.T) {
	err := run(context.Background(), []string{"init"}, nil, func(string) string { return "" }, http.DefaultClient)
	if err == nil || err.Error() != "MIRROR_DATABASE_URL is required" {
		t.Fatalf("error = %v", err)
	}
	err = run(context.Background(), []string{"init"}, nil, func(k string) string {
		if k == "MIRROR_DATABASE_URL" {
			return "postgres://x"
		}
		return ""
	}, http.DefaultClient)
	if err == nil || err.Error() != "MIRROR_API_KEY is required" {
		t.Fatalf("error = %v", err)
	}
}
