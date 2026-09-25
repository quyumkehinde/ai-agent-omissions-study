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

const DefaultBaseURL = "http://localhost:12111"

const (
	pageSize   = 100
	maxRetries = 8
)

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

type Account struct {
	ID       string `json:"id"`
	Object   string `json:"object"`
	Livemode bool   `json:"livemode"`
}

type Client struct {
	BaseURL string
	APIKey  string
	HTTP    *http.Client
	// Sleep is replaceable in tests.
	Sleep func(time.Duration)
}

func NewClient(baseURL, apiKey string) *Client {
	return &Client{
		BaseURL: baseURL,
		APIKey:  apiKey,
		HTTP:    &http.Client{Timeout: 30 * time.Second},
		Sleep:   time.Sleep,
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
			return fmt.Errorf("GET %s: reading body: %w", path, err)
		}
		switch {
		case resp.StatusCode == http.StatusUnauthorized:
			return ErrUnauthorized
		case resp.StatusCode == http.StatusTooManyRequests:
			if attempt >= maxRetries {
				return fmt.Errorf("GET %s: still rate limited after %d retries", path, maxRetries)
			}
			wait := time.Second
			if s, err := strconv.Atoi(resp.Header.Get("Retry-After")); err == nil && s >= 0 {
				wait = time.Duration(s) * time.Second
			}
			c.Sleep(wait)
			continue
		case resp.StatusCode < 200 || resp.StatusCode > 299:
			var e struct {
				Error struct{ Type, Message string } `json:"error"`
			}
			if json.Unmarshal(body, &e) == nil && e.Error.Message != "" {
				return fmt.Errorf("GET %s: %d %s: %s", path, resp.StatusCode, e.Error.Type, e.Error.Message)
			}
			return fmt.Errorf("GET %s: unexpected status %d", path, resp.StatusCode)
		}
		if err := json.Unmarshal(body, out); err != nil {
			return fmt.Errorf("GET %s: decoding response: %w", path, err)
		}
		return nil
	}
}

func (c *Client) Account(ctx context.Context) (*Account, error) {
	var a Account
	if err := c.get(ctx, "/v1/account", nil, &a); err != nil {
		return nil, err
	}
	return &a, nil
}

// listAll pages through a list endpoint, calling fn for each page.
func listAll[T any](ctx context.Context, c *Client, path string, extra url.Values, id func(T) string, fn func([]T) error) error {
	after := ""
	for {
		q := url.Values{"limit": {strconv.Itoa(pageSize)}}
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
			if err := fn(page.Data); err != nil {
				return err
			}
			after = id(page.Data[len(page.Data)-1])
		}
		if !page.HasMore {
			return nil
		}
		if len(page.Data) == 0 {
			return errors.New("API reported has_more with an empty page")
		}
	}
}

func (c *Client) ListCustomers(ctx context.Context, fn func([]Customer) error) error {
	return listAll(ctx, c, "/v1/customers", nil, func(x Customer) string { return x.ID }, fn)
}

// ListSubscriptions includes canceled subscriptions (status=all).
func (c *Client) ListSubscriptions(ctx context.Context, fn func([]Subscription) error) error {
	return listAll(ctx, c, "/v1/subscriptions", url.Values{"status": {"all"}}, func(x Subscription) string { return x.ID }, fn)
}
