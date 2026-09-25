package main

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"io"
	"os"

	_ "github.com/jackc/pgx/v5/stdlib"
)

const usage = "usage: mirror <init|status>"

func main() {
	os.Exit(run(context.Background(), os.Args[1:], os.Getenv, os.Stdout, os.Stderr))
}

func run(ctx context.Context, args []string, getenv func(string) string, out, errw io.Writer) int {
	if len(args) != 1 || (args[0] != "init" && args[0] != "status") {
		fmt.Fprintln(errw, usage)
		return 2
	}
	if err := execute(ctx, args[0], getenv, out); err != nil {
		fmt.Fprintln(errw, "error:", err)
		return 1
	}
	return 0
}

func execute(ctx context.Context, cmd string, getenv func(string) string, out io.Writer) error {
	dbURL, key := getenv("MIRROR_DATABASE_URL"), getenv("MIRROR_API_KEY")
	var missing []string
	if dbURL == "" {
		missing = append(missing, "MIRROR_DATABASE_URL")
	}
	// status does not talk to the API, but the key is still required for a uniform setup.
	if key == "" {
		missing = append(missing, "MIRROR_API_KEY")
	}
	if len(missing) > 0 {
		return fmt.Errorf("missing required environment variable(s): %v", missing)
	}
	base := getenv("MIRROR_API_URL")
	if base == "" {
		base = defaultBaseURL
	}

	client := NewClient(base, key)
	if cmd == "init" {
		// Validate the key before touching the database.
		if err := client.CheckAccount(ctx); err != nil {
			if errors.Is(err, ErrUnauthorized) {
				return fmt.Errorf("MIRROR_API_KEY was rejected by the API: %w", err)
			}
			return fmt.Errorf("checking API key: %w", err)
		}
	}

	db, err := sql.Open("pgx", dbURL)
	if err != nil {
		return err
	}
	defer db.Close()
	if err := db.PingContext(ctx); err != nil {
		return fmt.Errorf("connecting to Postgres: %w", err)
	}
	store := &Store{DB: db}

	if cmd == "init" {
		return initCmd(ctx, client, store, out)
	}
	return statusCmd(ctx, store, out)
}

func initCmd(ctx context.Context, client *Client, store *Store, out io.Writer) error {
	if err := store.CreateSchema(ctx); err != nil {
		return fmt.Errorf("creating tables: %w", err)
	}
	fmt.Fprintln(out, "API key OK; tables created.")

	nc := 0
	err := client.EachCustomerPage(ctx, func(p []Customer) error {
		nc += len(p)
		return store.UpsertCustomers(ctx, p)
	})
	if err != nil {
		return fmt.Errorf("importing customers: %w", err)
	}
	fmt.Fprintf(out, "Imported %d customers.\n", nc)

	ns := 0
	err = client.EachSubscriptionPage(ctx, func(p []Subscription) error {
		ns += len(p)
		return store.UpsertSubscriptions(ctx, p)
	})
	if err != nil {
		return fmt.Errorf("importing subscriptions: %w", err)
	}
	fmt.Fprintf(out, "Imported %d subscriptions.\n", ns)
	return nil
}

func statusCmd(ctx context.Context, store *Store, out io.Writer) error {
	for _, t := range Tables {
		n, err := store.Count(ctx, t)
		if err != nil {
			return err
		}
		fmt.Fprintf(out, "%s: %d\n", t, n)
	}
	return nil
}
