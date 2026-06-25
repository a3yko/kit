// Package cache is a Postgres-backed key/value cache — Rails' Solid Cache, no
// Redis. Values are bytes with an optional TTL; expired entries are ignored on
// read and removed by Purge (run it periodically, e.g. as a recurring job).
package cache

import (
	"context"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

// Cache is a handle to the cache table.
type Cache struct {
	pool  *pgxpool.Pool
	table string
}

// New returns a Cache backed by pool. Call EnsureSchema once at startup.
func New(pool *pgxpool.Pool) *Cache {
	return &Cache{pool: pool, table: "cache_entries"}
}

// EnsureSchema creates the cache table if it does not exist (idempotent).
func (c *Cache) EnsureSchema(ctx context.Context) error {
	const ddl = `CREATE TABLE IF NOT EXISTS cache_entries (
		key        text PRIMARY KEY,
		value      bytea NOT NULL,
		expires_at timestamptz
	);
	CREATE INDEX IF NOT EXISTS cache_entries_expires_idx ON cache_entries (expires_at);`
	if _, err := c.pool.Exec(ctx, ddl); err != nil {
		return fmt.Errorf("cache: ensure schema: %w", err)
	}
	return nil
}

// Get returns the value for key. found is false if the key is absent or expired.
func (c *Cache) Get(ctx context.Context, key string) (value []byte, found bool, err error) {
	const q = `SELECT value FROM cache_entries
		WHERE key = $1 AND (expires_at IS NULL OR expires_at > now())`
	err = c.pool.QueryRow(ctx, q, key).Scan(&value)
	switch {
	case err == pgx.ErrNoRows:
		return nil, false, nil
	case err != nil:
		return nil, false, fmt.Errorf("cache: get %q: %w", key, err)
	}
	return value, true, nil
}

// Set stores value under key. A ttl <= 0 means no expiry. Existing keys are
// overwritten.
func (c *Cache) Set(ctx context.Context, key string, value []byte, ttl time.Duration) error {
	var expires *time.Time
	if ttl > 0 {
		t := time.Now().Add(ttl)
		expires = &t
	}
	const q = `INSERT INTO cache_entries (key, value, expires_at) VALUES ($1, $2, $3)
		ON CONFLICT (key) DO UPDATE SET value = EXCLUDED.value, expires_at = EXCLUDED.expires_at`
	if _, err := c.pool.Exec(ctx, q, key, value, expires); err != nil {
		return fmt.Errorf("cache: set %q: %w", key, err)
	}
	return nil
}

// Delete removes key (absent keys are not an error).
func (c *Cache) Delete(ctx context.Context, key string) error {
	if _, err := c.pool.Exec(ctx, "DELETE FROM cache_entries WHERE key = $1", key); err != nil {
		return fmt.Errorf("cache: delete %q: %w", key, err)
	}
	return nil
}

// Purge deletes all expired entries and returns how many were removed.
func (c *Cache) Purge(ctx context.Context) (int64, error) {
	tag, err := c.pool.Exec(ctx, "DELETE FROM cache_entries WHERE expires_at IS NOT NULL AND expires_at <= now()")
	if err != nil {
		return 0, fmt.Errorf("cache: purge: %w", err)
	}
	return tag.RowsAffected(), nil
}
