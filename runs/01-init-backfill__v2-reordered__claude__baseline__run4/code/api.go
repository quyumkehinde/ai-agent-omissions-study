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

const (
	pageSize       = 100
	maxRateRetries = 8
)

// ErrUnauthorized is returned when the API rejects the key.
var ErrUnauthorized = errors.New("API key rejected (401 unauthorized)")

type Customer struct {
	ID      string  `json:"id"`
	Object  string  `json:"object"`
	Email   *string `json:"email"`
	Name    *string `json:"name"`
	Created int64   `json:"created"`
}

type Subscription struct {
	ID       string  `json:"id"`
	Object   string  `json:"object"`
	Customer *string `json:"customer"`
	Status   *string `json:"status"`
	Created  int64   `json:"created"`
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
		Sleep: func(ctx context.Context, d time.Duration) error {
			select {
			case <-time.After(d):
				return nil
			case <-ctx.Done():
				return ctx.Err()
			}
		},
	}
}

// get performs a GET, retrying on 429, and returns the body of a 2xx response.
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
			if attempt >= maxRateRetries {
				return nil, fmt.Errorf("GET %s: still rate limited after %d retries", path, attempt)
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
				return nil, fmt.Errorf("GET %s: %s: %s (HTTP %d)", path, e.Error.Type, e.Error.Message, resp.StatusCode)
			}
			return nil, fmt.Errorf("GET %s: HTTP %d", path, resp.StatusCode)
		}
		return body, nil
	}
}

func (c *Client) CheckAccount(ctx context.Context) error {
	_, err := c.get(ctx, "/v1/account", nil)
	return err
}

// listAll pages through path, calling handle with each page of raw items.
func listAll[T any](ctx context.Context, c *Client, path string, extra url.Values, id func(T) string, handle func([]T) error) error {
	after := ""
	for {
		q := url.Values{"limit": {strconv.Itoa(pageSize)}}
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

func (c *Client) EachCustomerPage(ctx context.Context, handle func([]Customer) error) error {
	return listAll(ctx, c, "/v1/customers", nil, func(x Customer) string { return x.ID }, handle)
}

// EachSubscriptionPage includes canceled subscriptions (status=all), which the
// API otherwise omits.
func (c *Client) EachSubscriptionPage(ctx context.Context, handle func([]Subscription) error) error {
	return listAll(ctx, c, "/v1/subscriptions", url.Values{"status": {"all"}}, func(x Subscription) string { return x.ID }, handle)
}
