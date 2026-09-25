package main

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"io"
	"os"
	"os/signal"

	_ "github.com/lib/pq"
)

const usage = "usage: mirror <init|status>"

func main() {
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt)
	defer stop()
	if err := run(ctx, os.Args[1:], os.Getenv, os.Stdout); err != nil {
		fmt.Fprintln(os.Stderr, "mirror:", err)
		os.Exit(1)
	}
}

func run(ctx context.Context, args []string, getenv func(string) string, out io.Writer) error {
	if len(args) != 1 || (args[0] != "init" && args[0] != "status") {
		return errors.New(usage)
	}
	cmd := args[0]

	dbURL := getenv("MIRROR_DATABASE_URL")
	if dbURL == "" {
		return errors.New("MIRROR_DATABASE_URL is not set")
	}
	var client *Client
	if cmd == "init" {
		key := getenv("MIRROR_API_KEY")
		if key == "" {
			return errors.New("MIRROR_API_KEY is not set")
		}
		base := getenv("MIRROR_API_URL")
		if base == "" {
			base = defaultAPIURL
		}
		client = NewClient(base, key)
	}

	db, err := sql.Open("postgres", dbURL)
	if err != nil {
		return fmt.Errorf("opening database: %w", err)
	}
	defer db.Close()
	store := &Store{DB: db}

	if cmd == "init" {
		return runInit(ctx, client, store, out)
	}
	return runStatus(ctx, store, out)
}

func runInit(ctx context.Context, c *Client, s *Store, out io.Writer) error {
	if _, err := c.Account(ctx); err != nil {
		return fmt.Errorf("checking API key: %w", err)
	}
	if err := s.DB.PingContext(ctx); err != nil {
		return fmt.Errorf("connecting to database: %w", err)
	}
	if err := s.CreateTables(ctx); err != nil {
		return fmt.Errorf("creating tables: %w", err)
	}
	var nc, ns int
	err := c.EachCustomer(ctx, func(cs []Customer) error {
		nc += len(cs)
		return s.UpsertCustomers(ctx, cs)
	})
	if err != nil {
		return fmt.Errorf("importing customers: %w", err)
	}
	err = c.EachSubscription(ctx, func(ss []Subscription) error {
		ns += len(ss)
		return s.UpsertSubscriptions(ctx, ss)
	})
	if err != nil {
		return fmt.Errorf("importing subscriptions: %w", err)
	}
	fmt.Fprintf(out, "imported %d customers and %d subscriptions\n", nc, ns)
	return nil
}

func runStatus(ctx context.Context, s *Store, out io.Writer) error {
	counts, err := s.Counts(ctx)
	if err != nil {
		return err
	}
	for _, t := range tables {
		fmt.Fprintf(out, "%-15s %d\n", t, counts[t])
	}
	return nil
}
