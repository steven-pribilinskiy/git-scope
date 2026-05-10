package tui

import (
	"fmt"
	"os/exec"
	"strings"

	"github.com/Bharath-code/git-scope/internal/browser"
	"github.com/Bharath-code/git-scope/internal/config"
	"github.com/Bharath-code/git-scope/internal/model"
	"github.com/Bharath-code/git-scope/internal/nudge"
	"github.com/Bharath-code/git-scope/internal/scan"
	"github.com/Bharath-code/git-scope/internal/stats"
	"github.com/Bharath-code/git-scope/internal/workspace"
	"github.com/charmbracelet/bubbles/spinner"
	"github.com/charmbracelet/bubbles/textinput"
	tea "github.com/charmbracelet/bubbletea"
	"mvdan.cc/sh/v3/shell"
)

// Update handles messages and updates the model
func (m Model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	var cmd tea.Cmd
	var cmds []tea.Cmd

	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width = msg.Width
		m.height = msg.Height
		m.resizeTable()

	case spinner.TickMsg:
		// Update spinner during loading
		if m.state == StateLoading {
			m.spinner, cmd = m.spinner.Update(msg)
			cmds = append(cmds, cmd)
		}

	case cacheLoadedMsg:
		// SWR fast path: show cached data immediately. The background
		// refresh fired alongside this will update the table when it lands.
		//
		// Race guard: if the refresh somehow finished first (we've already
		// flipped refreshing off and have data), don't overwrite the fresh
		// result with stale cache.
		if !m.refreshing && len(m.repos) > 0 {
			return m, nil
		}
		m.repos = msg.repos
		m.lastScanIncludesWorktrees = msg.includedWorktrees
		m.state = StateReady
		m.resetPage()
		m.updateTable()
		m.refreshing = true
		if len(msg.repos) == 0 {
			m.statusMsg = "↻ Scanning..."
		} else {
			m.statusMsg = fmt.Sprintf("✓ %d repos (cached) · ↻ refreshing…", len(msg.repos))
		}
		return m, nil

	case cacheMissMsg:
		// No usable cache. Stay in loading state until the in-flight
		// refresh completes.
		m.refreshing = true
		return m, nil

	case refreshCompleteMsg:
		// Discard if the in-flight refresh's worktree setting no longer
		// matches the user's current preference (raced with a W toggle).
		// The newer refresh will arrive shortly.
		if msg.includedWorktrees != m.includeWorktrees {
			return m, nil
		}
		m.repos = msg.repos
		m.lastScanIncludesWorktrees = msg.includedWorktrees
		m.refreshing = false
		m.state = StateReady
		m.resetPage()
		m.updateTable()
		if len(msg.repos) == 0 {
			m.statusMsg = "⚠️  No git repos found in configured directories. Press 'r' to rescan or run 'git-scope init' to configure."
		} else {
			m.statusMsg = fmt.Sprintf("✓ %d repos", len(msg.repos))
		}
		return m, nil

	case scanErrorMsg:
		// Only escalate to a hard error state when there's no data on
		// screen — if we already have stale cache visible, surface the
		// failure as a status line instead.
		if len(m.repos) > 0 {
			m.refreshing = false
			m.statusMsg = "❌ Refresh failed: " + msg.err.Error()
			return m, nil
		}
		m.state = StateError
		m.err = msg.err
		return m, nil

	case workspaceScanCompleteMsg:
		m.repos = msg.repos
		m.state = StateReady
		m.resetPage()
		m.updateTable()

		// Show helpful message about switched workspace
		if len(msg.repos) == 0 {
			m.statusMsg = fmt.Sprintf("⚠️  No git repos found in %s", msg.workspacePath)
		} else {
			m.statusMsg = fmt.Sprintf("✓ Switched to %s (%d repos)", msg.workspacePath, len(msg.repos))

			// Trigger star nudge after successful workspace switch
			if nudge.ShouldShowNudge() && !m.nudgeShownThisSession {
				m.showStarNudge = true
				m.nudgeShownThisSession = true
				nudge.MarkShown()
			}
		}
		return m, nil

	case workspaceScanErrorMsg:
		m.state = StateError
		m.err = msg.err
		return m, nil

	case runActionMsg:
		// Substitute {path} and parse the command into argv.
		cmdLine := strings.ReplaceAll(msg.action.Run, "{path}", msg.path)
		fields, err := shell.Fields(cmdLine, nil)
		if err != nil || len(fields) == 0 {
			m.statusMsg = fmt.Sprintf("❌ Invalid command for %q: %s", msg.action.Key, cmdLine)
			return m, nil
		}
		if _, err = exec.LookPath(fields[0]); err != nil {
			m.statusMsg = fmt.Sprintf("❌ '%s' not found on PATH", fields[0])
			return m, nil
		}
		c := exec.Command(fields[0], fields[1:]...)
		return m, tea.ExecProcess(c, func(err error) tea.Msg {
			return actionFinishedMsg{action: msg.action, err: err}
		})

	case actionFinishedMsg:
		if msg.err != nil {
			m.statusMsg = "Error: " + msg.err.Error()
		} else {
			m.statusMsg = ""
		}
		// Returning from an action (e.g. editor) — refresh quietly in the
		// background; the user is back from a context switch and doesn't
		// need a loading screen.
		m.refreshing = true
		return m, refreshScanCmd(m.cfg, m.includeWorktrees)

	case clipboardCopiedMsg:
		if msg.err != nil {
			m.statusMsg = "❌ Clipboard: " + msg.err.Error()
		} else {
			short := msg.text
			if len(short) > 60 {
				short = short[:57] + "…"
			}
			m.statusMsg = "📋 Copied: " + short
		}
		return m, nil

	case grassDataLoadedMsg:
		m.grassData = msg.data
		if msg.data != nil {
			m.statusMsg = fmt.Sprintf("🌿 %d commits in %d weeks", msg.data.TotalCommits, msg.data.WeeksCount)
		}
		return m, nil

	case diskDataLoadedMsg:
		m.diskData = msg.data
		if msg.data != nil {
			m.statusMsg = fmt.Sprintf("💾 %s total across %d repos", stats.FormatBytes(msg.data.TotalSize), msg.data.RepoCount)
		}
		return m, nil

	case timelineDataLoadedMsg:
		m.timelineData = msg.data
		if msg.data != nil {
			m.statusMsg = fmt.Sprintf("⏰ %d repos with recent activity", len(msg.data.Entries))
		}
		return m, nil

	case tea.KeyMsg:
		// Handle search mode separately
		if m.state == StateSearching {
			return m.handleSearchMode(msg)
		}

		// Handle workspace switch mode
		if m.state == StateWorkspaceSwitch {
			return m.handleWorkspaceSwitchMode(msg)
		}

		// Action menu / edit modes.
		if m.state == StateActionMenu {
			return m.handleActionMenuMode(msg)
		}
		if m.state == StateActionEdit {
			return m.handleActionEditMode(msg)
		}

		// Configured Alt+* shortcuts run their action directly from the table.
		if m.state == StateReady && strings.HasPrefix(msg.String(), "alt+") {
			if a, ok := findAction(m.cfg.Actions, msg.String()); ok {
				repo := m.GetSelectedRepo()
				if repo != nil {
					return m, dispatchAction(a, repo.Path)
				}
			}
		}

		// Normal mode key handling
		switch msg.String() {
		case "ctrl+c", "q":
			return m, tea.Quit

		case "S":
			// Open GitHub repo (Star nudge action)
			if m.showStarNudge {
				m.showStarNudge = false
				nudge.MarkCompleted()
				m.statusMsg = "⭐ Opening GitHub..."
				return m, openBrowserCmd(nudge.GitHubRepoURL)
			}

		case "/":
			// Enter search mode
			if m.state == StateReady {
				m.state = StateSearching
				m.resizeTable()
				m.textInput.Focus()
				m.textInput.SetValue(m.searchQuery)
				return m, textinput.Blink
			}

		case "enter":
			if m.state == StateReady {
				repo := m.GetSelectedRepo()
				if repo == nil {
					return m, nil
				}
				a, ok := findAction(m.cfg.Actions, "enter")
				if !ok {
					m.statusMsg = "No 'enter' action configured — press '?' to manage actions"
					return m, nil
				}
				m.statusMsg = a.Label + "..."
				return m, dispatchAction(a, repo.Path)
			}

		case "?":
			if m.state == StateReady {
				m.state = StateActionMenu
				if m.actionCursor >= len(m.cfg.Actions) {
					m.actionCursor = 0
				}
				m.confirmDelete = false
				return m, nil
			}

		case "r":
			// SWR: keep the current view on screen and refresh in
			// background. No StateLoading transition.
			m.refreshing = true
			m.statusMsg = "↻ Refreshing…"
			return m, refreshScanCmd(m.cfg, m.includeWorktrees)

		case "W":
			// Toggle linked-worktree inclusion. Single command — affects
			// both visibility and totals.
			//
			// Fast path: if the current scan already covers the desired
			// view (we have worktrees and just need to hide them, or we
			// don't need worktrees), this is an instant in-memory filter.
			// Slow path: only when going from "no worktrees scanned" to
			// "show worktrees" — we genuinely don't have the data yet.
			if m.state != StateReady {
				break
			}
			m.includeWorktrees = !m.includeWorktrees
			// Persist so the toggle survives restarts. Best-effort —
			// failures don't surface; the toggle still works in-session.
			_ = config.SaveState(config.DefaultStatePath(), config.State{
				IncludeWorktrees: m.includeWorktrees,
			})
			needRescan := m.includeWorktrees && !m.lastScanIncludesWorktrees
			if !needRescan {
				m.resetPage()
				m.updateTable()
				if m.includeWorktrees {
					m.statusMsg = "Worktrees: shown"
				} else {
					m.statusMsg = "Worktrees: hidden"
				}
				return m, nil
			}
			// Slow path: we don't have worktree data on disk yet.
			// Keep the current (worktree-less) view visible while
			// the background scan loads the larger set.
			m.refreshing = true
			m.statusMsg = "↻ Scanning worktrees…"
			return m, refreshScanCmd(m.cfg, m.includeWorktrees)

		case "f":
			// Cycle through filter modes
			if m.state == StateReady {
				m.filterMode = (m.filterMode + 1) % 3
				m.resetPage()
				m.updateTable()
				m.statusMsg = "Filter: " + m.GetFilterModeName()
				return m, nil
			}

		case "s":
			if m.state == StateReady {
				m.sortMode = (m.sortMode + 1) % 4
				m.resetPage()
				m.updateTable()
				m.statusMsg = "Sorted by: " + m.GetSortModeName()
				return m, nil
			}

		case "1":
			if m.state == StateReady {
				m.sortMode = SortByDirty
				m.resetPage()
				m.updateTable()
				m.statusMsg = "Sorted by: Dirty First"
				return m, nil
			}

		case "2":
			if m.state == StateReady {
				m.sortMode = SortByName
				m.resetPage()
				m.updateTable()
				m.statusMsg = "Sorted by: Name"
				return m, nil
			}

		case "3":
			if m.state == StateReady {
				m.sortMode = SortByBranch
				m.resetPage()
				m.updateTable()
				m.statusMsg = "Sorted by: Branch"
				return m, nil
			}

		case "4":
			if m.state == StateReady {
				m.sortMode = SortByLastCommit
				m.resetPage()
				m.updateTable()
				m.statusMsg = "Sorted by: Recent"
				return m, nil
			}

		case "c":
			// Clear search and filters
			if m.state == StateReady {
				m.searchQuery = ""
				m.textInput.SetValue("") // Also reset the text input
				m.filterMode = FilterAll
				m.resetPage()
				m.resizeTable()
				m.updateTable()
				m.statusMsg = "Filters cleared"
				return m, nil
			}

		case "e":
			if m.state == StateReady {
				// Check if editor exists (parse command to get binary name)
				fields, err := shell.Fields(m.cfg.Editor, nil)
				if err != nil || len(fields) == 0 {
					m.statusMsg = fmt.Sprintf("❌ Invalid editor command: '%s'", m.cfg.Editor)
				} else if _, err := exec.LookPath(fields[0]); err != nil {
					m.statusMsg = fmt.Sprintf("❌ Editor '%s' not found in PATH. Install it or edit ~/.config/git-scope/config.yml", fields[0])
				} else {
					m.statusMsg = fmt.Sprintf("✓ Editor: %s (edit config at ~/.config/git-scope/config.yml)", m.cfg.Editor)
				}
				return m, nil
			}

		case "g":
			// Toggle grass panel
			if m.state == StateReady {
				if m.activePanel == PanelGrass {
					m.activePanel = PanelNone
					m.statusMsg = ""
				} else {
					m.activePanel = PanelGrass
					m.statusMsg = "🌿 Loading contribution graph..."
					return m, loadGrassDataCmd(m.repos)
				}
				return m, nil
			}

		case "d":
			// Toggle disk usage panel
			if m.state == StateReady {
				if m.activePanel == PanelDisk {
					m.activePanel = PanelNone
					m.statusMsg = ""
				} else {
					m.activePanel = PanelDisk
					m.statusMsg = "💾 Calculating disk usage..."
					return m, loadDiskDataCmd(m.repos)
				}
				return m, nil
			}

		case "t":
			// Toggle timeline panel
			if m.state == StateReady {
				if m.activePanel == PanelTimeline {
					m.activePanel = PanelNone
					m.statusMsg = ""
				} else {
					m.activePanel = PanelTimeline
					m.statusMsg = "⏰ Loading timeline..."
					return m, loadTimelineDataCmd(m.repos)
				}
				return m, nil
			}

		case "esc":
			// Close panel if open
			if m.activePanel != PanelNone {
				m.activePanel = PanelNone
				m.statusMsg = ""
				return m, nil
			}

		case "w":
			// Open workspace switch modal
			if m.state == StateReady {
				m.state = StateWorkspaceSwitch
				m.workspaceInput.SetValue("")
				m.workspaceInput.Focus()
				m.workspaceError = ""
				return m, textinput.Blink
			}

		case "[":
			// Previous page
			if m.state == StateReady && m.canGoPrev() {
				m.currentPage--
				m.updateTable()
				m.statusMsg = fmt.Sprintf("Page %d of %d", m.currentPage+1, m.getTotalPages())
				return m, nil
			}

		case "]":
			// Next page
			if m.state == StateReady && m.canGoNext() {
				m.currentPage++
				m.updateTable()
				m.statusMsg = fmt.Sprintf("Page %d of %d", m.currentPage+1, m.getTotalPages())
				return m, nil
			}
		}
	}

	// Dismiss star nudge on any key (if not already handled)
	if m.showStarNudge {
		m.showStarNudge = false
		nudge.MarkDismissed()
	}

	// Update the table
	m.table, cmd = m.table.Update(msg)
	cmds = append(cmds, cmd)
	return m, tea.Batch(cmds...)
}

// handleSearchMode handles key events when in search mode
func (m Model) handleSearchMode(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch msg.String() {
	case "esc":
		// Cancel search, keep previous query
		m.state = StateReady
		m.resizeTable()
		m.textInput.Blur()
		return m, nil

	case "enter":
		// Apply search
		m.searchQuery = m.textInput.Value()
		m.state = StateReady
		m.resizeTable()
		m.textInput.Blur()
		m.resetPage()
		m.updateTable()
		if m.searchQuery != "" {
			m.statusMsg = "Searching: " + m.searchQuery
		} else {
			m.statusMsg = "Search cleared"
		}
		return m, nil

	case "ctrl+c":
		return m, tea.Quit
	}

	// Update text input
	var cmd tea.Cmd
	m.textInput, cmd = m.textInput.Update(msg)

	// Live search as you type
	m.searchQuery = m.textInput.Value()
	m.updateTable()

	return m, cmd
}

// grassDataLoadedMsg is sent when contribution data is loaded
type grassDataLoadedMsg struct {
	data *stats.ContributionData
}

// loadGrassDataCmd loads contribution data from all repos
func loadGrassDataCmd(repos []model.Repo) tea.Cmd {
	return func() tea.Msg {
		data, _ := stats.GetContributions(repos, 12) // Last 12 weeks
		return grassDataLoadedMsg{data: data}
	}
}

// diskDataLoadedMsg is sent when disk usage data is loaded
type diskDataLoadedMsg struct {
	data *stats.DiskUsageData
}

// loadDiskDataCmd loads disk usage data from all repos
func loadDiskDataCmd(repos []model.Repo) tea.Cmd {
	return func() tea.Msg {
		data, _ := stats.GetDiskUsage(repos)
		return diskDataLoadedMsg{data: data}
	}
}

// timelineDataLoadedMsg is sent when timeline data is loaded
type timelineDataLoadedMsg struct {
	data *stats.TimelineData
}

// loadTimelineDataCmd loads timeline data from all repos
func loadTimelineDataCmd(repos []model.Repo) tea.Cmd {
	return func() tea.Msg {
		data, _ := stats.GetTimeline(repos)
		return timelineDataLoadedMsg{data: data}
	}
}

// handleWorkspaceSwitchMode handles key events when in workspace switch mode
func (m Model) handleWorkspaceSwitchMode(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch msg.String() {
	case "esc":
		// Cancel workspace switch
		m.state = StateReady
		m.workspaceInput.Blur()
		m.workspaceError = ""
		return m, nil

	case "enter":
		// Validate and switch workspace
		inputPath := m.workspaceInput.Value()
		if inputPath == "" {
			m.workspaceError = "Please enter a path"
			return m, nil
		}

		// Normalize the path (expand ~, resolve symlinks, validate)
		normalizedPath, err := workspace.NormalizeWorkspacePath(inputPath)
		if err != nil {
			m.workspaceError = err.Error()
			return m, nil
		}

		// Switch to loading state and scan the new workspace
		m.state = StateLoading
		m.workspaceInput.Blur()
		m.workspaceError = ""
		m.activeWorkspace = normalizedPath
		m.statusMsg = "🔄 Switching to " + normalizedPath + "..."

		return m, scanWorkspaceCmd(normalizedPath, m.cfg.Ignore)

	case "tab":
		// Tab completion for directory paths
		currentPath := m.workspaceInput.Value()
		if currentPath != "" {
			completedPath := workspace.CompleteDirectoryPath(currentPath)
			if completedPath != currentPath {
				m.workspaceInput.SetValue(completedPath)
				// Move cursor to end
				m.workspaceInput.CursorEnd()
			}
		}
		return m, nil

	case "ctrl+c":
		return m, tea.Quit
	}

	// Update text input
	var cmd tea.Cmd
	m.workspaceInput, cmd = m.workspaceInput.Update(msg)

	// Clear error when typing
	if m.workspaceError != "" {
		m.workspaceError = ""
	}

	return m, cmd
}

// workspaceScanCompleteMsg is sent when workspace scanning is complete
type workspaceScanCompleteMsg struct {
	repos         []model.Repo
	workspacePath string
}

// workspaceScanErrorMsg is sent when workspace scanning fails
type workspaceScanErrorMsg struct {
	err error
}

// scanWorkspaceCmd scans a single workspace path for repositories
func scanWorkspaceCmd(workspacePath string, ignore []string) tea.Cmd {
	return func() tea.Msg {
		repos, err := scan.ScanRoots([]string{workspacePath}, ignore)
		if err != nil {
			return workspaceScanErrorMsg{err: err}
		}

		return workspaceScanCompleteMsg{
			repos:         repos,
			workspacePath: workspacePath,
		}
	}
}

// openBrowserCmd opens a URL in the default browser
func openBrowserCmd(url string) tea.Cmd {
	return func() tea.Msg {
		_ = browser.Open(url)
		return nil
	}
}

// handleActionMenuMode dispatches keys while the action menu is open.
func (m Model) handleActionMenuMode(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	key := msg.String()

	// First: a pending delete swallows the very next keystroke.
	if m.confirmDelete {
		m.confirmDelete = false
		if key == "y" || key == "Y" {
			return m.deleteSelectedAction()
		}
		// Any other key cancels the delete; fall through to normal dispatch
		// in case the user wanted to do something else.
	}

	switch key {
	case "esc", "?", "q":
		m.state = StateReady
		m.statusMsg = ""
		return m, nil
	case "up", "k":
		if m.actionCursor > 0 {
			m.actionCursor--
		}
		return m, nil
	case "down", "j":
		if m.actionCursor < len(m.cfg.Actions)-1 {
			m.actionCursor++
		}
		return m, nil
	case "enter":
		if len(m.cfg.Actions) == 0 {
			return m, nil
		}
		a := m.cfg.Actions[m.actionCursor]
		repo := m.GetSelectedRepo()
		if repo == nil {
			return m, nil
		}
		m.state = StateReady
		m.statusMsg = a.Label + "..."
		return m, dispatchAction(a, repo.Path)
	case "e":
		if len(m.cfg.Actions) > 0 {
			m.openActionEdit(m.actionCursor)
			return m, textinput.Blink
		}
		return m, nil
	case "a":
		m.openActionEdit(-1)
		return m, textinput.Blink
	case "d":
		if len(m.cfg.Actions) > 0 {
			m.confirmDelete = true
		}
		return m, nil
	case "ctrl+c":
		return m, tea.Quit
	}

	// Shortcut key match: run that action.
	if a, ok := findAction(m.cfg.Actions, key); ok {
		repo := m.GetSelectedRepo()
		if repo == nil {
			return m, nil
		}
		m.state = StateReady
		m.statusMsg = a.Label + "..."
		return m, dispatchAction(a, repo.Path)
	}
	return m, nil
}

// deleteSelectedAction removes the action under the cursor and persists the
// change. Returns a no-op command on success.
func (m Model) deleteSelectedAction() (Model, tea.Cmd) {
	if len(m.cfg.Actions) == 0 {
		return m, nil
	}
	idx := m.actionCursor
	deletedKey := m.cfg.Actions[idx].Key
	m.cfg.Actions = append(m.cfg.Actions[:idx], m.cfg.Actions[idx+1:]...)
	if m.actionCursor >= len(m.cfg.Actions) && m.actionCursor > 0 {
		m.actionCursor--
	}
	if err := config.WriteActions(config.DefaultConfigPath(), m.cfg.Actions); err != nil {
		m.statusMsg = "❌ Save failed: " + err.Error()
	} else {
		m.statusMsg = "Removed " + deletedKey
	}
	return m, nil
}

// openActionEdit prepares the edit modal for a given index (-1 = new).
func (m *Model) openActionEdit(idx int) {
	m.state = StateActionEdit
	m.actionEditIndex = idx
	m.actionEditField = 0
	m.actionEditErr = ""

	if idx >= 0 && idx < len(m.cfg.Actions) {
		m.actionEditBuf = m.cfg.Actions[idx]
	} else {
		m.actionEditBuf = config.Action{Key: "alt+", Label: "", Run: "{path}"}
	}

	m.actionKeyInput.SetValue(m.actionEditBuf.Key)
	m.actionLabelIn.SetValue(m.actionEditBuf.Label)
	if m.actionEditBuf.IsClipboard() {
		m.actionValueIn.SetValue(m.actionEditBuf.Clipboard)
	} else {
		m.actionValueIn.SetValue(m.actionEditBuf.Run)
	}
	m.focusEditField()
}

// focusEditField applies focus to the textinput matching actionEditField.
func (m *Model) focusEditField() {
	m.actionKeyInput.Blur()
	m.actionLabelIn.Blur()
	m.actionValueIn.Blur()
	switch m.actionEditField {
	case 0:
		m.actionKeyInput.Focus()
	case 1:
		m.actionLabelIn.Focus()
	case 3:
		m.actionValueIn.Focus()
	}
	// field 2 is the run/clipboard type toggle — no textinput focus.
}

// handleActionEditMode owns key events while the edit modal is open.
func (m Model) handleActionEditMode(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch msg.String() {
	case "esc":
		m.state = StateActionMenu
		m.actionEditErr = ""
		return m, nil
	case "ctrl+c":
		return m, tea.Quit
	case "tab":
		m.actionEditField = (m.actionEditField + 1) % 4
		m.focusEditField()
		return m, nil
	case "shift+tab":
		m.actionEditField = (m.actionEditField + 3) % 4
		m.focusEditField()
		return m, nil
	case " ":
		// Space toggles the run/clipboard type only when the type field
		// has focus. Otherwise it's a regular character.
		if m.actionEditField == 2 {
			m.actionEditBuf = toggleActionKind(m.actionEditBuf)
			m.actionValueIn.SetValue(m.actionEditBuf.Value())
			return m, nil
		}
	case "enter":
		return m.saveActionEdit()
	}

	// Forward typing to the focused textinput.
	var cmd tea.Cmd
	switch m.actionEditField {
	case 0:
		m.actionKeyInput, cmd = m.actionKeyInput.Update(msg)
		m.actionEditBuf.Key = strings.TrimSpace(m.actionKeyInput.Value())
	case 1:
		m.actionLabelIn, cmd = m.actionLabelIn.Update(msg)
		m.actionEditBuf.Label = m.actionLabelIn.Value()
	case 3:
		m.actionValueIn, cmd = m.actionValueIn.Update(msg)
		if m.actionEditBuf.IsClipboard() {
			m.actionEditBuf.Clipboard = m.actionValueIn.Value()
		} else {
			m.actionEditBuf.Run = m.actionValueIn.Value()
		}
	}
	if m.actionEditErr != "" {
		m.actionEditErr = ""
	}
	return m, cmd
}

// toggleActionKind flips an action between run and clipboard, preserving the
// current value string under the new field.
func toggleActionKind(a config.Action) config.Action {
	val := a.Value()
	if a.IsClipboard() {
		a.Clipboard = ""
		a.Run = val
	} else {
		a.Run = ""
		a.Clipboard = val
	}
	return a
}

// saveActionEdit validates the buffer, applies it to cfg.Actions, and
// persists to disk.
func (m Model) saveActionEdit() (Model, tea.Cmd) {
	buf := m.actionEditBuf
	buf.Key = strings.TrimSpace(buf.Key)
	buf.Label = strings.TrimSpace(buf.Label)

	// Build a candidate slice with the edit applied, then validate.
	candidate := make([]config.Action, len(m.cfg.Actions))
	copy(candidate, m.cfg.Actions)
	if m.actionEditIndex >= 0 && m.actionEditIndex < len(candidate) {
		candidate[m.actionEditIndex] = buf
	} else {
		candidate = append(candidate, buf)
	}
	if err := config.ValidateActions(candidate); err != nil {
		m.actionEditErr = err.Error()
		return m, nil
	}

	m.cfg.Actions = candidate
	if err := config.WriteActions(config.DefaultConfigPath(), m.cfg.Actions); err != nil {
		m.actionEditErr = "Save failed: " + err.Error()
		return m, nil
	}

	m.state = StateActionMenu
	m.actionEditErr = ""
	if m.actionEditIndex < 0 {
		m.actionCursor = len(m.cfg.Actions) - 1
		m.statusMsg = "Added " + buf.Key
	} else {
		m.statusMsg = "Saved " + buf.Key
	}
	return m, nil
}
