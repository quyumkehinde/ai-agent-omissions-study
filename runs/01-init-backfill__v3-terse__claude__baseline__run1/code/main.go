package main

import (
	"context"
	"database/sql"
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
	if len(args) != 1 {
		return errors.New(usage)
	}
	switch args[0] {
	case "init":
		return runInit(ctx, getenv, out)
	case "status":
		return runStatus(ctx, getenv, out)
	default:
		return fmt.Errorf("unknown command %q\n%s", args[0], usage)
	}
}

func requireEnv(getenv func(string) string, names ...string) (map[string]string, error) {
	vals := map[string]string{}
	var missing []string
	for _, n := range names {
		if v := getenv(n); v != "" {
			vals[n] = v
		} else {
			missing = append(missing, n)
		}
	}
	switch len(missing) {
	case 0:
		return vals, nil
	case 1:
		return nil, fmt.Errorf("%s is not set", missing[0])
	default:
		return nil, fmt.Errorf("%s and %s are not set", missing[0], missing[1])
	}
}

func openDB(ctx context.Context, url string) (*sql.DB, error) {
	db, err := sql.Open("postgres", url)
	if err != nil {
		return nil, fmt.Errorf("invalid MIRROR_DATABASE_URL: %w", err)
	}
	if err := db.PingContext(ctx); err != nil {
		db.Close()
		return nil, fmt.Errorf("connecting to database: %w", err)
	}
	return db, nil
}

func runInit(ctx context.Context, getenv func(string) string, out io.Writer) error {
	env, err := requireEnv(getenv, "MIRROR_DATABASE_URL", "MIRROR_API_KEY")
	if err != nil {
		return err
	}
	apiURL := getenv("MIRROR_API_URL")
	if apiURL == "" {
		apiURL = defaultAPIURL
	}
	client := NewClient(apiURL, env["MIRROR_API_KEY"])
	if err := client.CheckAccount(ctx); err != nil {
		if errors.Is(err, ErrUnauthorized) {
			return errors.New("MIRROR_API_KEY was rejected by the API (GET /v1/account returned 401)")
		}
		return fmt.Errorf("checking API key: %w", err)
	}
	db, err := openDB(ctx, env["MIRROR_DATABASE_URL"])
	if err != nil {
		return err
	}
	defer db.Close()
	if err := CreateSchema(ctx, db); err != nil {
		return fmt.Errorf("creating tables: %w", err)
	}
	fmt.Fprintln(out, "tables ready: customers, subscriptions")
	c, s, err := Backfill(ctx, client, db)
	if err != nil {
		return err
	}
	fmt.Fprintf(out, "backfilled %d customers, %d subscriptions\n", c, s)
	return nil
}

func runStatus(ctx context.Context, getenv func(string) string, out io.Writer) error {
	env, err := requireEnv(getenv, "MIRROR_DATABASE_URL")
	if err != nil {
		return err
	}
	db, err := openDB(ctx, env["MIRROR_DATABASE_URL"])
	if err != nil {
		return err
	}
	defer db.Close()
	counts, err := Counts(ctx, db)
	if err != nil {
		return err
	}
	for _, t := range tables {
		fmt.Fprintf(out, "%-14s %d\n", t, counts[t])
	}
	return nil
}
