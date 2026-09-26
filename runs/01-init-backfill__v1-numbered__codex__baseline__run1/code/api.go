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

type apiClient struct {
	baseURL, key string
	client       *http.Client
}

func (a *apiClient) checkAccount(ctx context.Context) error {
	resp, err := a.get(ctx, "/v1/account", nil)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode == http.StatusUnauthorized || resp.StatusCode == http.StatusForbidden {
		return fmt.Errorf("API key rejected (HTTP %d)", resp.StatusCode)
	}
	if resp.StatusCode < 200 || resp.StatusCode > 299 {
		return fmt.Errorf("check API key: API returned HTTP %d", resp.StatusCode)
	}
	return nil
}

func (a *apiClient) customers(ctx context.Context) ([]customer, error) {
	return paginate(a, ctx, "/v1/customers", nil, func(c customer) string { return c.ID })
}
func (a *apiClient) subscriptions(ctx context.Context) ([]subscription, error) {
	return paginate(a, ctx, "/v1/subscriptions", url.Values{"status": {"all"}}, func(s subscription) string { return s.ID })
}

func paginate[T any](a *apiClient, ctx context.Context, path string, initial url.Values, id func(T) string) ([]T, error) {
	q := make(url.Values)
	for k, values := range initial {
		q[k] = append([]string(nil), values...)
	}
	q.Set("limit", "100")
	var all []T
	for {
		resp, err := a.get(ctx, path, q)
		if err != nil {
			return nil, err
		}
		var page listResponse[T]
		err = json.NewDecoder(resp.Body).Decode(&page)
		resp.Body.Close()
		if err != nil {
			return nil, fmt.Errorf("decode API response: %w", err)
		}
		if resp.StatusCode < 200 || resp.StatusCode > 299 {
			return nil, fmt.Errorf("API returned HTTP %d", resp.StatusCode)
		}
		all = append(all, page.Data...)
		if !page.HasMore {
			return all, nil
		}
		if len(page.Data) == 0 {
			return nil, fmt.Errorf("API reported another page but returned no records")
		}
		q.Set("starting_after", id(page.Data[len(page.Data)-1]))
	}
}

func (a *apiClient) get(ctx context.Context, path string, q url.Values) (*http.Response, error) {
	for attempts := 0; ; attempts++ {
		u := a.baseURL + path
		if len(q) > 0 {
			u += "?" + q.Encode()
		}
		req, err := http.NewRequestWithContext(ctx, http.MethodGet, u, nil)
		if err != nil {
			return nil, err
		}
		req.Header.Set("Authorization", "Bearer "+a.key)
		resp, err := a.client.Do(req)
		if err != nil {
			return nil, err
		}
		if resp.StatusCode != http.StatusTooManyRequests {
			return resp, nil
		}
		resp.Body.Close()
		if attempts >= 4 {
			return nil, fmt.Errorf("API rate limit persisted after 5 attempts")
		}
		wait := time.Second
		if seconds, err := strconv.Atoi(strings.TrimSpace(resp.Header.Get("Retry-After"))); err == nil && seconds >= 0 {
			wait = time.Duration(seconds) * time.Second
		}
		select {
		case <-ctx.Done():
			return nil, ctx.Err()
		default:
		}
		sleep(wait)
	}
}
