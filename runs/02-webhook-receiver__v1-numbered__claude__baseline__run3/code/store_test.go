package main

import (
	"context"
	"os"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
)

// Runs against a real Postgres when MIRROR_TEST_DATABASE_URL is set.
func TestPostgresStore(t *testing.T) {
	url := os.Getenv("MIRROR_TEST_DATABASE_URL")
	if url == "" {
		t.Skip("MIRROR_TEST_DATABASE_URL not set")
	}
	ctx := context.Background()
	st, err := NewPostgresStore(ctx, url)
	if err != nil {
		t.Fatal(err)
	}
	defer st.Close()
	st.pool.Exec(ctx, `DELETE FROM events WHERE id = 'evt_pg_test'`)

	payload := []byte(`{"id":"evt_pg_test","type":"customer.created","created":1735689600}`)
	ev := Event{ID: "evt_pg_test", Type: "customer.created", Created: time.Unix(1735689600, 0).UTC(), Payload: payload}
	for i := 0; i < 2; i++ { // second save is a redelivery
		if err := st.Save(ctx, ev); err != nil {
			t.Fatalf("save %d: %v", i, err)
		}
	}
	var n int
	var typ string
	var created time.Time
	var got string
	err = st.pool.QueryRow(ctx, `SELECT count(*) OVER (), type, created_at, payload::text FROM events WHERE id = 'evt_pg_test'`).Scan(&n, &typ, &created, &got)
	if err != nil {
		t.Fatal(err)
	}
	if n != 1 || typ != ev.Type || !created.Equal(ev.Created) || got == "" {
		t.Fatalf("unexpected row: n=%d typ=%s created=%v payload=%s", n, typ, created, got)
	}
	var _ *pgxpool.Pool = st.pool
}
