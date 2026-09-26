package main

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"strconv"
	"time"
)

const defaultAPIBaseURL = "http://localhost:12111"

type apiClient struct {
	baseURL, apiKey string
	httpClient      *http.Client
}

func newAPIClient(key string) *apiClient {
	return &apiClient{baseURL: defaultAPIBaseURL, apiKey: key, httpClient: http.DefaultClient}
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
type customersPage struct {
	HasMore bool       `json:"has_more"`
	Data    []customer `json:"data"`
}
type subscriptionsPage struct {
	HasMore bool           `json:"has_more"`
	Data    []subscription `json:"data"`
}

func (c *apiClient) checkAccount(ctx context.Context) error {
	resp, err := c.get(ctx, "/v1/account", nil)
	if err != nil {
		return fmt.Errorf("check API key: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode == http.StatusUnauthorized || resp.StatusCode == http.StatusForbidden {
		return fmt.Errorf("API key rejected (HTTP %d)", resp.StatusCode)
	}
	if resp.StatusCode/100 != 2 {
		return responseError("check API key", resp)
	}
	return nil
}

func (c *apiClient) get(ctx context.Context, path string, query url.Values) (*http.Response, error) {
	u, err := url.Parse(c.baseURL + path)
	if err != nil {
		return nil, err
	}
	u.RawQuery = query.Encode()
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, u.String(), nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Authorization", "Bearer "+c.apiKey)
	return c.httpClient.Do(req)
}

func responseError(action string, resp *http.Response) error {
	return fmt.Errorf("%s: API returned HTTP %d", action, resp.StatusCode)
}

func importCustomers(ctx context.Context, db *sql.DB, c *apiClient) error {
	return c.eachCustomer(ctx, func(item customer) error {
		_, err := db.ExecContext(ctx, `INSERT INTO customers (id, object, email, name, created) VALUES ($1,$2,$3,$4,$5)
ON CONFLICT (id) DO UPDATE SET object=EXCLUDED.object, email=EXCLUDED.email, name=EXCLUDED.name, created=EXCLUDED.created`, item.ID, item.Object, item.Email, item.Name, item.Created)
		if err != nil {
			return fmt.Errorf("upsert customer %s: %w", item.ID, err)
		}
		return nil
	})
}

func importSubscriptions(ctx context.Context, db *sql.DB, c *apiClient) error {
	return c.eachSubscription(ctx, func(item subscription) error {
		_, err := db.ExecContext(ctx, `INSERT INTO subscriptions (id, object, customer, status, created) VALUES ($1,$2,$3,$4,$5)
ON CONFLICT (id) DO UPDATE SET object=EXCLUDED.object, customer=EXCLUDED.customer, status=EXCLUDED.status, created=EXCLUDED.created`, item.ID, item.Object, item.Customer, item.Status, item.Created)
		if err != nil {
			return fmt.Errorf("upsert subscription %s: %w", item.ID, err)
		}
		return nil
	})
}

func (c *apiClient) eachCustomer(ctx context.Context, fn func(customer) error) error {
	startingAfter := ""
	for {
		q := url.Values{"limit": {"100"}}
		if startingAfter != "" {
			q.Set("starting_after", startingAfter)
		}
		var page customersPage
		if err := c.getPage(ctx, "/v1/customers", q, &page); err != nil {
			return fmt.Errorf("list customers: %w", err)
		}
		for _, item := range page.Data {
			if err := fn(item); err != nil {
				return err
			}
			startingAfter = item.ID
		}
		if !page.HasMore {
			return nil
		}
		if len(page.Data) == 0 {
			return fmt.Errorf("list customers: API reported more results but returned none")
		}
	}
}

func (c *apiClient) eachSubscription(ctx context.Context, fn func(subscription) error) error {
	startingAfter := ""
	for {
		q := url.Values{"limit": {"100"}, "status": {"all"}}
		if startingAfter != "" {
			q.Set("starting_after", startingAfter)
		}
		var page subscriptionsPage
		if err := c.getPage(ctx, "/v1/subscriptions", q, &page); err != nil {
			return fmt.Errorf("list subscriptions: %w", err)
		}
		for _, item := range page.Data {
			if err := fn(item); err != nil {
				return err
			}
			startingAfter = item.ID
		}
		if !page.HasMore {
			return nil
		}
		if len(page.Data) == 0 {
			return fmt.Errorf("list subscriptions: API reported more results but returned none")
		}
	}
}

func (c *apiClient) getPage(ctx context.Context, path string, query url.Values, destination any) error {
	for attempts := 0; ; attempts++ {
		resp, err := c.get(ctx, path, query)
		if err != nil {
			return err
		}
		if resp.StatusCode == http.StatusTooManyRequests && attempts < 3 {
			resp.Body.Close()
			delay, _ := strconv.Atoi(resp.Header.Get("Retry-After"))
			if delay < 1 {
				delay = 1
			}
			select {
			case <-ctx.Done():
				return ctx.Err()
			case <-time.After(time.Duration(delay) * time.Second):
			}
			continue
		}
		if resp.StatusCode/100 != 2 {
			resp.Body.Close()
			return responseError("list data", resp)
		}
		err = json.NewDecoder(resp.Body).Decode(destination)
		resp.Body.Close()
		return err
	}
}
