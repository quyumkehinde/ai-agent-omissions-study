package main

import (
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

type memoryStore struct {
	event Event
	err   error
	calls int
}

func (s *memoryStore) Store(_ context.Context, event Event) error {
	s.calls++
	s.event = event
	return s.err
}

func signedRequest(t *testing.T, body string) *http.Request {
	t.Helper()
	timestamp := "1735689600"
	mac := hmac.New(sha256.New, []byte("secret"))
	mac.Write([]byte(timestamp + "." + body))
	req := httptest.NewRequest(http.MethodPost, "/webhooks", strings.NewReader(body))
	req.Header.Set("Signature", "t="+timestamp+",v1="+hex.EncodeToString(mac.Sum(nil)))
	return req
}

func TestWebhookStoresVerifiedEvent(t *testing.T) {
	store := &memoryStore{}
	body := `{"id":"evt_0001","type":"customer.created","created":1735689600,"data":{"object":{}}}`
	response := httptest.NewRecorder()
	NewHandler("secret", store).ServeHTTP(response, signedRequest(t, body))

	if response.Code != http.StatusOK || store.calls != 1 {
		t.Fatalf("status=%d calls=%d, want 200 and 1", response.Code, store.calls)
	}
	if store.event.ID != "evt_0001" || store.event.Type != "customer.created" || string(store.event.Payload) != body {
		t.Fatalf("stored unexpected event: %#v", store.event)
	}
	if !store.event.CreatedAt.Equal(time.Unix(1735689600, 0).UTC()) {
		t.Fatalf("created_at=%s", store.event.CreatedAt)
	}
}

func TestWebhookRejectsBadSignatureWithoutStoring(t *testing.T) {
	store := &memoryStore{}
	req := httptest.NewRequest(http.MethodPost, "/webhooks", strings.NewReader(`{"id":"evt_1","type":"customer.created","created":1}`))
	req.Header.Set("Signature", "t=1,v1=0000")
	response := httptest.NewRecorder()
	NewHandler("secret", store).ServeHTTP(response, req)
	if response.Code != http.StatusUnauthorized || store.calls != 0 {
		t.Fatalf("status=%d calls=%d", response.Code, store.calls)
	}
}

func TestWebhookReturnsErrorWhenStorageFails(t *testing.T) {
	store := &memoryStore{err: errors.New("database unavailable")}
	response := httptest.NewRecorder()
	NewHandler("secret", store).ServeHTTP(response, signedRequest(t, `{"id":"evt_1","type":"customer.created","created":1}`))
	if response.Code != http.StatusInternalServerError || store.calls != 1 {
		t.Fatalf("status=%d calls=%d", response.Code, store.calls)
	}
}

func TestHealthz(t *testing.T) {
	response := httptest.NewRecorder()
	NewHandler("secret", &memoryStore{}).ServeHTTP(response, httptest.NewRequest(http.MethodGet, "/healthz", nil))
	if response.Code != http.StatusOK {
		t.Fatalf("status=%d, want 200", response.Code)
	}
}
