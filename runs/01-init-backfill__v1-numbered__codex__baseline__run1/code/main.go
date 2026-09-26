package main

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"strings"
	"time"

	_ "github.com/lib/pq"
)

const defaultAPIURL = "http://localhost:12111"

func main() {
	if err := run(context.Background(), os.Args[1:], os.Stdout, os.Getenv, http.DefaultClient); err != nil {
		fmt.Fprintln(os.Stderr, "mirror:", err)
		os.Exit(1)
	}
}

// run is kept separate from main so command behavior can be tested without
// changing process environment or exiting the test process.
func run(ctx context.Context, args []string, out io.Writer, getenv func(string) string, httpClient *http.Client) error {
	if len(args) != 1 || (args[0] != "init" && args[0] != "status") {
		return errors.New("usage: mirror <init|status>")
	}

	databaseURL := strings.TrimSpace(getenv("MIRROR_DATABASE_URL"))
	if databaseURL == "" {
		return errors.New("MIRROR_DATABASE_URL is required")
	}
	apiKey := strings.TrimSpace(getenv("MIRROR_API_KEY"))
	if apiKey == "" {
		return errors.New("MIRROR_API_KEY is required")
	}

	db, err := sql.Open("postgres", databaseURL)
	if err != nil {
		return fmt.Errorf("open Postgres: %w", err)
	}
	defer db.Close()
	if err := db.PingContext(ctx); err != nil {
		return fmt.Errorf("connect to Postgres: %w", err)
	}

	if args[0] == "status" {
		return printStatus(ctx, db, out)
	}

	baseURL := strings.TrimRight(strings.TrimSpace(getenv("MIRROR_API_URL")), "/")
	if baseURL == "" {
		baseURL = defaultAPIURL
	}
	api := &apiClient{baseURL: baseURL, key: apiKey, client: httpClient}
	if err := api.checkAccount(ctx); err != nil {
		return err
	}
	if err := createTables(ctx, db); err != nil {
		return err
	}
	if err := importAll(ctx, db, api); err != nil {
		return err
	}
	fmt.Fprintln(out, "mirror initialized")
	return nil
}

func createTables(ctx context.Context, db *sql.DB) error {
	const customers = `CREATE TABLE IF NOT EXISTS customers (
 id TEXT PRIMARY KEY, object TEXT NOT NULL, email TEXT NOT NULL, name TEXT NOT NULL, created BIGINT NOT NULL
)`
	const subscriptions = `CREATE TABLE IF NOT EXISTS subscriptions (
 id TEXT PRIMARY KEY, object TEXT NOT NULL, customer TEXT NOT NULL, status TEXT NOT NULL, created BIGINT NOT NULL
)`
	if _, err := db.ExecContext(ctx, customers); err != nil {
		return fmt.Errorf("create customers table: %w", err)
	}
	if _, err := db.ExecContext(ctx, subscriptions); err != nil {
		return fmt.Errorf("create subscriptions table: %w", err)
	}
	return nil
}

func importAll(ctx context.Context, db *sql.DB, api *apiClient) error {
	customers, err := api.customers(ctx)
	if err != nil {
		return fmt.Errorf("fetch customers: %w", err)
	}
	subscriptions, err := api.subscriptions(ctx)
	if err != nil {
		return fmt.Errorf("fetch subscriptions: %w", err)
	}
	tx, err := db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("begin import transaction: %w", err)
	}
	defer tx.Rollback()
	for _, c := range customers {
		_, err = tx.ExecContext(ctx, `INSERT INTO customers (id, object, email, name, created) VALUES ($1,$2,$3,$4,$5)
ON CONFLICT (id) DO UPDATE SET object=EXCLUDED.object, email=EXCLUDED.email, name=EXCLUDED.name, created=EXCLUDED.created`, c.ID, c.Object, c.Email, c.Name, c.Created)
		if err != nil {
			return fmt.Errorf("store customer %q: %w", c.ID, err)
		}
	}
	for _, s := range subscriptions {
		_, err = tx.ExecContext(ctx, `INSERT INTO subscriptions (id, object, customer, status, created) VALUES ($1,$2,$3,$4,$5)
ON CONFLICT (id) DO UPDATE SET object=EXCLUDED.object, customer=EXCLUDED.customer, status=EXCLUDED.status, created=EXCLUDED.created`, s.ID, s.Object, s.Customer, s.Status, s.Created)
		if err != nil {
			return fmt.Errorf("store subscription %q: %w", s.ID, err)
		}
	}
	if err := tx.Commit(); err != nil {
		return fmt.Errorf("commit import: %w", err)
	}
	return nil
}

func printStatus(ctx context.Context, db *sql.DB, out io.Writer) error {
	for _, table := range []string{"customers", "subscriptions"} {
		var count int64
		if err := db.QueryRowContext(ctx, "SELECT COUNT(*) FROM "+table).Scan(&count); err != nil {
			return fmt.Errorf("count %s: %w", table, err)
		}
		if _, err := fmt.Fprintf(out, "%s: %d\n", table, count); err != nil {
			return err
		}
	}
	return nil
}

// sleep is a variable solely to make retry behavior deterministic in tests.
var sleep = time.Sleep
