package main

import (
	"context"
	"net/http"
	"net/http/httptest"
	"reflect"
	"testing"
)

func TestCheckAccountRejectsBadKey(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Authorization") != "Bearer good" {
			w.WriteHeader(http.StatusUnauthorized)
			return
		}
		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()
	c := &apiClient{baseURL: server.URL, apiKey: "bad", httpClient: server.Client()}
	if err := c.checkAccount(context.Background()); err == nil || err.Error() != "API key rejected (HTTP 401)" {
		t.Fatalf("checkAccount() error = %v, want clear rejection", err)
	}
}

func TestEachSubscriptionPaginatesAndRequestsAllStatuses(t *testing.T) {
	var starts []string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Query().Get("status") != "all" || r.URL.Query().Get("limit") != "100" {
			t.Errorf("unexpected query: %s", r.URL.RawQuery)
		}
		starts = append(starts, r.URL.Query().Get("starting_after"))
		w.Header().Set("Content-Type", "application/json")
		if r.URL.Query().Get("starting_after") == "" {
			_, _ = w.Write([]byte(`{"has_more":true,"data":[{"id":"sub_1","object":"subscription","customer":"cus_1","status":"canceled","created":1}]}`))
			return
		}
		_, _ = w.Write([]byte(`{"has_more":false,"data":[{"id":"sub_2","object":"subscription","customer":"cus_2","status":"active","created":2}]}`))
	}))
	defer server.Close()
	c := &apiClient{baseURL: server.URL, apiKey: "key", httpClient: server.Client()}
	var ids []string
	err := c.eachSubscription(context.Background(), func(s subscription) error { ids = append(ids, s.ID); return nil })
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(ids, []string{"sub_1", "sub_2"}) {
		t.Fatalf("ids = %#v", ids)
	}
	if !reflect.DeepEqual(starts, []string{"", "sub_1"}) {
		t.Fatalf("starting_after = %#v", starts)
	}
}
