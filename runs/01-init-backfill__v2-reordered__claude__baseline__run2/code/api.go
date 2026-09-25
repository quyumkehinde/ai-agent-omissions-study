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
	// sleep is replaceable in tests.
	sleep func(time.Duration)
}

func NewClient(baseURL, key string) *Client {
	return &Client{BaseURL: baseURL, APIKey: key, HTTP: &http.Client{Timeout: 30 * time.Second}, sleep: time.Sleep}
}

const maxRetries = 8

// get performs a GET, retrying on 429 (honoring Retry-After) and returns the body.
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
		case resp.StatusCode == http.StatusOK:
			return body, nil
		case resp.StatusCode == http.StatusUnauthorized:
			return nil, ErrUnauthorized
		case resp.StatusCode == http.StatusTooManyRequests && attempt < maxRetries:
			wait := time.Second
			if s, err := strconv.Atoi(resp.Header.Get("Retry-After")); err == nil && s >= 0 {
				wait = time.Duration(s) * time.Second
			}
			c.sleep(wait)
		default:
			var e struct {
				Error struct{ Type, Message string } `json:"error"`
			}
			if json.Unmarshal(body, &e) == nil && e.Error.Message != "" {
				return nil, fmt.Errorf("GET %s: %d %s: %s", path, resp.StatusCode, e.Error.Type, e.Error.Message)
			}
			return nil, fmt.Errorf("GET %s: unexpected status %d", path, resp.StatusCode)
		}
	}
}

func (c *Client) CheckAccount(ctx context.Context) error {
	body, err := c.get(ctx, "/v1/account", nil)
	if err != nil {
		return err
	}
	var a struct {
		ID string `json:"id"`
	}
	if err := json.Unmarshal(body, &a); err != nil || a.ID == "" {
		return errors.New("unexpected response from /v1/account")
	}
	return nil
}

// paginate walks a list endpoint, calling fn with each page's raw items.
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
		body, err := c.get(ctx, path, q)
		if err != nil {
			return err
		}
		var page struct {
			HasMore bool `json:"has_more"`
			Data    []T  `json:"data"`
		}
		if err := json.Unmarshal(body, &page); err != nil {
			return fmt.Errorf("decoding %s: %w", path, err)
		}
		if len(page.Data) > 0 {
			if err := fn(page.Data); err != nil {
				return err
			}
		}
		if !page.HasMore {
			return nil
		}
		if len(page.Data) == 0 {
			return fmt.Errorf("%s: has_more true but page empty", path)
		}
		after = id(page.Data[len(page.Data)-1])
	}
}

func (c *Client) EachCustomerPage(ctx context.Context, fn func([]Customer) error) error {
	return paginate(ctx, c, "/v1/customers", nil, func(x Customer) string { return x.ID }, fn)
}

// EachSubscriptionPage includes canceled subscriptions (status=all).
func (c *Client) EachSubscriptionPage(ctx context.Context, fn func([]Subscription) error) error {
	return paginate(ctx, c, "/v1/subscriptions", url.Values{"status": {"all"}}, func(x Subscription) string { return x.ID }, fn)
}
