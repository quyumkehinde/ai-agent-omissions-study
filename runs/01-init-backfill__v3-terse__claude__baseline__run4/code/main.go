package main

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"

	"github.com/jackc/pgx/v5"
)

const usage = "usage: mirror <init|status>"

type config struct {
	DatabaseURL string
	APIKey      string
	APIURL      string
}

// loadConfig reads required env vars and names every missing one.
func loadConfig(getenv func(string) string, needAPIKey bool) (config, error) {
	cfg := config{
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
	if needAPIKey && cfg.APIKey == "" {
		missing = append(missing, "MIRROR_API_KEY")
	}
	if len(missing) == 1 {
		return cfg, fmt.Errorf("missing required environment variable %s", missing[0])
	} else if len(missing) > 1 {
		return cfg, fmt.Errorf("missing required environment variables %s and %s", missing[0], missing[1])
	}
	return cfg, nil
}

func main() {
	if err := run(context.Background(), os.Args[1:], os.Getenv, os.Stdout); err != nil {
		fmt.Fprintln(os.Stderr, "mirror:", err)
		os.Exit(1)
	}
}

func run(ctx context.Context, args []string, getenv func(string) string, out io.Writer) error {
	if len(args) != 1 || (args[0] != "init" && args[0] != "status") {
		return errors.New(usage)
	}
	cfg, err := loadConfig(getenv, args[0] == "init")
	if err != nil {
		return err
	}
	if args[0] == "init" {
		return runInit(ctx, cfg, out)
	}
	return runStatus(ctx, cfg, out)
}

func runInit(ctx context.Context, cfg config, out io.Writer) error {
	client := NewClient(cfg.APIURL, cfg.APIKey)
	if err := client.CheckAccount(ctx); err != nil {
		return fmt.Errorf("checking MIRROR_API_KEY: %w", err)
	}
	conn, err := pgx.Connect(ctx, cfg.DatabaseURL)
	if err != nil {
		return fmt.Errorf("connecting to MIRROR_DATABASE_URL: %w", err)
	}
	defer conn.Close(ctx)
	if err := createSchema(ctx, conn); err != nil {
		return fmt.Errorf("creating tables: %w", err)
	}
	fmt.Fprintln(out, "tables ready: customers, subscriptions")
	c, s, err := Backfill(ctx, client, conn)
	if err != nil {
		return err
	}
	fmt.Fprintf(out, "backfilled %d customers, %d subscriptions\n", c, s)
	return nil
}

func runStatus(ctx context.Context, cfg config, out io.Writer) error {
	conn, err := pgx.Connect(ctx, cfg.DatabaseURL)
	if err != nil {
		return fmt.Errorf("connecting to MIRROR_DATABASE_URL: %w", err)
	}
	defer conn.Close(ctx)
	counts, err := RowCounts(ctx, conn)
	if err != nil {
		return err
	}
	for _, t := range tables {
		fmt.Fprintf(out, "%s: %d\n", t, counts[t])
	}
	return nil
}
