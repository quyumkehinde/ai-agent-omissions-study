package main

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"os/signal"
)

func main() {
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt)
	defer stop()
	baseURL := os.Getenv("MIRROR_API_URL")
	if baseURL == "" {
		baseURL = defaultBaseURL
	}
	if err := run(ctx, os.Args[1:], os.Getenv, baseURL, os.Stdout); err != nil {
		fmt.Fprintln(os.Stderr, "error:", err)
		os.Exit(1)
	}
}

func requireEnv(getenv func(string) string, names ...string) ([]string, error) {
	vals := make([]string, len(names))
	var missing []string
	for i, n := range names {
		vals[i] = getenv(n)
		if vals[i] == "" {
			missing = append(missing, n)
		}
	}
	if len(missing) > 0 {
		return nil, fmt.Errorf("missing required environment variable(s): %v", missing)
	}
	return vals, nil
}

func run(ctx context.Context, args []string, getenv func(string) string, baseURL string, out io.Writer) error {
	if len(args) != 1 || (args[0] != "init" && args[0] != "status") {
		return errors.New("usage: mirror <init|status>")
	}
	env, err := requireEnv(getenv, "MIRROR_DATABASE_URL", "MIRROR_API_KEY")
	if err != nil {
		return err
	}
	dsn, key := env[0], env[1]

	switch args[0] {
	case "init":
		// Validate the key before touching the database.
		client := NewClient(baseURL, key)
		if _, err := client.Account(ctx); err != nil {
			return fmt.Errorf("checking API key: %w", err)
		}
		store, err := OpenStore(ctx, dsn)
		if err != nil {
			return err
		}
		defer store.Close()
		return runInit(ctx, client, store, out)
	default:
		store, err := OpenStore(ctx, dsn)
		if err != nil {
			return err
		}
		defer store.Close()
		return runStatus(ctx, store, out)
	}
}

// Sink is the storage used by init and status.
type Sink interface {
	CreateSchema(ctx context.Context) error
	UpsertCustomers(ctx context.Context, cs []Customer) error
	UpsertSubscriptions(ctx context.Context, ss []Subscription) error
	Count(ctx context.Context, table string) (int64, error)
}

func runInit(ctx context.Context, c *Client, s Sink, out io.Writer) error {
	if err := s.CreateSchema(ctx); err != nil {
		return fmt.Errorf("creating tables: %w", err)
	}
	var nc, ns int
	if err := c.EachCustomerPage(ctx, func(p []Customer) error {
		nc += len(p)
		return s.UpsertCustomers(ctx, p)
	}); err != nil {
		return fmt.Errorf("importing customers: %w", err)
	}
	if err := c.EachSubscriptionPage(ctx, func(p []Subscription) error {
		ns += len(p)
		return s.UpsertSubscriptions(ctx, p)
	}); err != nil {
		return fmt.Errorf("importing subscriptions: %w", err)
	}
	fmt.Fprintf(out, "imported %d customers, %d subscriptions\n", nc, ns)
	return nil
}

func runStatus(ctx context.Context, s Sink, out io.Writer) error {
	for _, t := range []string{"customers", "subscriptions"} {
		n, err := s.Count(ctx, t)
		if err != nil {
			return fmt.Errorf("counting %s (has `mirror init` been run?): %w", t, err)
		}
		fmt.Fprintf(out, "%s: %d\n", t, n)
	}
	return nil
}
