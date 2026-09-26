package main

import (
	"context"
	"net/http"
	"net/http/httptest"
	"net/url"
	"reflect"
	"testing"
)

type memoryStore struct {
	customers     []customer
	subscriptions []subscription
}

func (m *memoryStore) upsertCustomer(_ context.Context, c customer) error {
	m.customers = append(m.customers, c)
	return nil
}
func (m *memoryStore) upsertSubscription(_ context.Context, s subscription) error {
	m.subscriptions = append(m.subscriptions, s)
	return nil
}

func TestBackfillPaginatesAndIncludesCanceledSubscriptions(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Authorization") != "Bearer test-key" {
			t.Errorf("missing auth")
		}
		switch r.URL.Path {
		case "/v1/customers":
			if r.URL.Query().Get("starting_after") == "cus_1" {
				_, _ = w.Write([]byte(`{"has_more":false,"data":[{"id":"cus_2","object":"customer","email":"b@example.test","name":"B","created":2}]}`))
				return
			}
			_, _ = w.Write([]byte(`{"has_more":true,"data":[{"id":"cus_1","object":"customer","email":"a@example.test","name":"A","created":1}]}`))
		case "/v1/subscriptions":
			if got := r.URL.Query().Get("status"); got != "all" {
				t.Errorf("status = %q, want all", got)
			}
			_, _ = w.Write([]byte(`{"has_more":false,"data":[{"id":"sub_1","object":"subscription","customer":"cus_1","status":"canceled","created":3}]}`))
		}
	}))
	defer server.Close()
	m := &memoryStore{}
	if err := backfill(context.Background(), newAPIClient(server.URL, "test-key"), m); err != nil {
		t.Fatal(err)
	}
	if got, want := []string{m.customers[0].ID, m.customers[1].ID, m.subscriptions[0].Status}, []string{"cus_1", "cus_2", "canceled"}; !reflect.DeepEqual(got, want) {
		t.Fatalf("got %v want %v", got, want)
	}
}

func TestCheckAccountRejectsBadKey(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { w.WriteHeader(http.StatusUnauthorized) }))
	defer server.Close()
	if err := newAPIClient(server.URL, "bad").checkAccount(context.Background()); err == nil {
		t.Fatal("expected rejection")
	}
}

func TestFetchAllRejectsBrokenPagination(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { _, _ = w.Write([]byte(`{"has_more":true,"data":[]}`)) }))
	defer server.Close()
	err := fetchAll(context.Background(), newAPIClient(server.URL, "key"), "/x", url.Values{}, func(customer) error { return nil })
	if err == nil {
		t.Fatal("expected pagination error")
	}
}
