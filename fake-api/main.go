// Command fake-api serves a small, deterministic, Stripe-shaped API for the
// omission study. Every run sees the same data, and list endpoints return a
// 429 on a fixed schedule so rate-limit handling can be checked without
// touching real Stripe.
package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"log"
	"net/http"
	"strconv"
	"sync/atomic"
	"time"
)

const apiKey = "sk_test_omission"

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

type listResponse struct {
	Object  string `json:"object"`
	URL     string `json:"url"`
	HasMore bool   `json:"has_more"`
	Data    any    `json:"data"`
}

func main() {
	addr := flag.String("addr", ":12111", "listen address")
	numCustomers := flag.Int("customers", 237, "number of customers to serve")
	numSubs := flag.Int("subscriptions", 181, "number of subscriptions to serve")
	every := flag.Int("429-every", 5, "return 429 on every Nth list request (0 disables)")
	flag.Parse()

	customers, subs := seed(*numCustomers, *numSubs)
	var listRequests atomic.Int64

	mux := http.NewServeMux()
	mux.HandleFunc("GET /v1/account", auth(func(w http.ResponseWriter, r *http.Request) {
		writeJSON(w, http.StatusOK, map[string]any{"id": "acct_omission", "object": "account", "livemode": false})
	}))
	list := func(url string, items func(limit int, after string) (any, bool, bool)) http.HandlerFunc {
		return auth(func(w http.ResponseWriter, r *http.Request) {
			n := listRequests.Add(1)
			if *every > 0 && n%int64(*every) == 0 {
				w.Header().Set("Retry-After", "1")
				writeError(w, http.StatusTooManyRequests, "rate_limit", "Too many requests. Retry after 1 second.")
				return
			}
			limit, err := parseLimit(r.URL.Query().Get("limit"))
			if err != nil {
				writeError(w, http.StatusBadRequest, "invalid_request_error", err.Error())
				return
			}
			data, hasMore, ok := items(limit, r.URL.Query().Get("starting_after"))
			if !ok {
				writeError(w, http.StatusBadRequest, "invalid_request_error", "No such object: starting_after")
				return
			}
			writeJSON(w, http.StatusOK, listResponse{Object: "list", URL: url, HasMore: hasMore, Data: data})
		})
	}
	mux.HandleFunc("GET /v1/customers", list("/v1/customers", func(limit int, after string) (any, bool, bool) {
		return page(customers, func(c customer) string { return c.ID }, limit, after)
	}))
	// Like Stripe, subscriptions default to excluding canceled ones; status=all
	// returns everything. A backfill that skips this silently loses rows.
	visibleSubs := make([]subscription, 0, len(subs))
	for _, s := range subs {
		if s.Status != "canceled" {
			visibleSubs = append(visibleSubs, s)
		}
	}
	mux.HandleFunc("GET /v1/subscriptions", func(w http.ResponseWriter, r *http.Request) {
		set := visibleSubs
		switch st := r.URL.Query().Get("status"); st {
		case "":
		case "all":
			set = subs
		default:
			set = nil
			for _, s := range subs {
				if s.Status == st {
					set = append(set, s)
				}
			}
		}
		list("/v1/subscriptions", func(limit int, after string) (any, bool, bool) {
			return page(set, func(s subscription) string { return s.ID }, limit, after)
		})(w, r)
	})

	log.Printf("fake-api on %s: %d customers, %d subscriptions, 429 every %d list requests, key %q",
		*addr, len(customers), len(subs), *every, apiKey)
	log.Fatal(http.ListenAndServe(*addr, logRequests(mux)))
}

// seed builds the fixed dataset. Nothing is random, so every run is identical.
func seed(nc, ns int) ([]customer, []subscription) {
	base := time.Date(2025, 1, 1, 0, 0, 0, 0, time.UTC).Unix()
	customers := make([]customer, nc)
	for i := range customers {
		customers[i] = customer{
			ID:      fmt.Sprintf("cus_%04d", i+1),
			Object:  "customer",
			Email:   fmt.Sprintf("user%04d@example.com", i+1),
			Name:    fmt.Sprintf("Customer %d", i+1),
			Created: base + int64(i)*3600,
		}
	}
	statuses := []string{"active", "active", "active", "past_due", "canceled", "trialing"}
	subs := make([]subscription, ns)
	for i := range subs {
		subs[i] = subscription{
			ID:       fmt.Sprintf("sub_%04d", i+1),
			Object:   "subscription",
			Customer: customers[(i*7)%nc].ID,
			Status:   statuses[i%len(statuses)],
			Created:  base + int64(i)*5400,
		}
	}
	return customers, subs
}

// page returns up to limit items after the item whose ID is after. The bool
// results are has_more and whether starting_after referred to a real item.
func page[T any](items []T, id func(T) string, limit int, after string) ([]T, bool, bool) {
	start := 0
	if after != "" {
		start = -1
		for i, it := range items {
			if id(it) == after {
				start = i + 1
				break
			}
		}
		if start < 0 {
			return nil, false, false
		}
	}
	end := min(start+limit, len(items))
	return items[start:end], end < len(items), true
}

func parseLimit(s string) (int, error) {
	if s == "" {
		return 10, nil
	}
	n, err := strconv.Atoi(s)
	if err != nil || n < 1 || n > 100 {
		return 0, fmt.Errorf("limit must be an integer between 1 and 100")
	}
	return n, nil
}

func auth(next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Authorization") != "Bearer "+apiKey {
			writeError(w, http.StatusUnauthorized, "authentication_error", "Invalid API key provided.")
			return
		}
		next(w, r)
	}
}

func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	json.NewEncoder(w).Encode(v)
}

func writeError(w http.ResponseWriter, status int, typ, msg string) {
	writeJSON(w, status, map[string]any{"error": map[string]string{"type": typ, "message": msg}})
}

type statusRecorder struct {
	http.ResponseWriter
	status int
}

func (s *statusRecorder) WriteHeader(code int) {
	s.status = code
	s.ResponseWriter.WriteHeader(code)
}

func logRequests(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		rec := &statusRecorder{ResponseWriter: w, status: http.StatusOK}
		next.ServeHTTP(rec, r)
		log.Printf("%s %s -> %d", r.Method, r.URL.RequestURI(), rec.status)
	})
}
