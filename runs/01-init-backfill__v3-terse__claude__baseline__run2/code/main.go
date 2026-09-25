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

const usage = `usage: mirror <command>

commands:
  init     validate credentials, create tables, and backfill all data
  status   print row counts per table

environment:
  MIRROR_DATABASE_URL  Postgres connection URL (required)
  MIRROR_API_KEY       API key (required for init)
  MIRROR_API_URL       API base URL (default ` + defaultAPIURL + `)
`

func main() {
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt)
	defer stop()
	os.Exit(run(ctx, os.Args[1:], os.Getenv, os.Stdout, os.Stderr))
}

func run(ctx context.Context, args []string, getenv func(string) string, out, errw io.Writer) int {
	if len(args) != 1 {
		fmt.Fprint(errw, usage)
		return 2
	}
	var err error
	switch args[0] {
	case "init":
		err = cmdInit(ctx, getenv, out)
	case "status":
		err = cmdStatus(ctx, getenv, out)
	case "-h", "--help", "help":
		fmt.Fprint(out, usage)
		return 0
	default:
		fmt.Fprintf(errw, "unknown command %q\n\n%s", args[0], usage)
		return 2
	}
	if err != nil {
		fmt.Fprintf(errw, "error: %v\n", err)
		return 1
	}
	return 0
}

func requireEnv(getenv func(string) string, names ...string) (map[string]string, error) {
	vals := map[string]string{}
	var missing []string
	for _, n := range names {
		v := getenv(n)
		if v == "" {
			missing = append(missing, n)
		}
		vals[n] = v
	}
	if len(missing) > 0 {
		return nil, fmt.Errorf("missing required environment variable(s): %v", missing)
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

	if _, err := client.Account(ctx); err != nil {
		if errors.Is(err, ErrUnauthorized) {
			return fmt.Errorf("MIRROR_API_KEY is invalid: %w", err)
		}
		return fmt.Errorf("checking API key: %w", err)
	}
	fmt.Fprintln(out, "API key OK")

	conn, err := pgx.Connect(ctx, env["MIRROR_DATABASE_URL"])
	if err != nil {
		return fmt.Errorf("connecting to database: %w", err)
	}
	defer conn.Close(ctx)

	if err := CreateSchema(ctx, conn); err != nil {
		return fmt.Errorf("creating tables: %w", err)
	}
	fmt.Fprintln(out, "Tables ready: customers, subscriptions")

	res, err := Backfill(ctx, client, conn)
	if err != nil {
		return err
	}
	fmt.Fprintf(out, "Backfilled %d customers, %d subscriptions\n", res.Customers, res.Subscriptions)
	return nil
}

func cmdStatus(ctx context.Context, getenv func(string) string, out io.Writer) error {
	env, err := requireEnv(getenv, "MIRROR_DATABASE_URL")
	if err != nil {
		return err
	}
	conn, err := pgx.Connect(ctx, env["MIRROR_DATABASE_URL"])
	if err != nil {
		return fmt.Errorf("connecting to database: %w", err)
	}
	defer conn.Close(ctx)

	for _, t := range tables {
		n, err := CountRows(ctx, conn, t)
		if err != nil {
			return fmt.Errorf("counting %s (has `mirror init` been run?): %w", t, err)
		}
		fmt.Fprintf(out, "%s: %d\n", t, n)
	}
	return nil
}
