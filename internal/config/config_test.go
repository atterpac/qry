package config

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// helper to set up a temp config dir and write an initial config file.
func setupTempConfig(t *testing.T, content string) (dir string, cleanup func()) {
	t.Helper()
	dir = t.TempDir()
	path := filepath.Join(dir, "config.yaml")
	if content != "" {
		if err := os.WriteFile(path, []byte(content), 0644); err != nil {
			t.Fatal(err)
		}
	}
	t.Setenv("QRY_CONFIG", path)
	return dir, func() {}
}

func TestRoundTripPreservesComments(t *testing.T) {
	const input = `# Top-level comment about the config
theme: tokyonight-night # inline theme comment
active_profile: dev
max_history: 50

# Profiles section
profiles:
  dev:
    engine: postgres
    dsn: "host=localhost" # dev DSN
`
	setupTempConfig(t, input)

	cfg, err := Load()
	if err != nil {
		t.Fatal(err)
	}

	// Mutate a value.
	cfg.Theme = "gruvbox"

	if err := cfg.Save(); err != nil {
		t.Fatal(err)
	}

	data, err := os.ReadFile(ConfigPath())
	if err != nil {
		t.Fatal(err)
	}
	out := string(data)

	// The new value must be present.
	if !strings.Contains(out, "gruvbox") {
		t.Errorf("expected theme gruvbox in output:\n%s", out)
	}
	// Comments must survive.
	for _, comment := range []string{
		"# Top-level comment about the config",
		"# inline theme comment",
		"# dev DSN",
		"# Profiles section",
	} {
		if !strings.Contains(out, comment) {
			t.Errorf("missing comment %q in output:\n%s", comment, out)
		}
	}
}

func TestRoundTripAddsNewKeys(t *testing.T) {
	const input = `# Minimal config
theme: tokyonight-night
`
	setupTempConfig(t, input)

	cfg, err := Load()
	if err != nil {
		t.Fatal(err)
	}

	// Add a profile (new key tree).
	cfg.SaveProfile("prod", ConnectionConfig{
		Engine: EnginePostgres,
		DSN:    "host=prod",
	})
	cfg.ActiveProfile = "prod"

	if err := cfg.Save(); err != nil {
		t.Fatal(err)
	}

	data, err := os.ReadFile(ConfigPath())
	if err != nil {
		t.Fatal(err)
	}
	out := string(data)

	if !strings.Contains(out, "prod") {
		t.Errorf("expected new profile 'prod' in output:\n%s", out)
	}
	if !strings.Contains(out, "# Minimal config") {
		t.Errorf("expected original comment preserved in output:\n%s", out)
	}
}

func TestRoundTripRemovesDeletedKeys(t *testing.T) {
	const input = `# Keep this comment
theme: tokyonight-night
active_profile: keep

# Profiles
profiles:
  keep:
    engine: sqlite
    path: keep.db
  remove_me:
    engine: postgres
    dsn: "host=gone"
`
	setupTempConfig(t, input)

	cfg, err := Load()
	if err != nil {
		t.Fatal(err)
	}

	if err := cfg.DeleteProfile("remove_me"); err != nil {
		t.Fatal(err)
	}

	if err := cfg.Save(); err != nil {
		t.Fatal(err)
	}

	data, err := os.ReadFile(ConfigPath())
	if err != nil {
		t.Fatal(err)
	}
	out := string(data)

	if strings.Contains(out, "remove_me") {
		t.Errorf("deleted profile should not appear in output:\n%s", out)
	}
	if !strings.Contains(out, "# Keep this comment") {
		t.Errorf("expected comment preserved after deletion:\n%s", out)
	}
	if !strings.Contains(out, "# Profiles") {
		t.Errorf("expected profiles comment preserved:\n%s", out)
	}
}

func TestSaveWithNoExistingFile(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "config.yaml")
	t.Setenv("QRY_CONFIG", path)

	cfg := DefaultConfig()
	cfg.SaveProfile("test", ConnectionConfig{Engine: EngineSQLite, Path: "test.db"})
	cfg.ActiveProfile = "test"

	if err := cfg.Save(); err != nil {
		t.Fatal(err)
	}

	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	out := string(data)

	if !strings.Contains(out, "test.db") {
		t.Errorf("expected test.db in output:\n%s", out)
	}

	// Subsequent save should also work (rawDoc gets populated).
	cfg.Theme = "catppuccin"
	if err := cfg.Save(); err != nil {
		t.Fatal(err)
	}
	data, err = os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(data), "catppuccin") {
		t.Errorf("expected catppuccin after second save:\n%s", string(data))
	}
}
