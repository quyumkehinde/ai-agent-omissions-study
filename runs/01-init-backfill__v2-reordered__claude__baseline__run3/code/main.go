package main

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"os/signal"

	"github.com/jackc/pgx/v5"
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

// requireEnv returns the values of the named variables, or an error naming all that are unset.
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
	if len(missing) > 0 {
		return nil, fmt.Errorf("missing required environment variable(s): %v", missing)
	}
	return vals, nil
}

func run(ctx context.Context, args []string, getenv func(string) string, out io.Writer) error {
	if len(args) != 1 {
		return errors.New(usage)
	}
	switch args[0] {
	case "init":
		env, err := requireEnv(getenv, "MIRROR_DATABASE_URL", "MIRROR_API_KEY")
		if err != nil {
			return err
		}
		apiURL := getenv("MIRROR_API_URL")
		if apiURL == "" {
			apiURL = defaultAPIURL
		}
		return runInit(ctx, NewClient(apiURL, env["MIRROR_API_KEY"]), env["MIRROR_DATABASE_URL"], out)
	case "status":
		env, err := requireEnv(getenv, "MIRROR_DATABASE_URL")
		if err != nil {
			return err
		}
		return runStatus(ctx, env["MIRROR_DATABASE_URL"], out)
	default:
		return fmt.Errorf("unknown command %q\n%s", args[0], usage)
	}
}

func runInit(ctx context.Context, c *Client, dbURL string, out io.Writer) error {
	// Validate the key before touching the database.
	if err := c.CheckAccount(ctx); err != nil {
		if errors.Is(err, ErrUnauthorized) {
			return fmt.Errorf("MIRROR_API_KEY was rejected by the API: %w", err)
		}
		return fmt.Errorf("checking API key: %w", err)
	}
	db, err := pgx.Connect(ctx, dbURL)
	if err != nil {
		return fmt.Errorf("connecting to MIRROR_DATABASE_URL: %w", err)
	}
	defer db.Close(ctx)
	if err := CreateSchema(ctx, db); err != nil {
		return fmt.Errorf("creating tables: %w", err)
	}
	fmt.Fprintln(out, "API key OK; tables created.")
	nc, ns, err := Backfill(ctx, c, db)
	if err != nil {
		return err
	}
	fmt.Fprintf(out, "Imported %d customers and %d subscriptions.\n", nc, ns)
	return nil
}

func runStatus(ctx context.Context, dbURL string, out io.Writer) error {
	db, err := pgx.Connect(ctx, dbURL)
	if err != nil {
		return fmt.Errorf("connecting to MIRROR_DATABASE_URL: %w", err)
	}
	defer db.Close(ctx)
	counts, err := Counts(ctx, db)
	if err != nil {
		return err
	}
	for _, t := range tables {
		fmt.Fprintf(out, "%-14s %d\n", t, counts[t])
	}
	return nil
}
