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
)

type memoryStore struct {
	events []Event
	err    error
}

func (s *memoryStore) StoreEvent(_ context.Context, event Event) error {
	if s.err != nil {
		return s.err
	}
	s.events = append(s.events, event)
	return nil
}

func signedRequest(t *testing.T, secret, body string) *http.Request {
	t.Helper()
	timestamp := "1735689600"
	mac := hmac.New(sha256.New, []byte(secret))
	_, _ = mac.Write([]byte(timestamp + "." + body))
	req := httptest.NewRequest(http.MethodPost, "/webhooks", strings.NewReader(body))
	req.Header.Set("Signature", "t="+timestamp+",v1="+hex.EncodeToString(mac.Sum(nil)))
	return req
}

func TestHealthz(t *testing.T) {
	rr := httptest.NewRecorder()
	NewHandler("secret", &memoryStore{}).ServeHTTP(rr, httptest.NewRequest(http.MethodGet, "/healthz", nil))
	if rr.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", rr.Code)
	}
}

func TestWebhookStoresVerifiedEvent(t *testing.T) {
	store := &memoryStore{}
	body := `{"id":"evt_0001","type":"customer.created","created":1735689600,"data":{"object":{"id":"cus_1"}}}`
	rr := httptest.NewRecorder()
	NewHandler("top-secret", store).ServeHTTP(rr, signedRequest(t, "top-secret", body))
	if rr.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200; body: %s", rr.Code, rr.Body.String())
	}
	if len(store.events) != 1 {
		t.Fatalf("stored events = %d, want 1", len(store.events))
	}
	event := store.events[0]
	if event.ID != "evt_0001" || event.Type != "customer.created" || event.Created != 1735689600 || string(event.Payload) != body {
		t.Fatalf("stored event = %#v", event)
	}
}

func TestWebhookRejectsInvalidSignature(t *testing.T) {
	store := &memoryStore{}
	body := `{"id":"evt_0001","type":"customer.created","created":1735689600}`
	req := signedRequest(t, "right-secret", body)
	rr := httptest.NewRecorder()
	NewHandler("wrong-secret", store).ServeHTTP(rr, req)
	if rr.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400", rr.Code)
	}
	if len(store.events) != 0 {
		t.Fatal("event was stored despite an invalid signature")
	}
}

func TestWebhookOnlyAcknowledgesAfterStorage(t *testing.T) {
	body := `{"id":"evt_0001","type":"customer.created","created":1735689600}`
	rr := httptest.NewRecorder()
	NewHandler("secret", &memoryStore{err: errors.New("database unavailable")}).ServeHTTP(rr, signedRequest(t, "secret", body))
	if rr.Code != http.StatusInternalServerError {
		t.Fatalf("status = %d, want 500", rr.Code)
	}
}
