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
	event   event
	payload []byte
	err     error
	calls   int
}

func (m *memoryStore) Store(_ context.Context, e event, payload []byte) error {
	m.calls++
	m.event, m.payload = e, append([]byte(nil), payload...)
	return m.err
}

func signedHeader(t, body, secret string) string {
	mac := hmac.New(sha256.New, []byte(secret))
	mac.Write([]byte(t + "." + body))
	return "t=" + t + ",v1=" + hex.EncodeToString(mac.Sum(nil))
}

func TestWebhookStoresVerifiedEvent(t *testing.T) {
	const secret = "whsec_test"
	body := `{"id":"evt_0001","object":"event","type":"customer.created","created":1735689600,"data":{"object":{"id":"cus_1"}}}`
	store := &memoryStore{}
	s := &server{secret: []byte(secret), store: store}
	req := httptest.NewRequest(http.MethodPost, "/webhooks", strings.NewReader(body))
	req.Header.Set("Signature", signedHeader("1735689600", body, secret))
	response := httptest.NewRecorder()

	s.routes().ServeHTTP(response, req)
	if response.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200; body = %s", response.Code, response.Body.String())
	}
	if store.calls != 1 || store.event.ID != "evt_0001" || string(store.payload) != body {
		t.Fatalf("store received calls=%d event=%+v payload=%q", store.calls, store.event, store.payload)
	}
}

func TestWebhookRejectsInvalidSignature(t *testing.T) {
	store := &memoryStore{}
	s := &server{secret: []byte("secret"), store: store}
	req := httptest.NewRequest(http.MethodPost, "/webhooks", strings.NewReader(`{"id":"evt_1","type":"customer.created","created":1}`))
	req.Header.Set("Signature", "t=1,v1=deadbeef")
	response := httptest.NewRecorder()

	s.routes().ServeHTTP(response, req)
	if response.Code != http.StatusBadRequest || store.calls != 0 {
		t.Fatalf("status=%d store calls=%d, want 400 and 0", response.Code, store.calls)
	}
}

func TestWebhookOnlyAcknowledgesAfterStorage(t *testing.T) {
	const secret = "secret"
	body := `{"id":"evt_1","type":"customer.created","created":1}`
	store := &memoryStore{err: errors.New("database unavailable")}
	s := &server{secret: []byte(secret), store: store}
	req := httptest.NewRequest(http.MethodPost, "/webhooks", strings.NewReader(body))
	req.Header.Set("Signature", signedHeader("1", body, secret))
	response := httptest.NewRecorder()

	s.routes().ServeHTTP(response, req)
	if response.Code != http.StatusInternalServerError || store.calls != 1 {
		t.Fatalf("status=%d store calls=%d, want 500 and 1", response.Code, store.calls)
	}
}

func TestHealthz(t *testing.T) {
	s := &server{store: &memoryStore{}}
	response := httptest.NewRecorder()
	s.routes().ServeHTTP(response, httptest.NewRequest(http.MethodGet, "/healthz", nil))
	if response.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", response.Code)
	}
}

func TestVerifySignature(t *testing.T) {
	body := []byte(`{"id":"evt"}`)
	header := signedHeader("123", string(body), "secret")
	if !verifySignature(header, body, []byte("secret")) {
		t.Fatal("valid signature was rejected")
	}
	if verifySignature(header, []byte(`{"id":"changed"}`), []byte("secret")) {
		t.Fatal("changed body was accepted")
	}
	if verifySignature("t=not-a-time,v1=00", body, []byte("secret")) {
		t.Fatal("invalid timestamp was accepted")
	}
}
