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

const pageLimit = 100

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
	return &Client{
		BaseURL: baseURL,
		APIKey:  key,
		HTTP:    &http.Client{Timeout: 30 * time.Second},
		Sleep:   sleepCtx,
	}
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

// get performs an authenticated GET, retrying on 429, and decodes JSON into out.
func (c *Client) get(ctx context.Context, path string, q url.Values, out any) error {
	u := c.BaseURL + path
	if len(q) > 0 {
		u += "?" + q.Encode()
	}
	for attempt := 0; ; attempt++ {
		req, err := http.NewRequestWithContext(ctx, http.MethodGet, u, nil)
		if err != nil {
			return err
		}
		req.Header.Set("Authorization", "Bearer "+c.APIKey)
		resp, err := c.HTTP.Do(req)
		if err != nil {
			return fmt.Errorf("GET %s: %w", path, err)
		}
		body, err := io.ReadAll(resp.Body)
		resp.Body.Close()
		if err != nil {
			return fmt.Errorf("GET %s: reading body: %w", path, err)
		}
		switch {
		case resp.StatusCode == http.StatusUnauthorized:
			return errUnauthorized
		case resp.StatusCode == http.StatusTooManyRequests:
			if attempt >= maxRetries {
				return fmt.Errorf("GET %s: still rate limited after %d retries", path, maxRetries)
			}
			wait := time.Second
			if s, err := strconv.Atoi(resp.Header.Get("Retry-After")); err == nil && s >= 0 {
				wait = time.Duration(s) * time.Second
			}
			if err := c.Sleep(ctx, wait); err != nil {
				return err
			}
			continue
		case resp.StatusCode < 200 || resp.StatusCode > 299:
			return fmt.Errorf("GET %s: %s", path, apiError(resp.StatusCode, body))
		}
		if err := json.Unmarshal(body, out); err != nil {
			return fmt.Errorf("GET %s: decoding response: %w", path, err)
		}
		return nil
	}
}

func apiError(status int, body []byte) string {
	var e struct {
		Error struct {
			Type    string `json:"type"`
			Message string `json:"message"`
		} `json:"error"`
	}
	if json.Unmarshal(body, &e) == nil && e.Error.Message != "" {
		return fmt.Sprintf("HTTP %d: %s (%s)", status, e.Error.Message, e.Error.Type)
	}
	return fmt.Sprintf("HTTP %d", status)
}

// CheckAccount verifies the API key.
func (c *Client) CheckAccount(ctx context.Context) error {
	var acct struct {
		ID string `json:"id"`
	}
	return c.get(ctx, "/v1/account", nil, &acct)
}

type page[T any] struct {
	HasMore bool `json:"has_more"`
	Data    []T  `json:"data"`
}

// paginate walks a list endpoint, calling fn for each page.
func paginate[T any](ctx context.Context, c *Client, path string, extra url.Values, id func(T) string, fn func([]T) error) error {
	after := ""
	for {
		q := url.Values{"limit": {strconv.Itoa(pageLimit)}}
		for k, v := range extra {
			q[k] = v
		}
		if after != "" {
			q.Set("starting_after", after)
		}
		var p page[T]
		if err := c.get(ctx, path, q, &p); err != nil {
			return err
		}
		if len(p.Data) > 0 {
			if err := fn(p.Data); err != nil {
				return err
			}
			after = id(p.Data[len(p.Data)-1])
		}
		if !p.HasMore {
			return nil
		}
		if len(p.Data) == 0 {
			return fmt.Errorf("GET %s: has_more is true but page is empty", path)
		}
	}
}

func (c *Client) EachCustomerPage(ctx context.Context, fn func([]Customer) error) error {
	return paginate(ctx, c, "/v1/customers", nil, func(x Customer) string { return x.ID }, fn)
}

// EachSubscriptionPage includes canceled subscriptions (status=all); the API
// omits them by default.
func (c *Client) EachSubscriptionPage(ctx context.Context, fn func([]Subscription) error) error {
	return paginate(ctx, c, "/v1/subscriptions", url.Values{"status": {"all"}}, func(x Subscription) string { return x.ID }, fn)
}
