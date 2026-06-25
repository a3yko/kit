package jobs

import (
	"context"
	"errors"
	"io"
	"log/slog"
	"os"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
)

func testPool(t *testing.T) *pgxpool.Pool {
	t.Helper()
	url := os.Getenv("TEST_DATABASE_URL")
	if url == "" {
		t.Skip("set TEST_DATABASE_URL to run DB integration tests")
	}
	pool, err := pgxpool.New(context.Background(), url)
	if err != nil {
		t.Fatalf("connect: %v", err)
	}
	t.Cleanup(pool.Close)
	return pool
}

func freshQueue(t *testing.T, opts ...Option) *Queue {
	t.Helper()
	pool := testPool(t)
	ctx := context.Background()
	_, _ = pool.Exec(ctx, "DROP TABLE IF EXISTS jobs")
	t.Cleanup(func() { _, _ = pool.Exec(ctx, "DROP TABLE IF EXISTS jobs") })

	silent := WithLogger(slog.New(slog.NewTextHandler(io.Discard, nil)))
	q := New(pool, append([]Option{silent, WithPollInterval(20 * time.Millisecond)}, opts...)...)
	if err := q.EnsureSchema(ctx); err != nil {
		t.Fatalf("EnsureSchema: %v", err)
	}
	return q
}

func TestEnqueueRunsHandlerAndDeletes(t *testing.T) {
	q := freshQueue(t)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	got := make(chan []byte, 1)
	q.Register("greet", func(_ context.Context, p []byte) error {
		got <- p
		return nil
	})
	if err := q.Enqueue(context.Background(), "greet", []byte("hi")); err != nil {
		t.Fatalf("Enqueue: %v", err)
	}

	go func() { _ = q.Work(ctx, 2) }()

	select {
	case p := <-got:
		if string(p) != "hi" {
			t.Errorf("payload = %q, want hi", p)
		}
	case <-time.After(3 * time.Second):
		t.Fatal("job was not processed")
	}

	// Successful jobs are deleted.
	var remaining int
	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		_ = q.pool.QueryRow(context.Background(), "SELECT count(*) FROM jobs").Scan(&remaining)
		if remaining == 0 {
			break
		}
		time.Sleep(20 * time.Millisecond)
	}
	if remaining != 0 {
		t.Errorf("jobs remaining = %d, want 0 (deleted on success)", remaining)
	}
}

func TestFailureMarksFailedAfterMaxAttempts(t *testing.T) {
	q := freshQueue(t, WithMaxAttempts(1))
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	q.Register("boom", func(_ context.Context, _ []byte) error {
		return errors.New("nope")
	})
	if err := q.Enqueue(context.Background(), "boom", nil); err != nil {
		t.Fatalf("Enqueue: %v", err)
	}

	go func() { _ = q.Work(ctx, 1) }()

	deadline := time.Now().Add(3 * time.Second)
	for time.Now().Before(deadline) {
		var failed bool
		var lastErr *string
		err := q.pool.QueryRow(context.Background(),
			"SELECT failed_at IS NOT NULL, last_error FROM jobs LIMIT 1").Scan(&failed, &lastErr)
		if err == nil && failed {
			if lastErr == nil || *lastErr == "" {
				t.Error("expected last_error to be recorded")
			}
			return
		}
		time.Sleep(20 * time.Millisecond)
	}
	t.Fatal("job was not marked failed within the deadline")
}
