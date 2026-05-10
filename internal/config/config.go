package config

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"time"

	"gopkg.in/yaml.v3"
)

// DefaultCacheFreshness is the freshness window for the SWR cache when the
// user hasn't set one. Cache hits younger than this serve without kicking
// off a background refresh.
const DefaultCacheFreshness = time.Minute

// Config holds the application configuration
type Config struct {
	Roots            []string `yaml:"roots"`
	Ignore           []string `yaml:"ignore"`
	Editor           string   `yaml:"editor"`
	PageSize         int      `yaml:"pageSize,omitempty"`
	IncludeWorktrees bool     `yaml:"includeWorktrees,omitempty"`
	Actions          []Action `yaml:"actions,omitempty"`
	// CacheFreshness controls the SWR cache window — cache reads younger
	// than this are served without firing a background refresh. Accepts
	// any Go duration string (`30s`, `1m`, `10m`). Empty or unparseable
	// values fall back to DefaultCacheFreshness.
	CacheFreshness string `yaml:"cacheFreshness,omitempty"`
}

// CacheFreshnessDuration returns the parsed cache freshness window, or the
// default when the config value is empty / unparseable / non-positive.
func (c *Config) CacheFreshnessDuration() time.Duration {
	if c.CacheFreshness == "" {
		return DefaultCacheFreshness
	}
	d, err := time.ParseDuration(c.CacheFreshness)
	if err != nil || d <= 0 {
		return DefaultCacheFreshness
	}
	return d
}

// Action is a keyboard-driven command bound to a key. Each action runs against
// the currently selected repo in the TUI. Exactly one of Run or Clipboard
// must be set:
//   - Run: shell command executed via tea.ExecProcess (terminal handover).
//   - Clipboard: text written to the system clipboard, no command launched.
//
// `{path}` in either value is substituted with the absolute repo path.
type Action struct {
	Key       string `yaml:"key"`
	Label     string `yaml:"label"`
	Run       string `yaml:"run,omitempty"`
	Clipboard string `yaml:"clipboard,omitempty"`
}

// IsClipboard reports whether the action copies to clipboard rather than
// launching a process.
func (a Action) IsClipboard() bool { return a.Clipboard != "" }

// Value returns the template string (either Run or Clipboard).
func (a Action) Value() string {
	if a.Clipboard != "" {
		return a.Clipboard
	}
	return a.Run
}

// DefaultActions returns the built-in action set used when config.yml has no
// `actions:` block. Uses cfg.Editor for the Enter binding so existing users
// see no behavioural change.
func DefaultActions(editor string) []Action {
	if editor == "" {
		editor = "code"
	}
	return []Action{
		{Key: "enter", Label: "Open in editor", Run: editor + " {path}"},
		{Key: "alt+c", Label: "Copy path", Clipboard: "{path}"},
		{Key: "alt+g", Label: "Open in gitui", Run: "gitui -d {path}"},
	}
}

// defaultConfig returns sensible defaults
// By default, scan the current directory so git-scope works out of the box
func defaultConfig() *Config {
	// Get current working directory as default root
	cwd, err := os.Getwd()
	if err != nil {
		// Fallback to home directory if cwd fails
		cwd, _ = os.UserHomeDir()
	}

	return &Config{
		Roots: []string{cwd},
		Ignore: []string{
			"node_modules",
			".next",
			"dist",
			"build",
			"target",
			".venv",
			"vendor",
		},
		Editor:   "code",
		PageSize: 15,
	}
}

// Load reads configuration from a YAML file
// Returns default config if file doesn't exist
func Load(path string) (*Config, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		// If file does not exist, return defaults (no error)
		if os.IsNotExist(err) {
			return defaultConfig(), nil
		}
		return nil, fmt.Errorf("read config: %w", err)
	}

	cfg := defaultConfig()
	if err := yaml.Unmarshal(data, cfg); err != nil {
		return nil, fmt.Errorf("parse config: %w", err)
	}

	// Expand ~ in paths
	for i, root := range cfg.Roots {
		cfg.Roots[i] = expandPath(root)
	}

	// Ensure pageSize has a sensible value
	if cfg.PageSize <= 0 {
		cfg.PageSize = 15
	}

	// Populate defaults when the user hasn't defined any actions yet.
	if len(cfg.Actions) == 0 {
		cfg.Actions = DefaultActions(cfg.Editor)
	}

	return cfg, nil
}

// expandPath expands ~ to user home directory and resolves relative paths
func expandPath(path string) string {
	// Handle ~ prefix
	if strings.HasPrefix(path, "~/") {
		home, err := os.UserHomeDir()
		if err != nil {
			return path
		}
		path = filepath.Join(home, path[2:])
	}

	// Handle "." or relative paths - convert to absolute
	if path == "." || !filepath.IsAbs(path) {
		absPath, err := filepath.Abs(path)
		if err != nil {
			return path
		}
		path = absPath
	}

	return path
}

// DefaultConfigPath returns the default config file path
func DefaultConfigPath() string {
	home, err := os.UserHomeDir()
	if err != nil {
		return "./config.yml"
	}
	return filepath.Join(home, ".config", "git-scope", "config.yml")
}

// State holds user-toggled preferences that should persist across runs but
// don't belong in the human-edited YAML config (and would clobber its
// comments on rewrite).
type State struct {
	IncludeWorktrees bool `json:"include_worktrees"`
}

// DefaultStatePath returns the default location for the state file.
func DefaultStatePath() string {
	home, err := os.UserHomeDir()
	if err != nil {
		return "./state.json"
	}
	return filepath.Join(home, ".config", "git-scope", "state.json")
}

// LoadState reads the persisted state file. Returns a zero-value State and
// no error when the file is missing — first-run is not an error.
func LoadState(path string) (State, error) {
	var s State
	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return s, nil
		}
		return s, fmt.Errorf("read state: %w", err)
	}
	if err := json.Unmarshal(data, &s); err != nil {
		return s, fmt.Errorf("parse state: %w", err)
	}
	return s, nil
}

// SaveState writes the state file, creating the parent directory as needed.
func SaveState(path string, s State) error {
	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, 0755); err != nil {
		return fmt.Errorf("create state dir: %w", err)
	}
	data, err := json.MarshalIndent(s, "", "  ")
	if err != nil {
		return fmt.Errorf("marshal state: %w", err)
	}
	if err := os.WriteFile(path, data, 0644); err != nil {
		return fmt.Errorf("write state: %w", err)
	}
	return nil
}

var validKeyRE = regexp.MustCompile(`^(enter|alt\+[a-z])$`)

// ValidateActions ensures every action has a key/label and exactly one of
// Run or Clipboard, and that keys are unique and well-formed. Returns the
// first problem it finds.
func ValidateActions(actions []Action) error {
	seen := make(map[string]struct{}, len(actions))
	for i, a := range actions {
		if a.Key == "" {
			return fmt.Errorf("action %d: key is required", i+1)
		}
		if !validKeyRE.MatchString(a.Key) {
			return fmt.Errorf("action %q: key must be 'enter' or 'alt+<letter>'", a.Key)
		}
		if _, dup := seen[a.Key]; dup {
			return fmt.Errorf("action %q: duplicate key", a.Key)
		}
		seen[a.Key] = struct{}{}
		if a.Label == "" {
			return fmt.Errorf("action %q: label is required", a.Key)
		}
		if a.Run == "" && a.Clipboard == "" {
			return fmt.Errorf("action %q: one of 'run' or 'clipboard' is required", a.Key)
		}
		if a.Run != "" && a.Clipboard != "" {
			return fmt.Errorf("action %q: only one of 'run' or 'clipboard' may be set", a.Key)
		}
	}
	return nil
}

// WriteActions rewrites just the `actions:` key in config.yml, preserving the
// rest of the file's content and comments via the yaml.Node AST.
//
// If the file doesn't exist yet, a minimal one is created. If `actions:`
// already exists, its value node is replaced; otherwise it's appended to
// the root mapping.
func WriteActions(path string, actions []Action) error {
	if err := ValidateActions(actions); err != nil {
		return err
	}

	var root yaml.Node
	if data, err := os.ReadFile(path); err == nil {
		if err := yaml.Unmarshal(data, &root); err != nil {
			return fmt.Errorf("parse config for rewrite: %w", err)
		}
	} else if !os.IsNotExist(err) {
		return fmt.Errorf("read config: %w", err)
	}

	// yaml.Unmarshal returns a doc node wrapping a mapping. Construct that
	// scaffold if we're starting from scratch.
	if root.Kind == 0 {
		root.Kind = yaml.DocumentNode
		root.Content = []*yaml.Node{{Kind: yaml.MappingNode}}
	}
	if len(root.Content) == 0 || root.Content[0].Kind != yaml.MappingNode {
		return fmt.Errorf("config root is not a mapping")
	}
	mapping := root.Content[0]

	// Encode the new actions list once, take the encoded node.
	var encoded yaml.Node
	if err := encoded.Encode(actions); err != nil {
		return fmt.Errorf("encode actions: %w", err)
	}

	// Find existing "actions" key/value pair to replace; otherwise append.
	replaced := false
	for i := 0; i+1 < len(mapping.Content); i += 2 {
		if mapping.Content[i].Value == "actions" {
			mapping.Content[i+1] = &encoded
			replaced = true
			break
		}
	}
	if !replaced {
		keyNode := &yaml.Node{Kind: yaml.ScalarNode, Tag: "!!str", Value: "actions"}
		mapping.Content = append(mapping.Content, keyNode, &encoded)
	}

	out, err := yaml.Marshal(&root)
	if err != nil {
		return fmt.Errorf("marshal config: %w", err)
	}

	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, 0755); err != nil {
		return fmt.Errorf("create config dir: %w", err)
	}
	tmp, err := os.CreateTemp(dir, ".config.yml.*")
	if err != nil {
		return fmt.Errorf("temp file: %w", err)
	}
	tmpPath := tmp.Name()
	if _, err := tmp.Write(out); err != nil {
		tmp.Close()
		os.Remove(tmpPath)
		return fmt.Errorf("write temp config: %w", err)
	}
	if err := tmp.Close(); err != nil {
		os.Remove(tmpPath)
		return fmt.Errorf("close temp config: %w", err)
	}
	if err := os.Rename(tmpPath, path); err != nil {
		os.Remove(tmpPath)
		return fmt.Errorf("rename temp config: %w", err)
	}
	return nil
}

// ConfigExists checks if a config file exists at the given path
func ConfigExists(path string) bool {
	_, err := os.Stat(path)
	return err == nil
}

// CreateConfig creates a new config file at the given path
func CreateConfig(path string, roots []string, editor string) error {
	// Ensure directory exists
	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, 0755); err != nil {
		return fmt.Errorf("create config dir: %w", err)
	}

	cfg := &Config{
		Roots: roots,
		Ignore: []string{
			"node_modules",
			".next",
			"dist",
			"build",
			"target",
			".venv",
			"vendor",
		},
		Editor: editor,
	}

	data, err := yaml.Marshal(cfg)
	if err != nil {
		return fmt.Errorf("marshal config: %w", err)
	}

	// Add header comment
	content := "# git-scope configuration\n# Edit this file to customize scanning behavior\n\n" + string(data)

	if err := os.WriteFile(path, []byte(content), 0644); err != nil {
		return fmt.Errorf("write config: %w", err)
	}

	return nil
}
