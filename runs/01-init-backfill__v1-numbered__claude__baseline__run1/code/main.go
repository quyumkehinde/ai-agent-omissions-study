package main

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"os/signal"
)

const usage = "usage: mirror <init|status>"

type Config struct {
	DatabaseURL string
	APIKey      string
	BaseURL     string
}

func loadConfig(getenv func(string) string) (Config, error) {
	cfg := Config{
		DatabaseURL: getenv("MIRROR_DATABASE_URL"),
		APIKey:      getenv("MIRROR_API_KEY"),
		BaseURL:     getenv("MIRROR_API_URL"),
	}
	var missing []string
	if cfg.DatabaseURL == "" {
		missing = append(missing, "MIRROR_DATABASE_URL")
	}
	if cfg.APIKey == "" {
		missing = append(missing, "MIRROR_API_KEY")
	}
	if len(missing) > 0 {
		return cfg, fmt.Errorf("missing required environment variable(s): %v", missing)
	}
	if cfg.BaseURL == "" {
		cfg.BaseURL = DefaultBaseURL
	}
	return cfg, nil
}

func main() {
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt)
	defer stop()
	if err := run(ctx, os.Args[1:], os.Getenv, os.Stdout); err != nil {
		fmt.Fprintln(os.Stderr, "error:", err)
		os.Exit(1)
	}
}

func run(ctx context.Context, args []string, getenv func(string) string, out io.Writer) error {
	if len(args) != 1 || (args[0] != "init" && args[0] != "status") {
		return errors.New(usage)
	}
	cfg, err := loadConfig(getenv)
	if err != nil {
		return err
	}
	switch args[0] {
	case "init":
		return runInit(ctx, cfg, out)
	default:
		return runStatus(ctx, cfg, out)
	}
}

func runInit(ctx context.Context, cfg Config, out io.Writer) error {
	api := NewClient(cfg.BaseURL, cfg.APIKey)
	acct, err := api.Account(ctx)
	if err != nil {
		return fmt.Errorf("checking API key: %w", err)
	}
	fmt.Fprintf(out, "API key OK (account %s)\n", acct.ID)

	st, err := OpenStore(ctx, cfg.DatabaseURL)
	if err != nil {
		return err
	}
	defer st.Close()
	return initialImport(ctx, api, st, out)
}

func initialImport(ctx context.Context, api *Client, st *Store, out io.Writer) error {
	if err := st.CreateTables(ctx); err != nil {
		return fmt.Errorf("creating tables: %w", err)
	}
	fmt.Fprintln(out, "Tables ready")

	var nc, ns int
	err := api.ListCustomers(ctx, func(cs []Customer) error {
		nc += len(cs)
		return st.UpsertCustomers(ctx, cs)
	})
	if err != nil {
		return fmt.Errorf("importing customers: %w", err)
	}
	fmt.Fprintf(out, "Imported %d customers\n", nc)

	err = api.ListSubscriptions(ctx, func(ss []Subscription) error {
		ns += len(ss)
		return st.UpsertSubscriptions(ctx, ss)
	})
	if err != nil {
		return fmt.Errorf("importing subscriptions: %w", err)
	}
	fmt.Fprintf(out, "Imported %d subscriptions\n", ns)
	return nil
}

func runStatus(ctx context.Context, cfg Config, out io.Writer) error {
	st, err := OpenStore(ctx, cfg.DatabaseURL)
	if err != nil {
		return err
	}
	defer st.Close()
	for _, t := range []string{"customers", "subscriptions"} {
		n, err := st.Count(ctx, t)
		if err != nil {
			return fmt.Errorf("counting %s (has `mirror init` been run?): %w", t, err)
		}
		fmt.Fprintf(out, "%s: %d\n", t, n)
	}
	return nil
}
