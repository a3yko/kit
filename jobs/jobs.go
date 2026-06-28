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
//	q.Enqueue(ctx, "email.welcome", payload)          // run now
//	q.Enqueue(ctx, "report", payload, jobs.In(time.Hour)) // run later
//	q.Work(ctx, 5) // 5 concurrent workers; blocks until ctx is cancelled
//
// Beyond fire-and-forget enqueue it supports priorities (WithPriority),
// deduplicated enqueue (WithUnique — at most one live job per key), and
// self-rescheduling periodic jobs (RegisterPeriodic), e.g. an hourly billing
// sweep or a nightly cache purge. Claims take a lease (WithLease): a worker that
// crashes mid-job releases it automatically once the lease lapses, so there is
// no separate stuck-job sweeper to run.
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
	periodic     map[string]time.Duration
	pollInterval time.Duration
	maxAttempts  int
	lease        time.Duration
}

// Option configures a Queue.
type Option func(*Queue)

// WithLogger sets the logger used for worker/job errors (default slog.Default()).
func WithLogger(l *slog.Logger) Option { return func(q *Queue) { q.log = l } }

// WithPollInterval sets how often idle workers poll for new jobs (default 1s).
func WithPollInterval(d time.Duration) Option { return func(q *Queue) { q.pollInterval = d } }

// WithMaxAttempts sets the retry ceiling for newly enqueued jobs (default 25).
func WithMaxAttempts(n int) Option { return func(q *Queue) { q.maxAttempts = n } }

// WithLease sets how long a claimed job is hidden from other workers (default
// 5m). A job whose handler outlives the lease may be picked up and run again, so
// keep it comfortably above your slowest handler; set it lower to recover from
// crashes faster.
func WithLease(d time.Duration) Option { return func(q *Queue) { q.lease = d } }

// New returns a Queue backed by pool. Call EnsureSchema once at startup.
func New(pool *pgxpool.Pool, opts ...Option) *Queue {
	q := &Queue{
		pool:         pool,
		log:          slog.Default(),
		handlers:     make(map[string]Handler),
		periodic:     make(map[string]time.Duration),
		pollInterval: time.Second,
		maxAttempts:  25,
		lease:        5 * time.Minute,
	}
	for _, o := range opts {
		o(q)
	}
	return q
}

// EnsureSchema creates (or upgrades) the jobs table. Idempotent: safe to call on
// every boot, including against a table created by an earlier version.
func (q *Queue) EnsureSchema(ctx context.Context) error {
	const ddl = `CREATE TABLE IF NOT EXISTS jobs (
		id           bigint GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
		queue        text NOT NULL DEFAULT 'default',
		kind         text NOT NULL,
		payload      bytea NOT NULL DEFAULT '\x',
		priority     int NOT NULL DEFAULT 0,
		dedupe_key   text,
		run_at       timestamptz NOT NULL DEFAULT now(),
		attempts     int NOT NULL DEFAULT 0,
		max_attempts int NOT NULL DEFAULT 25,
		failed_at    timestamptz,
		last_error   text,
		created_at   timestamptz NOT NULL DEFAULT now()
	);
	ALTER TABLE jobs ADD COLUMN IF NOT EXISTS priority   int NOT NULL DEFAULT 0;
	ALTER TABLE jobs ADD COLUMN IF NOT EXISTS dedupe_key text;
	CREATE INDEX IF NOT EXISTS jobs_ready_idx ON jobs (priority DESC, run_at) WHERE failed_at IS NULL;
	CREATE UNIQUE INDEX IF NOT EXISTS jobs_dedupe_idx ON jobs (dedupe_key) WHERE failed_at IS NULL;`
	if _, err := q.pool.Exec(ctx, ddl); err != nil {
		return fmt.Errorf("jobs: ensure schema: %w", err)
	}
	return nil
}

// Register binds a handler to a job kind. Call it before Work; the registry is
// not safe to mutate once workers are running.
func (q *Queue) Register(kind string, h Handler) { q.handlers[kind] = h }

// RegisterPeriodic binds a handler that, after each successful run, is
// re-enqueued to run again after interval. Work seeds one on startup, so a
// single call gives you a recurring task (hourly sweep, nightly cleanup) with no
// external cron. It is deduplicated by kind: restarts and multiple workers won't
// pile up duplicates. A failing run retries on the normal backoff schedule and
// only reschedules the next occurrence once it eventually succeeds.
func (q *Queue) RegisterPeriodic(kind string, interval time.Duration, h Handler) {
	q.handlers[kind] = h
	q.periodic[kind] = interval
}

// EnqueueOption configures a single Enqueue.
type EnqueueOption func(*enqueueOpts)

type enqueueOpts struct {
	queue    string
	runAt    time.Time
	priority int
	unique   string
}

// At schedules the job to run no earlier than t.
func At(t time.Time) EnqueueOption { return func(o *enqueueOpts) { o.runAt = t } }

// In schedules the job to run after delay d.
func In(d time.Duration) EnqueueOption { return func(o *enqueueOpts) { o.runAt = time.Now().Add(d) } }

// OnQueue places the job on a named queue (default "default").
func OnQueue(name string) EnqueueOption { return func(o *enqueueOpts) { o.queue = name } }

// WithPriority sets the job priority; higher runs first among due jobs (default 0).
func WithPriority(p int) EnqueueOption { return func(o *enqueueOpts) { o.priority = p } }

// WithUnique deduplicates the job by key: while a job with this key is still
// live (pending or running, i.e. not yet succeeded or permanently failed), a
// second WithUnique enqueue of the same key is a no-op. Use it to make enqueue
// idempotent — e.g. "ensure a billing sweep is scheduled" called from many
// places or on every boot.
func WithUnique(key string) EnqueueOption { return func(o *enqueueOpts) { o.unique = key } }

// Enqueue adds a job of the given kind with an opaque payload. With WithUnique it
// is a no-op when a live job already holds the key.
func (q *Queue) Enqueue(ctx context.Context, kind string, payload []byte, opts ...EnqueueOption) error {
	o := enqueueOpts{queue: "default", runAt: time.Now()}
	for _, fn := range opts {
		fn(&o)
	}
	if payload == nil {
		payload = []byte{} // the column is NOT NULL; nil would insert SQL NULL
	}
	var dedupe any
	if o.unique != "" {
		dedupe = o.unique
	}
	// ON CONFLICT targets the partial unique index on dedupe_key; for NULL keys
	// (no WithUnique) there is never a conflict, so every such job inserts.
	const ins = `INSERT INTO jobs (queue, kind, payload, priority, dedupe_key, run_at, max_attempts)
		VALUES ($1, $2, $3, $4, $5, $6, $7)
		ON CONFLICT (dedupe_key) WHERE failed_at IS NULL DO NOTHING`
	if _, err := q.pool.Exec(ctx, ins, o.queue, kind, payload, o.priority, dedupe, o.runAt, q.maxAttempts); err != nil {
		return fmt.Errorf("jobs: enqueue %q: %w", kind, err)
	}
	return nil
}

// Stats is a point-in-time snapshot of the queue.
type Stats struct {
	Pending int64 // live jobs not yet permanently failed (includes leased/running)
	Failed  int64 // jobs that exhausted their attempts
}

// Stats counts pending and permanently-failed jobs.
func (q *Queue) Stats(ctx context.Context) (Stats, error) {
	const sel = `SELECT
		count(*) FILTER (WHERE failed_at IS NULL),
		count(*) FILTER (WHERE failed_at IS NOT NULL)
		FROM jobs`
	var s Stats
	if err := q.pool.QueryRow(ctx, sel).Scan(&s.Pending, &s.Failed); err != nil {
		return Stats{}, fmt.Errorf("jobs: stats: %w", err)
	}
	return s, nil
}

// PurgeFailed deletes permanently-failed jobs older than before, returning the
// number removed. Run it occasionally (e.g. from a RegisterPeriodic job) to keep
// the table from growing without bound.
func (q *Queue) PurgeFailed(ctx context.Context, before time.Time) (int64, error) {
	tag, err := q.pool.Exec(ctx, "DELETE FROM jobs WHERE failed_at IS NOT NULL AND failed_at < $1", before)
	if err != nil {
		return 0, fmt.Errorf("jobs: purge failed: %w", err)
	}
	return tag.RowsAffected(), nil
}

// Work runs concurrency worker goroutines until ctx is cancelled, then returns.
// Each worker claims and runs due jobs; SKIP LOCKED keeps them from colliding.
// Periodic jobs are seeded before the workers start.
func (q *Queue) Work(ctx context.Context, concurrency int) error {
	if concurrency < 1 {
		concurrency = 1
	}
	for kind := range q.periodic {
		if err := q.Enqueue(ctx, kind, nil, WithUnique(kind)); err != nil {
			q.log.Error("jobs: seed periodic", "kind", kind, "err", err)
		}
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
// found. The claim pushes run_at out by the lease so a crashed worker's job
// becomes due again automatically instead of being lost or run twice.
func (q *Queue) runOne(ctx context.Context) (claimed bool, err error) {
	const claim = `UPDATE jobs SET attempts = attempts + 1, run_at = now() + $1::interval
		WHERE id = (
			SELECT id FROM jobs
			WHERE failed_at IS NULL AND run_at <= now()
			ORDER BY priority DESC, run_at
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
	switch err := q.pool.QueryRow(ctx, claim, q.lease.String()).Scan(&id, &kind, &payload, &attempts, &maxAttempts); {
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
		// A periodic job reschedules its next run only after a clean success.
		if interval, ok := q.periodic[kind]; ok {
			if err := q.Enqueue(ctx, kind, payload, In(interval), WithUnique(kind)); err != nil {
				q.log.Error("jobs: reschedule periodic", "kind", kind, "err", err)
			}
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
