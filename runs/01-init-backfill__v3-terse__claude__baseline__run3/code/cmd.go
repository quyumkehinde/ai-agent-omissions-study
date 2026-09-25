package main

import (
	"context"
	"fmt"
	"io"
	"os"

	"github.com/jackc/pgx/v5"
)

type Config struct {
	DatabaseURL string
	APIKey      string
	APIURL      string
}

// LoadConfig reads the environment, naming every missing variable.
func LoadConfig(getenv func(string) string) (Config, error) {
	cfg := Config{
		DatabaseURL: getenv("MIRROR_DATABASE_URL"),
		APIKey:      getenv("MIRROR_API_KEY"),
		APIURL:      getenv("MIRROR_API_URL"),
	}
	if cfg.APIURL == "" {
		cfg.APIURL = defaultAPIURL
	}
	var missing []string
	if cfg.DatabaseURL == "" {
		missing = append(missing, "MIRROR_DATABASE_URL")
	}
	if cfg.APIKey == "" {
		missing = append(missing, "MIRROR_API_KEY")
	}
	if len(missing) == 1 {
		return cfg, fmt.Errorf("%s is not set", missing[0])
	}
	if len(missing) > 1 {
		return cfg, fmt.Errorf("%s and %s are not set", missing[0], missing[1])
	}
	return cfg, nil
}

func runInit(ctx context.Context, cfg Config, out io.Writer) error {
	client := NewClient(cfg.APIURL, cfg.APIKey)
	if err := client.CheckAccount(ctx); err != nil {
		return fmt.Errorf("checking MIRROR_API_KEY: %w", err)
	}
	conn, err := pgx.Connect(ctx, cfg.DatabaseURL)
	if err != nil {
		return fmt.Errorf("connecting to MIRROR_DATABASE_URL: %w", err)
	}
	defer conn.Close(ctx)

	if err := CreateSchema(ctx, conn); err != nil {
		return err
	}
	fmt.Fprintln(out, "created tables: customers, subscriptions")
	return Backfill(ctx, client, conn, out)
}

func Backfill(ctx context.Context, client *Client, conn *pgx.Conn, out io.Writer) error {
	nc, ns := 0, 0
	err := client.EachCustomerPage(ctx, func(cs []Customer) error {
		nc += len(cs)
		return UpsertCustomers(ctx, conn, cs)
	})
	if err != nil {
		return fmt.Errorf("backfilling customers: %w", err)
	}
	fmt.Fprintf(out, "backfilled %d customers\n", nc)
	err = client.EachSubscriptionPage(ctx, func(ss []Subscription) error {
		ns += len(ss)
		return UpsertSubscriptions(ctx, conn, ss)
	})
	if err != nil {
		return fmt.Errorf("backfilling subscriptions: %w", err)
	}
	fmt.Fprintf(out, "backfilled %d subscriptions\n", ns)
	return nil
}

func runStatus(ctx context.Context, cfg Config, out io.Writer) error {
	conn, err := pgx.Connect(ctx, cfg.DatabaseURL)
	if err != nil {
		return fmt.Errorf("connecting to MIRROR_DATABASE_URL: %w", err)
	}
	defer conn.Close(ctx)
	counts, err := Counts(ctx, conn)
	if err != nil {
		return err
	}
	for _, t := range tables {
		fmt.Fprintf(out, "%-14s %d\n", t, counts[t])
	}
	return nil
}

func run(ctx context.Context, args []string, getenv func(string) string, out io.Writer) error {
	if len(args) != 1 || (args[0] != "init" && args[0] != "status") {
		return fmt.Errorf("usage: mirror <init|status>")
	}
	cfg, err := LoadConfig(getenv)
	if err != nil {
		// status doesn't need the API key.
		if args[0] != "status" || cfg.DatabaseURL == "" {
			if args[0] == "status" {
				return fmt.Errorf("MIRROR_DATABASE_URL is not set")
			}
			return err
		}
	}
	if args[0] == "init" {
		return runInit(ctx, cfg, out)
	}
	return runStatus(ctx, cfg, out)
}

func main() {
	if err := run(context.Background(), os.Args[1:], os.Getenv, os.Stdout); err != nil {
		fmt.Fprintln(os.Stderr, "error:", err)
		os.Exit(1)
	}
}
