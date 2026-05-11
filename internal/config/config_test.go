package config

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestWriteActions_PreservesComments(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "config.yml")

	original := `# git-scope configuration
# Edit this file to customize scanning behavior

roots:
    - ~/projects
ignore:
    - node_modules
editor: code # the editor binary
`
	if err := os.WriteFile(path, []byte(original), 0644); err != nil {
		t.Fatal(err)
	}

	actions := []Action{
		{Key: "enter", Label: "Open in editor", Run: "code {path}"},
		{Key: "alt+c", Label: "Copy path", Clipboard: "{path}"},
	}
	if err := WriteActions(path, actions); err != nil {
		t.Fatalf("WriteActions: %v", err)
	}

	got, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	content := string(got)

	// Header comments survive the rewrite.
	if !strings.Contains(content, "# git-scope configuration") {
		t.Errorf("header comment lost. got:\n%s", content)
	}
	if !strings.Contains(content, "# Edit this file to customize scanning behavior") {
		t.Errorf("second header comment lost. got:\n%s", content)
	}
	// Inline comment on editor: line. yaml.v3 preserves trailing comments on
	// scalar values when re-encoding via Node round-trip.
	if !strings.Contains(content, "# the editor binary") {
		t.Errorf("inline editor comment lost. got:\n%s", content)
	}

	// Actions block is present with both entries.
	if !strings.Contains(content, "actions:") {
		t.Errorf("actions block missing. got:\n%s", content)
	}
	if !strings.Contains(content, "alt+c") {
		t.Errorf("alt+c action missing. got:\n%s", content)
	}

	// File is loadable and roundtrips cleanly.
	cfg, err := Load(path)
	if err != nil {
		t.Fatalf("reload: %v", err)
	}
	if len(cfg.Actions) != 2 || cfg.Actions[1].Key != "alt+c" {
		t.Errorf("roundtrip mismatch: %+v", cfg.Actions)
	}
}

func TestWriteActions_AppendsWhenAbsent(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "config.yml")
	if err := os.WriteFile(path, []byte("roots:\n  - .\neditor: code\n"), 0644); err != nil {
		t.Fatal(err)
	}
	actions := []Action{{Key: "enter", Label: "x", Run: "code {path}"}}
	if err := WriteActions(path, actions); err != nil {
		t.Fatal(err)
	}
	cfg, err := Load(path)
	if err != nil {
		t.Fatal(err)
	}
	if len(cfg.Actions) != 1 {
		t.Errorf("expected 1 action, got %d", len(cfg.Actions))
	}
}

func TestSaveLoadState_WorkspacesRoundTrip(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "state.json")

	in := State{
		IncludeWorktrees: true,
		Workspaces: []Workspace{
			{Label: "Cloudbeds", Paths: []string{"~/projects/cloudbeds/tools"}},
			{Label: "OSS", Paths: []string{"~/projects/oss/devtools", "~/projects/oss/ai-agents"}},
		},
		ActiveWorkspace: "OSS",
	}
	if err := SaveState(path, in); err != nil {
		t.Fatal(err)
	}
	out, err := LoadState(path)
	if err != nil {
		t.Fatal(err)
	}
	if !out.IncludeWorktrees || out.ActiveWorkspace != "OSS" || len(out.Workspaces) != 2 {
		t.Fatalf("round-trip mismatch: %+v", out)
	}
	if len(out.Workspaces[1].Paths) != 2 {
		t.Errorf("expected 2 paths in OSS workspace, got %v", out.Workspaces[1].Paths)
	}
	if ws, ok := out.FindWorkspace("Cloudbeds"); !ok || ws.Paths[0] != "~/projects/cloudbeds/tools" {
		t.Errorf("FindWorkspace failed: ok=%v ws=%+v", ok, ws)
	}
}

func TestValidateWorkspace(t *testing.T) {
	existing := []Workspace{{Label: "A", Paths: []string{"/a"}}}
	cases := []struct {
		name    string
		ws      Workspace
		editIdx int
		wantErr bool
	}{
		{"valid new", Workspace{Label: "B", Paths: []string{"/b"}}, -1, false},
		{"valid edit-in-place keeps label", Workspace{Label: "A", Paths: []string{"/a2"}}, 0, false},
		{"empty label", Workspace{Label: "", Paths: []string{"/x"}}, -1, true},
		{"no paths", Workspace{Label: "X", Paths: nil}, -1, true},
		{"empty path", Workspace{Label: "X", Paths: []string{"  "}}, -1, true},
		{"duplicate label", Workspace{Label: "A", Paths: []string{"/x"}}, -1, true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			err := ValidateWorkspace(tc.ws, existing, tc.editIdx)
			if (err != nil) != tc.wantErr {
				t.Errorf("got %v, wantErr=%v", err, tc.wantErr)
			}
		})
	}
}

func TestCacheFreshnessDuration(t *testing.T) {
	cases := []struct {
		name, in string
		want     time.Duration
	}{
		{"empty falls back to default", "", DefaultCacheFreshness},
		{"explicit 30s", "30s", 30 * time.Second},
		{"explicit 5m", "5m", 5 * time.Minute},
		{"unparseable falls back", "potato", DefaultCacheFreshness},
		{"zero falls back", "0s", DefaultCacheFreshness},
		{"negative falls back", "-1m", DefaultCacheFreshness},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			c := &Config{CacheFreshness: tc.in}
			if got := c.CacheFreshnessDuration(); got != tc.want {
				t.Errorf("got %v, want %v", got, tc.want)
			}
		})
	}
}

func TestValidateActions(t *testing.T) {
	cases := []struct {
		name    string
		actions []Action
		wantErr bool
	}{
		{
			"valid",
			[]Action{{Key: "enter", Label: "x", Run: "code {path}"}, {Key: "alt+c", Label: "y", Clipboard: "{path}"}},
			false,
		},
		{
			"bad key shape",
			[]Action{{Key: "ctrl+c", Label: "x", Run: "y"}},
			true,
		},
		{
			"both run and clipboard",
			[]Action{{Key: "enter", Label: "x", Run: "y", Clipboard: "z"}},
			true,
		},
		{
			"neither run nor clipboard",
			[]Action{{Key: "enter", Label: "x"}},
			true,
		},
		{
			"duplicate keys",
			[]Action{{Key: "alt+c", Label: "x", Run: "y"}, {Key: "alt+c", Label: "z", Run: "w"}},
			true,
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			err := ValidateActions(tc.actions)
			if (err != nil) != tc.wantErr {
				t.Errorf("ValidateActions = %v, wantErr=%v", err, tc.wantErr)
			}
		})
	}
}
