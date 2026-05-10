package tui

import (
	"strings"
	"time"

	"github.com/Bharath-code/git-scope/internal/cache"
	"github.com/Bharath-code/git-scope/internal/clipboard"
	"github.com/Bharath-code/git-scope/internal/config"
	"github.com/Bharath-code/git-scope/internal/model"
	"github.com/Bharath-code/git-scope/internal/scan"
	tea "github.com/charmbracelet/bubbletea"
)

// Cache max age - use cached data if less than 5 minutes old
const cacheMaxAge = 5 * time.Minute

// Run starts the Bubbletea TUI application
func Run(cfg *config.Config) error {
	m := NewModel(cfg)
	p := tea.NewProgram(m, tea.WithAltScreen())
	_, err := p.Run()
	return err
}

// scanReposCmd is a command that scans for repositories.
// If forceRefresh is true, bypass cache and scan fresh.
// includeWorktrees is the live toggle state (overrides cfg for this scan).
func scanReposCmd(cfg *config.Config, forceRefresh, includeWorktrees bool) tea.Cmd {
	return func() tea.Msg {
		cacheStore := cache.NewFileStore()

		// Try to load from cache first (unless forcing refresh)
		if !forceRefresh {
			cached, err := cacheStore.Load()
			if err == nil &&
				cacheStore.IsValid(cacheMaxAge) &&
				cacheStore.IsSameRoots(cfg.Roots) &&
				cacheStore.IsSameIncludeWorktrees(includeWorktrees) {
				return scanCompleteMsg{
					repos:             cached.Repos,
					fromCache:         true,
					includedWorktrees: includeWorktrees,
				}
			}
		}

		// Scan fresh
		repos, err := scan.ScanRootsWithOptions(cfg.Roots, cfg.Ignore, includeWorktrees)
		if err != nil {
			return scanErrorMsg{err: err}
		}

		// Save to cache
		_ = cacheStore.Save(repos, cfg.Roots, includeWorktrees)

		return scanCompleteMsg{
			repos:             repos,
			fromCache:         false,
			includedWorktrees: includeWorktrees,
		}
	}
}

// scanCompleteMsg is sent when scanning is complete
type scanCompleteMsg struct {
	repos             []model.Repo
	fromCache         bool
	includedWorktrees bool
}

// scanErrorMsg is sent when scanning fails
type scanErrorMsg struct {
	err error
}

// runActionMsg requests execution of a `run`-type action (shell command).
type runActionMsg struct {
	action config.Action
	path   string
}

// actionFinishedMsg fires when a launched action's process exits.
type actionFinishedMsg struct {
	action config.Action
	err    error
}

// clipboardCopiedMsg reports the result of a `clipboard`-type action.
type clipboardCopiedMsg struct {
	text string
	err  error
}

// dispatchAction routes the action to the right side-effect command.
// `enter`/alt+* keys from the table and from inside the menu both funnel
// through here.
func dispatchAction(a config.Action, repoPath string) tea.Cmd {
	if a.IsClipboard() {
		text := strings.ReplaceAll(a.Clipboard, "{path}", repoPath)
		return func() tea.Msg {
			err := clipboard.Copy(text)
			return clipboardCopiedMsg{text: text, err: err}
		}
	}
	return func() tea.Msg { return runActionMsg{action: a, path: repoPath} }
}

// findAction returns the action with the given key, if any.
func findAction(actions []config.Action, key string) (config.Action, bool) {
	for _, a := range actions {
		if a.Key == key {
			return a, true
		}
	}
	return config.Action{}, false
}
