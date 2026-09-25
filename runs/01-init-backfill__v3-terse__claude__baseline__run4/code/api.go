package main

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"time"
)

const defaultAPIURL = "http://localhost:12111"

// maxRetries bounds how many times a single request is retried after a 429.
const maxRetries = 8

var errUnauthorized = errors.New("API key rejected (401 unauthorized)")

type Customer struct {
	ID      string  `json:"id"`
	Object  string  `json:"object"`
	Email   *string `json:"email"`
	Name    *string `json:"name"`
	Created int64   `json:"created"`
}

type Subscription struct {
	ID       string `json:"id"`
	Object   string `json:"object"`
	Customer string `json:"customer"`
	Status   string `json:"status"`
	Created  int64  `json:"created"`
}

type Client struct {
	BaseURL string
	APIKey  string
	HTTP    *http.Client
	// Sleep is replaceable in tests.
	Sleep func(context.Context, time.Duration) error
}

func NewClient(baseURL, key string) *Client {
	return &Client{BaseURL: baseURL, APIKey: key, HTTP: &http.Client{Timeout: 30 * time.Second}, Sleep: sleepCtx}
}

func sleepCtx(ctx context.Context, d time.Duration) error {
	t := time.NewTimer(d)
	defer t.Stop()
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-t.C:
		return nil
	}
}

// get performs an authenticated GET, honoring 429 Retry-After, and returns the body of a 2xx response.
func (c *Client) get(ctx context.Context, path string, q url.Values) ([]byte, error) {
	u := c.BaseURL + path
	if len(q) > 0 {
		u += "?" + q.Encode()
	}
	for attempt := 0; ; attempt++ {
		req, err := http.NewRequestWithContext(ctx, http.MethodGet, u, nil)
		if err != nil {
			return nil, err
		}
		req.Header.Set("Authorization", "Bearer "+c.APIKey)
		resp, err := c.HTTP.Do(req)
		if err != nil {
			return nil, err
		}
		body, err := io.ReadAll(resp.Body)
		resp.Body.Close()
		if err != nil {
			return nil, err
		}
		switch {
		case resp.StatusCode/100 == 2:
			return body, nil
		case resp.StatusCode == http.StatusUnauthorized:
			return nil, errUnauthorized
		case resp.StatusCode == http.StatusTooManyRequests && attempt < maxRetries:
			wait := time.Second
			if s, err := strconv.Atoi(resp.Header.Get("Retry-After")); err == nil && s >= 0 {
				wait = time.Duration(s) * time.Second
			}
			if err := c.Sleep(ctx, wait); err != nil {
				return nil, err
			}
			continue
		}
		var e struct {
			Error struct{ Type, Message string } `json:"error"`
		}
		if json.Unmarshal(body, &e) == nil && e.Error.Message != "" {
			return nil, fmt.Errorf("GET %s: %s (%d): %s", path, e.Error.Type, resp.StatusCode, e.Error.Message)
		}
		return nil, fmt.Errorf("GET %s: unexpected status %d", path, resp.StatusCode)
	}
}

func (c *Client) CheckAccount(ctx context.Context) error {
	_, err := c.get(ctx, "/v1/account", nil)
	return err
}

type page[T any] struct {
	HasMore bool `json:"has_more"`
	Data    []T  `json:"data"`
}

// listAll pages through a list endpoint, calling fn for each page.
func listAll[T any](ctx context.Context, c *Client, path string, extra url.Values, id func(T) string, fn func([]T) error) error {
	after := ""
	for {
		q := url.Values{"limit": {"100"}}
		for k, v := range extra {
			q[k] = v
		}
		if after != "" {
			q.Set("starting_after", after)
		}
		body, err := c.get(ctx, path, q)
		if err != nil {
			return err
		}
		var p page[T]
		if err := json.Unmarshal(body, &p); err != nil {
			return fmt.Errorf("GET %s: decoding response: %w", path, err)
		}
		if len(p.Data) > 0 {
			if err := fn(p.Data); err != nil {
				return err
			}
		}
		if !p.HasMore {
			return nil
		}
		if len(p.Data) == 0 {
			return fmt.Errorf("GET %s: has_more set but page is empty", path)
		}
		after = id(p.Data[len(p.Data)-1])
	}
}

func (c *Client) ListCustomers(ctx context.Context, fn func([]Customer) error) error {
	return listAll(ctx, c, "/v1/customers", nil, func(x Customer) string { return x.ID }, fn)
}

// ListSubscriptions requests status=all; without it the API omits canceled subscriptions.
func (c *Client) ListSubscriptions(ctx context.Context, fn func([]Subscription) error) error {
	return listAll(ctx, c, "/v1/subscriptions", url.Values{"status": {"all"}}, func(x Subscription) string { return x.ID }, fn)
}
