// Package env is a tiny, zero-dependency .env loader. It reads KEY=VALUE pairs
// from a file into the process environment without overriding variables already
// set — so the real environment (systemd, the shell, CI) always wins and the
// .env file is just a local-dev convenience.
//
// It deliberately does nothing else: no config struct, no type coercion, no
// interpolation. Pair it with your own config.Load() that reads os.Getenv.
package env

import (
	"bufio"
	"os"
	"strings"
)

// Load reads KEY=VALUE pairs from the file at path into the process environment,
// skipping variables that are already set. A missing file is not an error (it
// returns nil) — the file is optional. Supports blank lines, "#" comments, an
// optional "export " prefix, and single/double-quoted values.
func Load(path string) error {
	f, err := os.Open(path)
	if err != nil {
		if os.IsNotExist(err) {
			return nil // no file is fine
		}
		return err
	}
	defer f.Close()

	sc := bufio.NewScanner(f)
	for sc.Scan() {
		line := strings.TrimSpace(sc.Text())
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		line = strings.TrimPrefix(line, "export ")
		k, v, ok := strings.Cut(line, "=")
		if !ok {
			continue
		}
		k, v = strings.TrimSpace(k), strings.TrimSpace(v)
		if len(v) >= 2 && (v[0] == '"' && v[len(v)-1] == '"' || v[0] == '\'' && v[len(v)-1] == '\'') {
			v = v[1 : len(v)-1]
		}
		if _, exists := os.LookupEnv(k); !exists {
			if err := os.Setenv(k, v); err != nil {
				return err
			}
		}
	}
	return sc.Err()
}
