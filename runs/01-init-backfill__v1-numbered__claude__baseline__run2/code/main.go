package main

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"io"
	"os"
	"os/signal"
	"strings"

	_ "github.com/lib/pq"
)

type config struct {
	DatabaseURL string
	APIKey      string
	BaseURL     string
}

func loadConfig(getenv func(string) string) (config, error) {
	var missing []string
	cfg := config{
		DatabaseURL: getenv("MIRROR_DATABASE_URL"),
		APIKey:      getenv("MIRROR_API_KEY"),
		BaseURL:     getenv("MIRROR_API_URL"),
	}
	if cfg.DatabaseURL == "" {
		missing = append(missing, "MIRROR_DATABASE_URL")
	}
	if cfg.APIKey == "" {
		missing = append(missing, "MIRROR_API_KEY")
	}
	if len(missing) > 0 {
		return cfg, fmt.Errorf("missing required environment variable(s): %s", strings.Join(missing, ", "))
	}
	if cfg.BaseURL == "" {
		cfg.BaseURL = defaultBaseURL
	}
	return cfg, nil
}

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
		return errors.New("usage: mirror <init|status>")
	}
	cfg, err := loadConfig(getenv)
	if err != nil {
		return err
	}
	db, err := sql.Open("postgres", cfg.DatabaseURL)
	if err != nil {
		return fmt.Errorf("opening database: %w", err)
	}
	defer db.Close()

	switch args[0] {
	case "init":
		return runInit(ctx, NewClient(cfg.BaseURL, cfg.APIKey), db, out)
	default:
		return runStatus(ctx, db, out)
	}
}

func runInit(ctx context.Context, c *Client, db *sql.DB, out io.Writer) error {
	acct, err := c.CheckAccount(ctx)
	if err != nil {
		if errors.Is(err, ErrUnauthorized) {
			return fmt.Errorf("MIRROR_API_KEY was rejected by the API: %w", err)
		}
		return fmt.Errorf("checking API key: %w", err)
	}
	fmt.Fprintf(out, "API key OK (account %s)\n", acct)

	if err := db.PingContext(ctx); err != nil {
		return fmt.Errorf("connecting to database: %w", err)
	}
	if err := createTables(ctx, db); err != nil {
		return fmt.Errorf("creating tables: %w", err)
	}
	fmt.Fprintln(out, "Tables ready")

	var nc, ns int
	if err := c.EachCustomer(ctx, func(cs []Customer) error {
		nc += len(cs)
		return upsertCustomers(ctx, db, cs)
	}); err != nil {
		return fmt.Errorf("importing customers: %w", err)
	}
	fmt.Fprintf(out, "Imported %d customers\n", nc)

	if err := c.EachSubscription(ctx, func(ss []Subscription) error {
		ns += len(ss)
		return upsertSubscriptions(ctx, db, ss)
	}); err != nil {
		return fmt.Errorf("importing subscriptions: %w", err)
	}
	fmt.Fprintf(out, "Imported %d subscriptions\n", ns)
	return nil
}

func runStatus(ctx context.Context, db *sql.DB, out io.Writer) error {
	for _, t := range []string{"customers", "subscriptions"} {
		n, err := countRows(ctx, db, t)
		if err != nil {
			return err
		}
		fmt.Fprintf(out, "%s: %d\n", t, n)
	}
	return nil
}
