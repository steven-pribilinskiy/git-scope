package tui

import (
	"strings"

	"github.com/Bharath-code/git-scope/internal/cache"
	"github.com/Bharath-code/git-scope/internal/clipboard"
	"github.com/Bharath-code/git-scope/internal/config"
	"github.com/Bharath-code/git-scope/internal/model"
	"github.com/Bharath-code/git-scope/internal/scan"
	tea "github.com/charmbracelet/bubbletea"
)

// Run starts the Bubbletea TUI application
func Run(cfg *config.Config) error {
	m := NewModel(cfg)
	p := tea.NewProgram(m, tea.WithAltScreen())
	_, err := p.Run()
	return err
}

// Cache strategy: stale-while-revalidate.
//
// On every load entry point (initial launch, manual `r`, post-action, slow
// worktree toggle) we do two things in parallel: (1) hand the cached repos to
// the UI immediately so the user can interact without waiting, and (2) kick
// off a fresh scan in the background that overwrites the cache and replaces
// the table when it lands.
//
// There's no time-based cache expiry — the cache is always shown if its
// structure matches the current request (same roots; worktree setting can
// differ, the refresh fixes it). Refresh always runs.

// loadCacheCmd reads the on-disk cache and emits it. Emits cacheMissMsg if
// the cache is absent or structurally incompatible with the current request.
func loadCacheCmd(cfg *config.Config, includeWorktrees bool) tea.Cmd {
	return func() tea.Msg {
		store := cache.NewFileStore()
		cached, err := store.Load()
		if err != nil {
			return cacheMissMsg{}
		}
		// Roots must match — different roots mean a different repo universe.
		// IncludeWorktrees mismatch is tolerated: we'd rather show stale
		// data than a blank screen; the in-flight refresh corrects it.
		if !store.IsSameRoots(cfg.Roots) {
			return cacheMissMsg{}
		}
		return cacheLoadedMsg{
			repos:             cached.Repos,
			includedWorktrees: cached.IncludeWorktrees,
		}
	}
}

// refreshScanCmd performs a fresh scan and writes the cache.
func refreshScanCmd(cfg *config.Config, includeWorktrees bool) tea.Cmd {
	return func() tea.Msg {
		repos, err := scan.ScanRootsWithOptions(cfg.Roots, cfg.Ignore, includeWorktrees)
		if err != nil {
			return scanErrorMsg{err: err}
		}
		store := cache.NewFileStore()
		_ = store.Save(repos, cfg.Roots, includeWorktrees)
		return refreshCompleteMsg{
			repos:             repos,
			includedWorktrees: includeWorktrees,
		}
	}
}

// loadAndRefreshCmd batches a cache load with a background refresh — the
// canonical SWR sequence used at most load entry points.
func loadAndRefreshCmd(cfg *config.Config, includeWorktrees bool) tea.Cmd {
	return tea.Batch(
		loadCacheCmd(cfg, includeWorktrees),
		refreshScanCmd(cfg, includeWorktrees),
	)
}

// cacheLoadedMsg carries cached repos delivered to the UI on first paint.
type cacheLoadedMsg struct {
	repos             []model.Repo
	includedWorktrees bool
}

// cacheMissMsg is emitted when there's no usable cache (cold start or roots
// changed). The UI shows the loading screen until the refresh lands.
type cacheMissMsg struct{}

// refreshCompleteMsg carries the result of a background scan. The handler
// must verify includedWorktrees against the current model — a refresh in
// flight when the user toggled W can race; we only accept matching results.
type refreshCompleteMsg struct {
	repos             []model.Repo
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
