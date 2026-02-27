package config

import (
	"bytes"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"sync"

	"gopkg.in/yaml.v3"
)

const DefaultTheme = "tokyonight-night"

// EngineType identifies the database engine.
type EngineType string

const (
	EnginePostgres  EngineType = "postgres"
	EngineMySQL     EngineType = "mysql"
	EngineSQLite    EngineType = "sqlite"
	EngineSurrealDB EngineType = "surrealdb"
)

// CommandOutputType defines how command output should be displayed.
type CommandOutputType string

const (
	OutputLog CommandOutputType = "log"
)

// CommandConfig defines a user-configured command.
type CommandConfig struct {
	Description string            `yaml:"description,omitempty"`
	Cmd         string            `yaml:"cmd"`
	Output      CommandOutputType `yaml:"output,omitempty"`
	Confirm     bool              `yaml:"confirm,omitempty"`
}

// SavedQuery is a named SQL query stored per-profile.
type SavedQuery struct {
	Name  string `yaml:"name"`
	Query string `yaml:"query"`
}

// AuthConfig holds authentication settings (for SurrealDB etc).
type AuthConfig struct {
	Username string `yaml:"username,omitempty"`
	Password string `yaml:"password,omitempty"`
}

// ConnectionConfig holds database connection settings for a profile.
type ConnectionConfig struct {
	Engine       EngineType                  `yaml:"engine"`
	DSN          string                      `yaml:"dsn,omitempty"`
	Path         string                      `yaml:"path,omitempty"`          // SQLite file path
	URL          string                      `yaml:"url,omitempty"`           // SurrealDB websocket URL
	Namespace    string                      `yaml:"namespace,omitempty"`     // SurrealDB namespace
	Database     string                      `yaml:"database,omitempty"`      // SurrealDB/default database
	Auth         AuthConfig                  `yaml:"auth,omitempty"`
	SavedQueries []SavedQuery               `yaml:"saved_queries,omitempty"`
	Commands     map[string]CommandConfig    `yaml:"commands,omitempty"`
}

// ExpandEnv expands environment variables in sensitive fields.
func (c ConnectionConfig) ExpandEnv() ConnectionConfig {
	return ConnectionConfig{
		Engine:       c.Engine,
		DSN:          os.ExpandEnv(c.DSN),
		Path:         os.ExpandEnv(c.Path),
		URL:          os.ExpandEnv(c.URL),
		Namespace:    c.Namespace,
		Database:     c.Database,
		Auth: AuthConfig{
			Username: os.ExpandEnv(c.Auth.Username),
			Password: os.ExpandEnv(c.Auth.Password),
		},
		SavedQueries: c.SavedQueries,
		Commands:     c.Commands,
	}
}

// Bookmark represents a saved resource shortcut.
type Bookmark struct {
	Type     string `yaml:"type"`               // "table", "query", "database"
	Name     string `yaml:"name"`
	Schema   string `yaml:"schema,omitempty"`
	Database string `yaml:"database,omitempty"`
}

// Config represents the application configuration.
type Config struct {
	Theme         string                         `yaml:"theme"`
	ActiveProfile string                         `yaml:"active_profile,omitempty"`
	MaxHistory    int                            `yaml:"max_history,omitempty"`
	Profiles      map[string]ConnectionConfig    `yaml:"profiles,omitempty"`
	Commands      map[string]CommandConfig        `yaml:"commands,omitempty"`
	Bookmarks     []Bookmark                      `yaml:"bookmarks,omitempty"`

	defaulted bool       // true when ensureDefaults() created placeholder profiles
	rawDoc    *yaml.Node // preserved document node from last Load (nil when no file existed)
	mu        sync.Mutex // protects concurrent Save calls
}

// HasUserProfiles reports whether the config has user-defined profiles
// (as opposed to auto-generated defaults).
func (c *Config) HasUserProfiles() bool {
	return !c.defaulted
}

// DefaultConfig returns a config with sensible defaults but no profiles.
func DefaultConfig() *Config {
	return &Config{
		Theme:      DefaultTheme,
		MaxHistory: 100,
	}
}

// Load reads the config file from disk.
func Load() (*Config, error) {
	path := ConfigPath()

	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			cfg := DefaultConfig()
			cfg.ensureDefaults()
			return cfg, nil
		}
		return nil, fmt.Errorf("reading config: %w", err)
	}

	// Parse into a yaml.Node first to preserve comments, then decode
	// into the struct.
	var doc yaml.Node
	if err := yaml.Unmarshal(data, &doc); err != nil {
		return nil, fmt.Errorf("parsing config: %w", err)
	}

	cfg := &Config{}
	if err := doc.Decode(cfg); err != nil {
		return nil, fmt.Errorf("decoding config: %w", err)
	}
	cfg.rawDoc = &doc

	cfg.ensureDefaults()
	return cfg, nil
}

func (c *Config) ensureDefaults() {
	if c.Profiles == nil || len(c.Profiles) == 0 {
		c.defaulted = true
		c.Profiles = map[string]ConnectionConfig{
			"default": {Engine: EngineSQLite, Path: "~/.config/qry/default.db"},
		}
		c.ActiveProfile = "default"
	}
	if c.MaxHistory <= 0 {
		c.MaxHistory = 100
	}
	if c.ActiveProfile == "" {
		for name := range c.Profiles {
			c.ActiveProfile = name
			break
		}
	} else if _, ok := c.Profiles[c.ActiveProfile]; !ok {
		for name := range c.Profiles {
			c.ActiveProfile = name
			break
		}
	}
}

// Save writes the config to disk, preserving any YAML comments from the
// original file when possible.
func (c *Config) Save() error {
	c.mu.Lock()
	defer c.mu.Unlock()

	if err := EnsureConfigDir(); err != nil {
		return fmt.Errorf("creating config dir: %w", err)
	}

	var data []byte
	if c.rawDoc != nil {
		// Encode struct into a fresh node, then merge values into the
		// preserved node tree so comments survive.
		fresh, err := encodeToNode(c)
		if err != nil {
			return fmt.Errorf("encoding config: %w", err)
		}
		mergeNode(c.rawDoc.Content[0], fresh.Content[0])
		var buf bytes.Buffer
		enc := yaml.NewEncoder(&buf)
		enc.SetIndent(2)
		if err := enc.Encode(c.rawDoc); err != nil {
			return fmt.Errorf("marshaling config: %w", err)
		}
		enc.Close()
		data = buf.Bytes()
	} else {
		// No prior file — produce clean YAML.
		var buf bytes.Buffer
		enc := yaml.NewEncoder(&buf)
		enc.SetIndent(2)
		if err := enc.Encode(c); err != nil {
			return fmt.Errorf("marshaling config: %w", err)
		}
		enc.Close()
		data = buf.Bytes()
		// Store the node for subsequent saves.
		var doc yaml.Node
		if err := yaml.Unmarshal(data, &doc); err == nil {
			c.rawDoc = &doc
		}
	}

	path := ConfigPath()
	return os.WriteFile(path, data, 0644)
}

// GetProfile returns a profile by name.
func (c *Config) GetProfile(name string) (ConnectionConfig, bool) {
	if c.Profiles == nil {
		return ConnectionConfig{}, false
	}
	profile, ok := c.Profiles[name]
	return profile, ok
}

// GetActiveProfile returns the active profile name and its configuration.
func (c *Config) GetActiveProfile() (string, ConnectionConfig) {
	if c.Profiles == nil || c.ActiveProfile == "" {
		return "default", ConnectionConfig{Engine: EngineSQLite, Path: "~/.config/qry/default.db"}
	}
	profile, ok := c.Profiles[c.ActiveProfile]
	if !ok {
		for name, cfg := range c.Profiles {
			return name, cfg
		}
		return "default", ConnectionConfig{Engine: EngineSQLite, Path: "~/.config/qry/default.db"}
	}
	return c.ActiveProfile, profile
}

// SetActiveProfile sets the active profile by name.
func (c *Config) SetActiveProfile(name string) error {
	if c.Profiles == nil {
		return fmt.Errorf("no profiles configured")
	}
	if _, ok := c.Profiles[name]; !ok {
		return fmt.Errorf("profile %q not found", name)
	}
	c.ActiveProfile = name
	return nil
}

// SaveProfile saves or updates a profile.
func (c *Config) SaveProfile(name string, cfg ConnectionConfig) {
	if c.Profiles == nil {
		c.Profiles = make(map[string]ConnectionConfig)
	}
	c.Profiles[name] = cfg
}

// DeleteProfile deletes a profile by name.
func (c *Config) DeleteProfile(name string) error {
	if c.Profiles == nil {
		return fmt.Errorf("profile %q not found", name)
	}
	if _, ok := c.Profiles[name]; !ok {
		return fmt.Errorf("profile %q not found", name)
	}
	if c.ActiveProfile == name {
		return fmt.Errorf("cannot delete active profile %q", name)
	}
	delete(c.Profiles, name)
	return nil
}

// ListProfiles returns a sorted list of profile names.
func (c *Config) ListProfiles() []string {
	if c.Profiles == nil {
		return nil
	}
	names := make([]string, 0, len(c.Profiles))
	for name := range c.Profiles {
		names = append(names, name)
	}
	sort.Strings(names)
	return names
}

// GetMergedCommands returns global commands merged with profile-specific commands.
func (c *Config) GetMergedCommands(profileName string) map[string]CommandConfig {
	merged := make(map[string]CommandConfig)
	for name, cmd := range c.Commands {
		merged[name] = cmd
	}
	if profile, ok := c.Profiles[profileName]; ok {
		for name, cmd := range profile.Commands {
			merged[name] = cmd
		}
	}
	return merged
}

// ListCommandNames returns a sorted list of all command names for a profile.
func (c *Config) ListCommandNames(profileName string) []string {
	merged := c.GetMergedCommands(profileName)
	names := make([]string, 0, len(merged))
	for name := range merged {
		names = append(names, name)
	}
	sort.Strings(names)
	return names
}

// AddBookmark adds a bookmark if it doesn't already exist.
func (c *Config) AddBookmark(b Bookmark) bool {
	for _, existing := range c.Bookmarks {
		if existing.Type == b.Type && existing.Name == b.Name && existing.Schema == b.Schema {
			return false
		}
	}
	c.Bookmarks = append(c.Bookmarks, b)
	return true
}

// RemoveBookmarkMatch removes a bookmark that matches the given fields.
func (c *Config) RemoveBookmarkMatch(b Bookmark) {
	for i, existing := range c.Bookmarks {
		if existing.Type == b.Type && existing.Name == b.Name && existing.Schema == b.Schema {
			c.Bookmarks = append(c.Bookmarks[:i], c.Bookmarks[i+1:]...)
			return
		}
	}
}

// SavedQueryForProfile adds a saved query to the given profile.
func (c *Config) SavedQueryForProfile(profileName, name, query string) {
	if profile, ok := c.Profiles[profileName]; ok {
		// Update if exists
		for i, sq := range profile.SavedQueries {
			if sq.Name == name {
				profile.SavedQueries[i].Query = query
				c.Profiles[profileName] = profile
				return
			}
		}
		profile.SavedQueries = append(profile.SavedQueries, SavedQuery{Name: name, Query: query})
		c.Profiles[profileName] = profile
	}
}

// DeleteSavedQuery removes a saved query from the given profile.
func (c *Config) DeleteSavedQuery(profileName, name string) {
	if profile, ok := c.Profiles[profileName]; ok {
		for i, sq := range profile.SavedQueries {
			if sq.Name == name {
				profile.SavedQueries = append(profile.SavedQueries[:i], profile.SavedQueries[i+1:]...)
				c.Profiles[profileName] = profile
				return
			}
		}
	}
}

// --- YAML node merge helpers ---

// encodeToNode marshals a value into a yaml.Node document.
func encodeToNode(v any) (*yaml.Node, error) {
	var doc yaml.Node
	raw, err := yaml.Marshal(v)
	if err != nil {
		return nil, err
	}
	if err := yaml.Unmarshal(raw, &doc); err != nil {
		return nil, err
	}
	return &doc, nil
}

// mergeNode recursively updates preserved with values from fresh,
// keeping comments from preserved intact.
func mergeNode(preserved, fresh *yaml.Node) {
	if preserved.Kind != fresh.Kind {
		// Kind changed (e.g. scalar became mapping); replace wholesale
		// but keep the preserved head/foot comments.
		hc, lc, fc := preserved.HeadComment, preserved.LineComment, preserved.FootComment
		*preserved = *fresh
		preserved.HeadComment = hc
		preserved.LineComment = lc
		preserved.FootComment = fc
		return
	}

	switch preserved.Kind {
	case yaml.MappingNode:
		mergeMapping(preserved, fresh)
	case yaml.SequenceNode:
		// Replace sequence content wholesale — item-level comments are
		// not preserved because array elements rarely carry them.
		preserved.Content = fresh.Content
	case yaml.ScalarNode:
		preserved.Value = fresh.Value
		preserved.Tag = fresh.Tag
	}
}

// mergeMapping synchronises a preserved mapping node with a fresh one.
// Existing keys keep their comments; new keys are appended; removed keys
// are deleted.
func mergeMapping(preserved, fresh *yaml.Node) {
	// Build index of fresh keys → value node index.
	freshIdx := make(map[string]int, len(fresh.Content)/2)
	for i := 0; i < len(fresh.Content)-1; i += 2 {
		freshIdx[fresh.Content[i].Value] = i
	}

	// Walk preserved: update existing, mark removed.
	var kept []*yaml.Node
	seen := make(map[string]bool, len(preserved.Content)/2)
	for i := 0; i < len(preserved.Content)-1; i += 2 {
		key := preserved.Content[i].Value
		if fi, ok := freshIdx[key]; ok {
			// Key still exists — recurse into value.
			mergeNode(preserved.Content[i+1], fresh.Content[fi+1])
			kept = append(kept, preserved.Content[i], preserved.Content[i+1])
			seen[key] = true
		}
		// else: key was removed, drop it
	}

	// Append new keys from fresh that weren't in preserved.
	for i := 0; i < len(fresh.Content)-1; i += 2 {
		key := fresh.Content[i].Value
		if !seen[key] {
			kept = append(kept, fresh.Content[i], fresh.Content[i+1])
		}
	}

	preserved.Content = kept
}

// ConfigDir returns the configuration directory path following XDG spec.
func ConfigDir() string {
	if xdg := os.Getenv("XDG_CONFIG_HOME"); xdg != "" {
		return filepath.Join(xdg, "qry")
	}

	home, err := os.UserHomeDir()
	if err != nil {
		return ".qry"
	}

	configDir := filepath.Join(home, ".config")
	if info, err := os.Stat(configDir); err == nil && info.IsDir() {
		return filepath.Join(configDir, "qry")
	}

	return filepath.Join(home, ".qry")
}

// ConfigPath returns the full path to the config file.
func ConfigPath() string {
	if p := os.Getenv("QRY_CONFIG"); p != "" {
		return p
	}
	return filepath.Join(ConfigDir(), "config.yaml")
}

// EnsureConfigDir creates the config directory if it doesn't exist.
func EnsureConfigDir() error {
	return os.MkdirAll(ConfigDir(), 0755)
}
