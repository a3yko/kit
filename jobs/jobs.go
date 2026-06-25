// Package jobs is a Postgres-backed background job queue — Rails' Solid Queue,
// no Redis. Jobs are rows; workers claim due rows with FOR UPDATE SKIP LOCKED,
// run a registered handler, delete on success, and retry with exponential
// backoff on failure until max attempts, after which the row is left marked
// failed for inspection.
//
// Register all handlers before calling Work:
//
//	q := jobs.New(pool)
//	q.EnsureSchema(ctx)
//	q.Register("email.welcome", sendWelcome)
//	q.Enqueue(ctx, "email.welcome", payload)
//	q.Work(ctx, 5) // 5 concurrent workers; blocks until ctx is cancelled
package jobs

import (
	"context"
	"fmt"
	"log/slog"
	"sync"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

// Handler processes one job's payload. Returning an error triggers a retry.
type Handler func(ctx context.Context, payload []byte) error

// Queue is a handle to the jobs table plus the worker's handler registry.
type Queue struct {
	pool         *pgxpool.Pool
	log          *slog.Logger
	handlers     map[string]Handler
	pollInterval time.Duration
	maxAttempts  int
}

// Option configures a Queue.
type Option func(*Queue)

// WithLogger sets the logger used for worker/job errors (default slog.Default()).
func WithLogger(l *slog.Logger) Option { return func(q *Queue) { q.log = l } }

// WithPollInterval sets how often idle workers poll for new jobs (default 1s).
func WithPollInterval(d time.Duration) Option { return func(q *Queue) { q.pollInterval = d } }

// WithMaxAttempts sets the retry ceiling for newly enqueued jobs (default 25).
func WithMaxAttempts(n int) Option { return func(q *Queue) { q.maxAttempts = n } }

// New returns a Queue backed by pool. Call EnsureSchema once at startup.
func New(pool *pgxpool.Pool, opts ...Option) *Queue {
	q := &Queue{
		pool:         pool,
		log:          slog.Default(),
		handlers:     make(map[string]Handler),
		pollInterval: time.Second,
		maxAttempts:  25,
	}
	for _, o := range opts {
		o(q)
	}
	return q
}

// EnsureSchema creates the jobs table if it does not exist (idempotent).
func (q *Queue) EnsureSchema(ctx context.Context) error {
	const ddl = `CREATE TABLE IF NOT EXISTS jobs (
		id           bigint GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
		queue        text NOT NULL DEFAULT 'default',
		kind         text NOT NULL,
		payload      bytea NOT NULL DEFAULT '\x',
		run_at       timestamptz NOT NULL DEFAULT now(),
		attempts     int NOT NULL DEFAULT 0,
		max_attempts int NOT NULL DEFAULT 25,
		failed_at    timestamptz,
		last_error   text,
		created_at   timestamptz NOT NULL DEFAULT now()
	);
	CREATE INDEX IF NOT EXISTS jobs_ready_idx ON jobs (run_at) WHERE failed_at IS NULL;`
	if _, err := q.pool.Exec(ctx, ddl); err != nil {
		return fmt.Errorf("jobs: ensure schema: %w", err)
	}
	return nil
}

// Register binds a handler to a job kind. Call it before Work; the registry is
// not safe to mutate once workers are running.
func (q *Queue) Register(kind string, h Handler) { q.handlers[kind] = h }

// Option for a single Enqueue.
type EnqueueOption func(*enqueueOpts)

type enqueueOpts struct {
	queue string
	runAt time.Time
}

// At schedules the job to run no earlier than t.
func At(t time.Time) EnqueueOption { return func(o *enqueueOpts) { o.runAt = t } }

// In schedules the job to run after delay d.
func In(d time.Duration) EnqueueOption { return func(o *enqueueOpts) { o.runAt = time.Now().Add(d) } }

// OnQueue places the job on a named queue (default "default").
func OnQueue(name string) EnqueueOption { return func(o *enqueueOpts) { o.queue = name } }

// Enqueue adds a job of the given kind with an opaque payload.
func (q *Queue) Enqueue(ctx context.Context, kind string, payload []byte, opts ...EnqueueOption) error {
	o := enqueueOpts{queue: "default", runAt: time.Now()}
	for _, fn := range opts {
		fn(&o)
	}
	if payload == nil {
		payload = []byte{} // the column is NOT NULL; nil would insert SQL NULL
	}
	const ins = `INSERT INTO jobs (queue, kind, payload, run_at, max_attempts)
		VALUES ($1, $2, $3, $4, $5)`
	if _, err := q.pool.Exec(ctx, ins, o.queue, kind, payload, o.runAt, q.maxAttempts); err != nil {
		return fmt.Errorf("jobs: enqueue %q: %w", kind, err)
	}
	return nil
}

// Work runs concurrency worker goroutines until ctx is cancelled, then returns.
// Each worker claims and runs due jobs; SKIP LOCKED keeps them from colliding.
func (q *Queue) Work(ctx context.Context, concurrency int) error {
	if concurrency < 1 {
		concurrency = 1
	}
	var wg sync.WaitGroup
	for range concurrency {
		wg.Go(func() { q.worker(ctx) }) // Go 1.25: Add(1)+go+Done() in one call
	}
	wg.Wait()
	return ctx.Err()
}

func (q *Queue) worker(ctx context.Context) {
	ticker := time.NewTicker(q.pollInterval)
	defer ticker.Stop()
	for {
		// Drain all ready jobs, then wait for the next tick.
		for {
			claimed, err := q.runOne(ctx)
			if err != nil {
				q.log.Error("jobs: worker error", "err", err)
				break
			}
			if !claimed || ctx.Err() != nil {
				break
			}
		}
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
		}
	}
}

// runOne claims and runs at most one due job. claimed reports whether a job was
// found.
func (q *Queue) runOne(ctx context.Context) (claimed bool, err error) {
	const claim = `UPDATE jobs SET attempts = attempts + 1
		WHERE id = (
			SELECT id FROM jobs
			WHERE failed_at IS NULL AND run_at <= now()
			ORDER BY run_at
			FOR UPDATE SKIP LOCKED
			LIMIT 1
		)
		RETURNING id, kind, payload, attempts, max_attempts`

	var (
		id          int64
		kind        string
		payload     []byte
		attempts    int
		maxAttempts int
	)
	switch err := q.pool.QueryRow(ctx, claim).Scan(&id, &kind, &payload, &attempts, &maxAttempts); {
	case err == pgx.ErrNoRows:
		return false, nil
	case err != nil:
		return false, fmt.Errorf("jobs: claim: %w", err)
	}

	runErr := q.dispatch(ctx, kind, payload)
	if runErr == nil {
		if _, err := q.pool.Exec(ctx, "DELETE FROM jobs WHERE id = $1", id); err != nil {
			return true, fmt.Errorf("jobs: delete %d: %w", id, err)
		}
		return true, nil
	}

	// Failure: retry with backoff, or give up after max attempts.
	q.log.Error("jobs: job failed", "id", id, "kind", kind, "attempt", attempts, "err", runErr)
	if attempts >= maxAttempts {
		_, err := q.pool.Exec(ctx,
			"UPDATE jobs SET failed_at = now(), last_error = $2 WHERE id = $1", id, runErr.Error())
		return true, err
	}
	_, err = q.pool.Exec(ctx,
		"UPDATE jobs SET run_at = now() + $2::interval, last_error = $3 WHERE id = $1",
		id, backoff(attempts).String(), runErr.Error())
	return true, err
}

// dispatch runs the handler for kind, converting a missing handler or a panic
// into an error so the worker loop can record it.
func (q *Queue) dispatch(ctx context.Context, kind string, payload []byte) (err error) {
	h, ok := q.handlers[kind]
	if !ok {
		return fmt.Errorf("jobs: no handler registered for %q", kind)
	}
	defer func() {
		if v := recover(); v != nil {
			err = fmt.Errorf("jobs: handler panicked: %v", v)
		}
	}()
	return h(ctx, payload)
}

// backoff is an exponential retry delay capped at 1 hour.
func backoff(attempt int) time.Duration {
	d := time.Duration(1<<min(attempt, 12)) * time.Second
	if d > time.Hour {
		return time.Hour
	}
	return d
}
