package main

import (
	"context"
	"database/sql"
	"io"
	"strings"
	"testing"
)

func TestRunNamesMissingConfiguration(t *testing.T) {
	open := func(string) (*sql.DB, error) { t.Fatal("database should not be opened"); return nil, nil }
	api := func(string) *apiClient { t.Fatal("API should not be constructed"); return nil }
	for _, test := range []struct {
		name string
		env  map[string]string
		want string
	}{
		{"database", map[string]string{}, "MIRROR_DATABASE_URL is not set"},
		{"key", map[string]string{"MIRROR_DATABASE_URL": "postgres://example"}, "MIRROR_API_KEY is not set"},
	} {
		t.Run(test.name, func(t *testing.T) {
			err := run(context.Background(), []string{"init"}, func(k string) string { return test.env[k] }, open, api, io.Discard)
			if err == nil || !strings.Contains(err.Error(), test.want) {
				t.Fatalf("run() error = %v, want %q", err, test.want)
			}
		})
	}
}
