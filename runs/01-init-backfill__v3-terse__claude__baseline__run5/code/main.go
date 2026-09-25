package main

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"os/signal"
)

const defaultAPIURL = "http://localhost:12111"

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
		return errors.New("usage: mirror <init|status>")
	}
	switch args[0] {
	case "init":
		return cmdInit(ctx, getenv, out)
	case "status":
		return cmdStatus(ctx, getenv, out)
	default:
		return fmt.Errorf("unknown command %q (usage: mirror <init|status>)", args[0])
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
	if len(missing) == 1 {
		return nil, fmt.Errorf("%s is not set", missing[0])
	} else if len(missing) > 1 {
		return nil, fmt.Errorf("%s and %s are not set", missing[0], missing[1])
	}
	return vals, nil
}

func cmdInit(ctx context.Context, getenv func(string) string, out io.Writer) error {
	env, err := requireEnv(getenv, "MIRROR_DATABASE_URL", "MIRROR_API_KEY")
	if err != nil {
		return err
	}
	apiURL := getenv("MIRROR_API_URL")
	if apiURL == "" {
		apiURL = defaultAPIURL
	}
	client := NewClient(apiURL, env["MIRROR_API_KEY"])
	if err := client.Account(ctx); err != nil {
		if errors.Is(err, ErrUnauthorized) {
			return errors.New("MIRROR_API_KEY was rejected by the API (GET /v1/account returned 401)")
		}
		return fmt.Errorf("checking API key: %w", err)
	}
	fmt.Fprintln(out, "API key OK")

	db, err := openDB(ctx, env["MIRROR_DATABASE_URL"])
	if err != nil {
		return err
	}
	defer db.Close()
	if err := createSchema(ctx, db); err != nil {
		return fmt.Errorf("creating tables: %w", err)
	}
	fmt.Fprintln(out, "Tables ready")

	return backfill(ctx, client, db, out)
}

func cmdStatus(ctx context.Context, getenv func(string) string, out io.Writer) error {
	env, err := requireEnv(getenv, "MIRROR_DATABASE_URL")
	if err != nil {
		return err
	}
	db, err := openDB(ctx, env["MIRROR_DATABASE_URL"])
	if err != nil {
		return err
	}
	defer db.Close()
	counts, err := rowCounts(ctx, db)
	if err != nil {
		return err
	}
	for _, t := range tableNames {
		fmt.Fprintf(out, "%-14s %d\n", t, counts[t])
	}
	return nil
}
