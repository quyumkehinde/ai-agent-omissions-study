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

// maxRetries is how many times a rate-limited request is retried.
const maxRetries = 8

var ErrUnauthorized = errors.New("API key rejected (401 Unauthorized)")

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
	// Sleep is replaceable so tests don't wait on Retry-After.
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

// get performs an authenticated GET, retrying on 429, and returns the body of a 2xx response.
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
		case resp.StatusCode == http.StatusUnauthorized:
			return nil, ErrUnauthorized
		case resp.StatusCode == http.StatusTooManyRequests:
			if attempt >= maxRetries {
				return nil, fmt.Errorf("GET %s: still rate limited after %d retries", path, maxRetries)
			}
			wait := time.Second
			if s, err := strconv.Atoi(resp.Header.Get("Retry-After")); err == nil && s >= 0 {
				wait = time.Duration(s) * time.Second
			}
			if err := c.Sleep(ctx, wait); err != nil {
				return nil, err
			}
			continue
		case resp.StatusCode < 200 || resp.StatusCode > 299:
			var e struct {
				Error struct{ Type, Message string } `json:"error"`
			}
			if json.Unmarshal(body, &e) == nil && e.Error.Message != "" {
				return nil, fmt.Errorf("GET %s: %s (%s)", path, e.Error.Message, e.Error.Type)
			}
			return nil, fmt.Errorf("GET %s: unexpected status %d", path, resp.StatusCode)
		}
		return body, nil
	}
}

// CheckAccount verifies the API key by calling GET /v1/account.
func (c *Client) CheckAccount(ctx context.Context) error {
	body, err := c.get(ctx, "/v1/account", nil)
	if err != nil {
		return err
	}
	var a struct {
		ID string `json:"id"`
	}
	if err := json.Unmarshal(body, &a); err != nil || a.ID == "" {
		return errors.New("GET /v1/account: unexpected response")
	}
	return nil
}

// listAll pages through a list endpoint, calling handle once per page.
func listAll[T any](ctx context.Context, c *Client, path string, extra url.Values, id func(T) string, handle func([]T) error) error {
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
		var page struct {
			HasMore bool `json:"has_more"`
			Data    []T  `json:"data"`
		}
		if err := json.Unmarshal(body, &page); err != nil {
			return fmt.Errorf("GET %s: decoding response: %w", path, err)
		}
		if len(page.Data) > 0 {
			if err := handle(page.Data); err != nil {
				return err
			}
		}
		if !page.HasMore {
			return nil
		}
		if len(page.Data) == 0 {
			return fmt.Errorf("GET %s: has_more is true but page is empty", path)
		}
		after = id(page.Data[len(page.Data)-1])
	}
}

func (c *Client) ListCustomers(ctx context.Context, handle func([]Customer) error) error {
	return listAll(ctx, c, "/v1/customers", nil, func(x Customer) string { return x.ID }, handle)
}

// ListSubscriptions includes canceled subscriptions, which the API omits by default.
func (c *Client) ListSubscriptions(ctx context.Context, handle func([]Subscription) error) error {
	return listAll(ctx, c, "/v1/subscriptions", url.Values{"status": {"all"}}, func(x Subscription) string { return x.ID }, handle)
}
