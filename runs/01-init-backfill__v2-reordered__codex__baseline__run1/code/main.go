package main

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"io"
	"os"
)

func main() {
	if err := run(context.Background(), os.Args[1:], os.Getenv, openPostgres, newAPIClient, os.Stdout); err != nil {
		fmt.Fprintln(os.Stderr, "mirror:", err)
		os.Exit(1)
	}
}

type envFunc func(string) string
type dbOpener func(string) (*sql.DB, error)
type apiFactory func(string) *apiClient

func run(ctx context.Context, args []string, getenv envFunc, openDB dbOpener, api apiFactory, out io.Writer) error {
	if len(args) != 1 || (args[0] != "init" && args[0] != "status") {
		return errors.New("usage: mirror <init|status>")
	}
	databaseURL := getenv("MIRROR_DATABASE_URL")
	if databaseURL == "" {
		return errors.New("MIRROR_DATABASE_URL is not set")
	}
	apiKey := getenv("MIRROR_API_KEY")
	if apiKey == "" {
		return errors.New("MIRROR_API_KEY is not set")
	}
	var client *apiClient
	if args[0] == "init" {
		client = api(apiKey)
		if err := client.checkAccount(ctx); err != nil {
			return err
		}
	}
	db, err := openDB(databaseURL)
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

	if err := createTables(ctx, db); err != nil {
		return err
	}
	if err := importCustomers(ctx, db, client); err != nil {
		return err
	}
	if err := importSubscriptions(ctx, db, client); err != nil {
		return err
	}
	return nil
}
