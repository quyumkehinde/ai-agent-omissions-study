package main

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"os/signal"
)

const usage = "usage: mirror <init|status>"

func main() {
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt)
	defer stop()
	if err := run(ctx, os.Args[1:], os.Getenv, os.Stdout); err != nil {
		fmt.Fprintln(os.Stderr, "error:", err)
		os.Exit(1)
	}
}

func run(ctx context.Context, args []string, getenv func(string) string, out io.Writer) error {
	if len(args) != 1 || (args[0] != "init" && args[0] != "status") {
		return errors.New(usage)
	}
	dbURL, apiKey := getenv("MIRROR_DATABASE_URL"), getenv("MIRROR_API_KEY")
	var missing []string
	if dbURL == "" {
		missing = append(missing, "MIRROR_DATABASE_URL")
	}
	if apiKey == "" {
		missing = append(missing, "MIRROR_API_KEY")
	}
	if len(missing) > 0 {
		return fmt.Errorf("missing required environment variable(s): %v", missing)
	}
	baseURL := getenv("MIRROR_API_URL")
	if baseURL == "" {
		baseURL = defaultBaseURL
	}

	store, err := OpenStore(dbURL)
	if err != nil {
		return fmt.Errorf("connecting to database: %w", err)
	}
	defer store.Close()

	switch args[0] {
	case "init":
		return runInit(ctx, NewClient(baseURL, apiKey), store, out)
	default:
		return runStatus(ctx, store, out)
	}
}

func runInit(ctx context.Context, c *Client, s *Store, out io.Writer) error {
	acct, err := c.CheckAccount(ctx)
	if err != nil {
		return fmt.Errorf("checking API key: %w", err)
	}
	fmt.Fprintf(out, "API key OK (account %s)\n", acct)

	if err := s.CreateTables(ctx); err != nil {
		return fmt.Errorf("creating tables: %w", err)
	}

	var nc, ns int
	err = c.EachCustomerPage(ctx, func(cs []Customer) error {
		nc += len(cs)
		return s.UpsertCustomers(ctx, cs)
	})
	if err != nil {
		return fmt.Errorf("importing customers: %w", err)
	}
	fmt.Fprintf(out, "Imported %d customers\n", nc)

	err = c.EachSubscriptionPage(ctx, func(ss []Subscription) error {
		ns += len(ss)
		return s.UpsertSubscriptions(ctx, ss)
	})
	if err != nil {
		return fmt.Errorf("importing subscriptions: %w", err)
	}
	fmt.Fprintf(out, "Imported %d subscriptions\n", ns)
	return nil
}

func runStatus(ctx context.Context, s *Store, out io.Writer) error {
	for _, t := range []string{"customers", "subscriptions"} {
		n, err := s.Count(ctx, t)
		if err != nil {
			return fmt.Errorf("counting %s (has `mirror init` been run?): %w", t, err)
		}
		fmt.Fprintf(out, "%s: %d\n", t, n)
	}
	return nil
}
