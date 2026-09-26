package main

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"
)

type apiClient struct {
	baseURL string
	key     string
	http    *http.Client
}

func newAPIClient(baseURL, key string) *apiClient {
	return &apiClient{baseURL: strings.TrimRight(baseURL, "/"), key: key, http: &http.Client{Timeout: 30 * time.Second}}
}

type customer struct {
	ID      string `json:"id"`
	Object  string `json:"object"`
	Email   string `json:"email"`
	Name    string `json:"name"`
	Created int64  `json:"created"`
}

type subscription struct {
	ID       string `json:"id"`
	Object   string `json:"object"`
	Customer string `json:"customer"`
	Status   string `json:"status"`
	Created  int64  `json:"created"`
}

type listResponse[T any] struct {
	HasMore bool `json:"has_more"`
	Data    []T  `json:"data"`
}

func (c *apiClient) checkAccount(ctx context.Context) error {
	resp, err := c.get(ctx, "/v1/account", nil)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode == http.StatusUnauthorized || resp.StatusCode == http.StatusForbidden {
		return fmt.Errorf("MIRROR_API_KEY was rejected (HTTP %d)", resp.StatusCode)
	}
	if resp.StatusCode/100 != 2 {
		return responseError(resp)
	}
	return nil
}

func (c *apiClient) get(ctx context.Context, path string, query url.Values) (*http.Response, error) {
	u := c.baseURL + path
	if len(query) > 0 {
		u += "?" + query.Encode()
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, u, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Authorization", "Bearer "+c.key)
	return c.http.Do(req)
}

func responseError(resp *http.Response) error {
	return fmt.Errorf("API request failed: HTTP %d", resp.StatusCode)
}

func backfill(ctx context.Context, client *apiClient, dst store) error {
	if err := fetchAll(ctx, client, "/v1/customers", nil, func(c customer) error { return dst.upsertCustomer(ctx, c) }); err != nil {
		return fmt.Errorf("backfill customers: %w", err)
	}
	q := url.Values{"status": {"all"}}
	if err := fetchAll(ctx, client, "/v1/subscriptions", q, func(s subscription) error { return dst.upsertSubscription(ctx, s) }); err != nil {
		return fmt.Errorf("backfill subscriptions: %w", err)
	}
	return nil
}

func fetchAll[T interface{ id() string }](ctx context.Context, client *apiClient, path string, initial url.Values, save func(T) error) error {
	q := url.Values{"limit": {"100"}}
	for k, v := range initial {
		q[k] = append([]string(nil), v...)
	}
	for {
		resp, err := client.get(ctx, path, q)
		if err != nil {
			return err
		}
		if resp.StatusCode == http.StatusTooManyRequests {
			wait, _ := strconv.Atoi(resp.Header.Get("Retry-After"))
			resp.Body.Close()
			if wait < 1 {
				wait = 1
			}
			select {
			case <-ctx.Done():
				return ctx.Err()
			case <-time.After(time.Duration(wait) * time.Second):
			}
			continue
		}
		if resp.StatusCode/100 != 2 {
			err := responseError(resp)
			resp.Body.Close()
			return err
		}
		var page listResponse[T]
		err = json.NewDecoder(resp.Body).Decode(&page)
		resp.Body.Close()
		if err != nil {
			return fmt.Errorf("decode response: %w", err)
		}
		for _, item := range page.Data {
			if err := save(item); err != nil {
				return err
			}
		}
		if !page.HasMore {
			return nil
		}
		if len(page.Data) == 0 {
			return fmt.Errorf("API returned has_more with an empty page")
		}
		q.Set("starting_after", page.Data[len(page.Data)-1].id())
	}
}

func (c customer) id() string     { return c.ID }
func (s subscription) id() string { return s.ID }
