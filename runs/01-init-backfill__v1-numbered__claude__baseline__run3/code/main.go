package main

import (
	"context"
	"fmt"
	"io"
	"os"
	"os/signal"
	"strings"
)

const usage = "usage: mirror <init|status>"

func main() {
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt)
	defer stop()
	os.Exit(run(ctx, os.Args[1:], os.Getenv, os.Stdout, os.Stderr))
}

func run(ctx context.Context, args []string, getenv func(string) string, stdout, stderr io.Writer) int {
	if len(args) != 1 || (args[0] != "init" && args[0] != "status") {
		fmt.Fprintln(stderr, usage)
		return 2
	}
	var missing []string
	for _, k := range []string{"MIRROR_DATABASE_URL", "MIRROR_API_KEY"} {
		if getenv(k) == "" {
			missing = append(missing, k)
		}
	}
	if len(missing) > 0 {
		fmt.Fprintf(stderr, "error: missing required environment variable(s): %s\n", strings.Join(missing, ", "))
		return 1
	}
	baseURL := getenv("MIRROR_API_URL")
	if baseURL == "" {
		baseURL = defaultBaseURL
	}
	client := NewClient(baseURL, getenv("MIRROR_API_KEY"))

	var err error
	switch args[0] {
	case "init":
		err = runInit(ctx, client, getenv("MIRROR_DATABASE_URL"), stdout)
	case "status":
		err = runStatus(ctx, getenv("MIRROR_DATABASE_URL"), stdout)
	}
	if err != nil {
		fmt.Fprintf(stderr, "error: %v\n", err)
		return 1
	}
	return 0
}

func runInit(ctx context.Context, client *Client, dsn string, out io.Writer) error {
	// Validate the key before touching the database.
	if err := client.CheckAccount(ctx); err != nil {
		return fmt.Errorf("checking API key: %w", err)
	}
	store, err := OpenStore(ctx, dsn)
	if err != nil {
		return err
	}
	defer store.Close(context.Background())
	if err := store.CreateSchema(ctx); err != nil {
		return fmt.Errorf("creating tables: %w", err)
	}
	nc, ns := 0, 0
	if err := client.ListCustomers(ctx, func(cs []Customer) error {
		nc += len(cs)
		return store.UpsertCustomers(ctx, cs)
	}); err != nil {
		return fmt.Errorf("importing customers: %w", err)
	}
	if err := client.ListSubscriptions(ctx, func(ss []Subscription) error {
		ns += len(ss)
		return store.UpsertSubscriptions(ctx, ss)
	}); err != nil {
		return fmt.Errorf("importing subscriptions: %w", err)
	}
	fmt.Fprintf(out, "imported %d customers, %d subscriptions\n", nc, ns)
	return nil
}

func runStatus(ctx context.Context, dsn string, out io.Writer) error {
	store, err := OpenStore(ctx, dsn)
	if err != nil {
		return err
	}
	defer store.Close(context.Background())
	for _, t := range []string{"customers", "subscriptions"} {
		n, err := store.Count(ctx, t)
		if err != nil {
			return fmt.Errorf("counting %s (has `mirror init` been run?): %w", t, err)
		}
		fmt.Fprintf(out, "%s: %d\n", t, n)
	}
	return nil
}
