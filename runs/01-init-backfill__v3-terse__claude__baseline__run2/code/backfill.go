package main

import (
	"context"
	"fmt"

	"github.com/jackc/pgx/v5"
)

type BackfillResult struct {
	Customers     int
	Subscriptions int
}

// Backfill copies every customer and subscription (including canceled) into Postgres.
// It is idempotent: rows are upserted by ID.
func Backfill(ctx context.Context, c *Client, conn *pgx.Conn) (BackfillResult, error) {
	var r BackfillResult
	err := c.EachCustomerPage(ctx, func(cs []Customer) error {
		if err := UpsertCustomers(ctx, conn, cs); err != nil {
			return fmt.Errorf("writing customers: %w", err)
		}
		r.Customers += len(cs)
		return nil
	})
	if err != nil {
		return r, fmt.Errorf("backfilling customers: %w", err)
	}
	err = c.EachSubscriptionPage(ctx, func(ss []Subscription) error {
		if err := UpsertSubscriptions(ctx, conn, ss); err != nil {
			return fmt.Errorf("writing subscriptions: %w", err)
		}
		r.Subscriptions += len(ss)
		return nil
	})
	if err != nil {
		return r, fmt.Errorf("backfilling subscriptions: %w", err)
	}
	return r, nil
}
