# jobs

A Postgres-backed background job queue — Rails' Solid Queue, no Redis. Jobs are
rows; workers claim due rows with `FOR UPDATE SKIP LOCKED`, run a registered
handler, **delete on success**, and **retry with exponential backoff** on
failure until `max_attempts`, after which the row is left marked failed for
inspection.

## Install

```sh
go get github.com/a3yko/kit/jobs
```

## Usage

```go
q := jobs.New(pool, jobs.WithPollInterval(time.Second))
if err := q.EnsureSchema(ctx); err != nil { /* ... */ }

// Register handlers BEFORE Work (the registry isn't mutated once running).
q.Register("email.welcome", func(ctx context.Context, payload []byte) error {
    var u WelcomeArgs
    if err := json.Unmarshal(payload, &u); err != nil {
        return err
    }
    return mailer.SendWelcome(ctx, u)
})

// Enqueue from anywhere:
payload, _ := json.Marshal(WelcomeArgs{UserID: id})
q.Enqueue(ctx, "email.welcome", payload)
q.Enqueue(ctx, "email.welcome", payload, jobs.In(10*time.Minute)) // delayed
q.Enqueue(ctx, "report.build", payload, jobs.OnQueue("reports"))

// Run workers (blocks until ctx is cancelled):
q.Work(ctx, 5) // 5 concurrent workers
```

## Behaviour

- **Claim:** one due, un-failed job per worker via `FOR UPDATE SKIP LOCKED`, so
  workers never collide and you can run many across processes.
- **Success:** the row is deleted (the queue stays lean).
- **Failure:** `attempts++`, reschedule `run_at = now() + backoff(attempts)`
  (exponential, capped at 1h). After `max_attempts` (default 25) the row gets
  `failed_at` + `last_error` and is left for inspection.
- **Panics** in a handler are recovered and treated as a failure.
- A job whose `kind` has no registered handler fails (so a missing handler is
  visible, not silently dropped).

Options: `WithLogger`, `WithPollInterval`, `WithMaxAttempts`. Run a periodic job
(or cron) to delete old `failed_at` rows if you want them pruned.
