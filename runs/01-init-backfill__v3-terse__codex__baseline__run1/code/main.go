package main

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"os"
)

func main() {
	if err := run(context.Background(), os.Args[1:], os.Getenv, openPostgres, os.Stdout); err != nil {
		fmt.Fprintln(os.Stderr, "mirror:", err)
		os.Exit(1)
	}
}

type getenv func(string) string
type dbOpener func(string) (*sql.DB, error)

func run(ctx context.Context, args []string, env getenv, open dbOpener, out interface{ Write([]byte) (int, error) }) error {
	if len(args) != 1 || (args[0] != "init" && args[0] != "status") {
		return errors.New("usage: mirror <init|status>")
	}
	databaseURL := env("MIRROR_DATABASE_URL")
	if databaseURL == "" {
		return errors.New("MIRROR_DATABASE_URL is required")
	}
	var client *apiClient
	if args[0] == "init" {
		apiKey := env("MIRROR_API_KEY")
		if apiKey == "" {
			return errors.New("MIRROR_API_KEY is required")
		}
		client = newAPIClient(apiBaseURL(env), apiKey)
	}
	db, err := open(databaseURL)
	if err != nil {
		return fmt.Errorf("open database: %w", err)
	}
	defer db.Close()

	if args[0] == "status" {
		return printStatus(ctx, db, out)
	}

	if err := client.checkAccount(ctx); err != nil {
		return err
	}
	if err := createTables(ctx, db); err != nil {
		return err
	}
	if err := backfill(ctx, client, sqlStore{db}); err != nil {
		return err
	}
	_, err = fmt.Fprintln(out, "Backfill complete.")
	return err
}

func apiBaseURL(env getenv) string {
	if base := env("MIRROR_API_BASE_URL"); base != "" {
		return base
	}
	return "http://localhost:12111"
}
