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

const defaultBaseURL = "http://localhost:12111"

// ErrUnauthorized is returned when the API rejects the API key.
var ErrUnauthorized = errors.New("API key rejected (401 unauthorized)")

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
	// sleep is replaceable in tests.
	sleep      func(context.Context, time.Duration) error
	maxRetries int
}

func NewClient(baseURL, apiKey string) *Client {
	return &Client{
		BaseURL:    baseURL,
		APIKey:     apiKey,
		HTTP:       &http.Client{Timeout: 30 * time.Second},
		sleep:      sleepCtx,
		maxRetries: 8,
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
			return fmt.Errorf("GET %s: reading response: %w", path, err)
		}
		switch {
		case resp.StatusCode == http.StatusUnauthorized:
			return ErrUnauthorized
		case resp.StatusCode == http.StatusTooManyRequests:
			if attempt >= c.maxRetries {
				return fmt.Errorf("GET %s: still rate limited after %d retries", path, attempt)
			}
			wait := time.Second
			if s, err := strconv.ParseFloat(resp.Header.Get("Retry-After"), 64); err == nil && s >= 0 {
				wait = time.Duration(s * float64(time.Second))
			}
			if err := c.sleep(ctx, wait); err != nil {
				return err
			}
			continue
		case resp.StatusCode < 200 || resp.StatusCode > 299:
			return fmt.Errorf("GET %s: %s", path, describeError(resp.StatusCode, body))
		}
		if err := json.Unmarshal(body, out); err != nil {
			return fmt.Errorf("GET %s: decoding response: %w", path, err)
		}
		return nil
	}
}

func describeError(status int, body []byte) string {
	var e struct {
		Error struct {
			Type    string `json:"type"`
			Message string `json:"message"`
		} `json:"error"`
	}
	if json.Unmarshal(body, &e) == nil && e.Error.Message != "" {
		return fmt.Sprintf("HTTP %d: %s: %s", status, e.Error.Type, e.Error.Message)
	}
	return fmt.Sprintf("HTTP %d", status)
}

// CheckAccount calls GET /v1/account.
func (c *Client) CheckAccount(ctx context.Context) (string, error) {
	var a struct {
		ID string `json:"id"`
	}
	if err := c.get(ctx, "/v1/account", nil, &a); err != nil {
		return "", err
	}
	return a.ID, nil
}

// paginate walks a list endpoint, calling handle for each page of raw items.
func paginate[T any](ctx context.Context, c *Client, path string, extra url.Values, id func(T) string, handle func([]T) error) error {
	after := ""
	for {
		q := url.Values{"limit": {"100"}}
		for k, v := range extra {
			q[k] = v
		}
		if after != "" {
			q.Set("starting_after", after)
		}
		var page struct {
			HasMore bool `json:"has_more"`
			Data    []T  `json:"data"`
		}
		if err := c.get(ctx, path, q, &page); err != nil {
			return err
		}
		if len(page.Data) > 0 {
			if err := handle(page.Data); err != nil {
				return err
			}
			after = id(page.Data[len(page.Data)-1])
		}
		if !page.HasMore {
			return nil
		}
		if len(page.Data) == 0 {
			return fmt.Errorf("GET %s: has_more is true but page is empty", path)
		}
	}
}

// EachCustomer calls handle for every page of customers.
func (c *Client) EachCustomer(ctx context.Context, handle func([]Customer) error) error {
	return paginate(ctx, c, "/v1/customers", nil, func(x Customer) string { return x.ID }, handle)
}

// EachSubscription calls handle for every page of subscriptions, including canceled ones.
func (c *Client) EachSubscription(ctx context.Context, handle func([]Subscription) error) error {
	return paginate(ctx, c, "/v1/subscriptions", url.Values{"status": {"all"}}, func(x Subscription) string { return x.ID }, handle)
}
