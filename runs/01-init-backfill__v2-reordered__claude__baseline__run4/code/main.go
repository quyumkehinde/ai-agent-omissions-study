package main

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"io"
	"os"
	"strings"

	"github.com/lib/pq"
)

const defaultAPIURL = "http://localhost:12111"

const usage = `usage: mirror <command>

commands:
  init     verify the API key, create tables, and import all existing data
  status   print the number of rows in each table

environment:
  MIRROR_DATABASE_URL  Postgres connection string (required)
  MIRROR_API_KEY       API key (required for init)
  MIRROR_API_URL       API base URL (default ` + defaultAPIURL + `)
`

func main() {
	os.Exit(run(context.Background(), os.Args[1:], os.Getenv, os.Stdout, os.Stderr))
}

func run(ctx context.Context, args []string, getenv func(string) string, stdout, stderr io.Writer) int {
	if len(args) != 1 || (args[0] != "init" && args[0] != "status") {
		fmt.Fprint(stderr, usage)
		return 2
	}
	cmd := args[0]

	required := []string{"MIRROR_DATABASE_URL"}
	if cmd == "init" {
		required = append(required, "MIRROR_API_KEY")
	}
	var missing []string
	for _, k := range required {
		if getenv(k) == "" {
			missing = append(missing, k)
		}
	}
	if len(missing) > 0 {
		fmt.Fprintf(stderr, "error: missing required environment variable(s): %s\n", strings.Join(missing, ", "))
		return 1
	}

	db, err := sql.Open("postgres", getenv("MIRROR_DATABASE_URL"))
	if err != nil {
		fmt.Fprintf(stderr, "error: invalid MIRROR_DATABASE_URL: %v\n", err)
		return 1
	}
	defer db.Close()

	switch cmd {
	case "init":
		baseURL := getenv("MIRROR_API_URL")
		if baseURL == "" {
			baseURL = defaultAPIURL
		}
		err = runInit(ctx, db, NewClient(strings.TrimRight(baseURL, "/"), getenv("MIRROR_API_KEY")), stdout)
	case "status":
		err = runStatus(ctx, db, stdout)
	}
	if err != nil {
		fmt.Fprintf(stderr, "error: %v\n", err)
		return 1
	}
	return 0
}

func runInit(ctx context.Context, db *sql.DB, c *Client, out io.Writer) error {
	if err := c.CheckAccount(ctx); err != nil {
		if errors.Is(err, ErrUnauthorized) {
			return fmt.Errorf("MIRROR_API_KEY was rejected by the API: %w", err)
		}
		return fmt.Errorf("checking API key: %w", err)
	}
	if err := CreateTables(ctx, db); err != nil {
		return fmt.Errorf("creating tables: %w", err)
	}
	fmt.Fprintln(out, "API key OK; tables ready")

	var nc, ns int
	if err := c.EachCustomerPage(ctx, func(p []Customer) error {
		nc += len(p)
		return UpsertCustomers(ctx, db, p)
	}); err != nil {
		return fmt.Errorf("importing customers: %w", err)
	}
	fmt.Fprintf(out, "imported %d customers\n", nc)

	if err := c.EachSubscriptionPage(ctx, func(p []Subscription) error {
		ns += len(p)
		return UpsertSubscriptions(ctx, db, p)
	}); err != nil {
		return fmt.Errorf("importing subscriptions: %w", err)
	}
	fmt.Fprintf(out, "imported %d subscriptions\n", ns)
	return nil
}

func runStatus(ctx context.Context, db *sql.DB, out io.Writer) error {
	for _, t := range tables {
		n, err := CountRows(ctx, db, t)
		if err != nil {
			var pe *pq.Error
			if errors.As(err, &pe) && pe.Code == "42P01" {
				return fmt.Errorf("table %q does not exist; run `mirror init` first", t)
			}
			return fmt.Errorf("counting %s: %w", t, err)
		}
		fmt.Fprintf(out, "%s: %d\n", t, n)
	}
	return nil
}
