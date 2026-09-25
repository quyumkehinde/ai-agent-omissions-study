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

const maxRateLimitRetries = 8

// ErrUnauthorized is returned when the API rejects the key.
var ErrUnauthorized = errors.New("API key was rejected (401 unauthorized)")

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

type Account struct {
	ID       string `json:"id"`
	Object   string `json:"object"`
	Livemode bool   `json:"livemode"`
}

type listResponse[T any] struct {
	HasMore bool `json:"has_more"`
	Data    []T  `json:"data"`
}

type Client struct {
	BaseURL string
	APIKey  string
	HTTP    *http.Client
	// Sleep waits between rate-limit retries; replaced in tests.
	Sleep func(ctx context.Context, d time.Duration) error
}

func NewClient(baseURL, apiKey string) *Client {
	return &Client{
		BaseURL: baseURL,
		APIKey:  apiKey,
		HTTP:    &http.Client{Timeout: 30 * time.Second},
		Sleep: func(ctx context.Context, d time.Duration) error {
			t := time.NewTimer(d)
			defer t.Stop()
			select {
			case <-ctx.Done():
				return ctx.Err()
			case <-t.C:
				return nil
			}
		},
	}
}

// get performs a GET, retrying on 429 (honoring Retry-After), and decodes JSON into out.
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
			return fmt.Errorf("GET %s: reading response: %w", path, err)
		}

		switch {
		case resp.StatusCode == http.StatusUnauthorized:
			return ErrUnauthorized
		case resp.StatusCode == http.StatusTooManyRequests:
			if attempt >= maxRateLimitRetries {
				return fmt.Errorf("GET %s: still rate limited after %d retries", path, attempt)
			}
			wait := time.Second
			if s, err := strconv.ParseFloat(resp.Header.Get("Retry-After"), 64); err == nil && s >= 0 {
				wait = time.Duration(s * float64(time.Second))
			}
			if err := c.Sleep(ctx, wait); err != nil {
				return err
			}
			continue
		case resp.StatusCode < 200 || resp.StatusCode > 299:
			return fmt.Errorf("GET %s: %s", path, apiErrorMessage(resp.StatusCode, body))
		}
		if err := json.Unmarshal(body, out); err != nil {
			return fmt.Errorf("GET %s: decoding response: %w", path, err)
		}
		return nil
	}
}

func apiErrorMessage(status int, body []byte) string {
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

func (c *Client) Account(ctx context.Context) (*Account, error) {
	var a Account
	if err := c.get(ctx, "/v1/account", nil, &a); err != nil {
		return nil, err
	}
	return &a, nil
}

// paginate walks a list endpoint, calling fn for each page in order.
func paginate[T any](ctx context.Context, c *Client, path string, extra url.Values, id func(T) string, fn func([]T) error) error {
	after := ""
	for {
		q := url.Values{"limit": {"100"}}
		for k, v := range extra {
			q[k] = v
		}
		if after != "" {
			q.Set("starting_after", after)
		}
		var page listResponse[T]
		if err := c.get(ctx, path, q, &page); err != nil {
			return err
		}
		if len(page.Data) > 0 {
			if err := fn(page.Data); err != nil {
				return err
			}
			after = id(page.Data[len(page.Data)-1])
		}
		if !page.HasMore || len(page.Data) == 0 {
			return nil
		}
	}
}

func (c *Client) EachCustomerPage(ctx context.Context, fn func([]Customer) error) error {
	return paginate(ctx, c, "/v1/customers", nil, func(x Customer) string { return x.ID }, fn)
}

// EachSubscriptionPage includes canceled subscriptions (status=all); the API omits them by default.
func (c *Client) EachSubscriptionPage(ctx context.Context, fn func([]Subscription) error) error {
	return paginate(ctx, c, "/v1/subscriptions", url.Values{"status": {"all"}}, func(x Subscription) string { return x.ID }, fn)
}
