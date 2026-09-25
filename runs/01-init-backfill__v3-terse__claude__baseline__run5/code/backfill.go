package main

import (
	"context"
	"database/sql"
	"fmt"
	"io"
)

// backfill copies all customers and subscriptions (including canceled) into
// Postgres. Upserts make it safe to re-run.
func backfill(ctx context.Context, c *Client, db *sql.DB, out io.Writer) error {
	nc, ns := 0, 0
	err := c.EachCustomerPage(ctx, func(cs []Customer) error {
		nc += len(cs)
		return upsertCustomers(ctx, db, cs)
	})
	if err != nil {
		return fmt.Errorf("backfilling customers: %w", err)
	}
	fmt.Fprintf(out, "Backfilled %d customers\n", nc)
	err = c.EachSubscriptionPage(ctx, func(ss []Subscription) error {
		ns += len(ss)
		return upsertSubscriptions(ctx, db, ss)
	})
	if err != nil {
		return fmt.Errorf("backfilling subscriptions: %w", err)
	}
	fmt.Fprintf(out, "Backfilled %d subscriptions\n", ns)
	return nil
}
