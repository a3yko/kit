package env

import (
	"os"
	"path/filepath"
	"testing"
)

func TestLoad(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, ".env")
	content := "" +
		"# a comment\n" +
		"\n" +
		"FOO=bar\n" +
		"export BAZ=qux\n" +
		"QUOTED=\"has spaces\"\n" +
		"SINGLE='single'\n" +
		"PRESET=fromfile\n"
	if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
		t.Fatal(err)
	}

	t.Setenv("PRESET", "fromenv") // already set → must NOT be overridden

	if err := Load(path); err != nil {
		t.Fatalf("Load: %v", err)
	}

	cases := map[string]string{
		"FOO":    "bar",
		"BAZ":    "qux",
		"QUOTED": "has spaces",
		"SINGLE": "single",
		"PRESET": "fromenv", // env wins over file
	}
	for k, want := range cases {
		if got := os.Getenv(k); got != want {
			t.Errorf("%s = %q, want %q", k, got, want)
		}
	}
}

func TestLoadMissingFileIsOK(t *testing.T) {
	if err := Load(filepath.Join(t.TempDir(), "nope.env")); err != nil {
		t.Errorf("Load(missing) = %v, want nil", err)
	}
}
