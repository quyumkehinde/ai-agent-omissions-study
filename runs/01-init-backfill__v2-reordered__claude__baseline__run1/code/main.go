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

func main() {
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt)
	defer stop()
	if err := run(ctx, os.Args[1:], os.Getenv, os.Stdout); err != nil {
		fmt.Fprintln(os.Stderr, "error:", err)
		os.Exit(1)
	}
}

// requireEnv returns the values of the named variables, or an error naming every missing one.
func requireEnv(getenv func(string) string, names ...string) ([]string, error) {
	vals := make([]string, len(names))
	var missing []string
	for i, n := range names {
		if vals[i] = getenv(n); vals[i] == "" {
			missing = append(missing, n)
		}
	}
	if len(missing) > 0 {
		return nil, fmt.Errorf("missing required environment variable(s): %v", missing)
	}
	return vals, nil
}

func run(ctx context.Context, args []string, getenv func(string) string, out io.Writer) error {
	if len(args) != 1 || (args[0] != "init" && args[0] != "status") {
		return errors.New(usage)
	}
	env, err := requireEnv(getenv, "MIRROR_DATABASE_URL", "MIRROR_API_KEY")
	if err != nil {
		return err
	}
	if args[0] == "status" {
		return runStatus(ctx, out, env[0])
	}
	apiURL := getenv("MIRROR_API_URL")
	if apiURL == "" {
		apiURL = defaultAPIURL
	}
	return runInit(ctx, out, env[0], NewClient(apiURL, env[1]))
}

func runInit(ctx context.Context, out io.Writer, dsn string, c *Client) error {
	acct, err := c.CheckAccount(ctx)
	if err != nil {
		if errors.Is(err, ErrUnauthorized) {
			return fmt.Errorf("MIRROR_API_KEY was rejected by the API: %w", err)
		}
		return fmt.Errorf("checking API key: %w", err)
	}
	fmt.Fprintf(out, "API key OK (account %s)\n", acct)

	st, err := OpenStore(ctx, dsn)
	if err != nil {
		return err
	}
	defer st.Close()
	if err := st.CreateTables(ctx); err != nil {
		return fmt.Errorf("creating tables: %w", err)
	}

	var nc, ns int
	if err := c.ListCustomers(ctx, func(cs []Customer) error {
		nc += len(cs)
		return st.UpsertCustomers(ctx, cs)
	}); err != nil {
		return fmt.Errorf("importing customers: %w", err)
	}
	fmt.Fprintf(out, "imported %d customers\n", nc)
	if err := c.ListSubscriptions(ctx, func(ss []Subscription) error {
		ns += len(ss)
		return st.UpsertSubscriptions(ctx, ss)
	}); err != nil {
		return fmt.Errorf("importing subscriptions: %w", err)
	}
	fmt.Fprintf(out, "imported %d subscriptions\n", ns)
	return nil
}

func runStatus(ctx context.Context, out io.Writer, dsn string) error {
	st, err := OpenStore(ctx, dsn)
	if err != nil {
		return err
	}
	defer st.Close()
	for _, t := range tables {
		n, err := st.Count(ctx, t)
		if err != nil {
			return fmt.Errorf("counting %s (has `mirror init` been run?): %w", t, err)
		}
		fmt.Fprintf(out, "%s: %d\n", t, n)
	}
	return nil
}
