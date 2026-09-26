package main

import (
	"bytes"
	"context"
	"database/sql"
	"errors"
	"strings"
	"testing"
)

func TestRunNamesMissingDatabaseURL(t *testing.T) {
	err := run(context.Background(), []string{"init"}, func(string) string { return "" }, func(string) (*sql.DB, error) { return nil, errors.New("should not open") }, &bytes.Buffer{})
	if err == nil || !strings.Contains(err.Error(), "MIRROR_DATABASE_URL") {
		t.Fatalf("got %v", err)
	}
}

func TestRunNamesMissingAPIKey(t *testing.T) {
	env := func(k string) string {
		if k == "MIRROR_DATABASE_URL" {
			return "postgres://example"
		}
		return ""
	}
	err := run(context.Background(), []string{"init"}, env, func(string) (*sql.DB, error) { return &sql.DB{}, nil }, &bytes.Buffer{})
	if err == nil || !strings.Contains(err.Error(), "MIRROR_API_KEY") {
		t.Fatalf("got %v", err)
	}
}
