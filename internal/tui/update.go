package tui

import (
	"fmt"
	"os/exec"
	"strings"

	"github.com/Bharath-code/git-scope/internal/browser"
	"github.com/Bharath-code/git-scope/internal/config"
	"github.com/Bharath-code/git-scope/internal/model"
	"github.com/Bharath-code/git-scope/internal/nudge"
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
		// resizeTable internally calls applyViewSettings, which under
		// Adaptive layout flips standard↔compact in lockstep with the
		// new row/column shape.
		m.resizeTable()
		// Panel renders depend on width/height — rebuild any that have
		// data so the next View paints at the new size without going
		// through the lazy fallback path.
		if m.grassData != nil {
			m.grassRendered = renderGrassPanel(m.grassData, m.width/2, m.height-15)
		}
		if m.diskData != nil {
			m.diskRendered = renderDiskPanel(m.diskData, m.width/2, m.height-15)
		}
		if m.timelineData != nil {
			m.timelineRendered = renderTimelinePanel(m.timelineData, m.width/2, m.height-15)
		}

	case spinner.TickMsg:
		// Update spinner during loading
		if m.state == StateLoading {
			m.spinner, cmd = m.spinner.Update(msg)
			cmds = append(cmds, cmd)
		}

	case refreshCompleteMsg:
		// Worktree-setting race: a W toggle since this refresh was kicked
		// off invalidates the result. Drop it and clear the indicator —
		// the new toggle has already queued its own refresh.
		if msg.includedWorktrees != m.includeWorktrees {
			m.refreshing = false
			return m, nil
		}
		// Silent path: if the fresh data is byte-identical to what's on
		// screen, drop the badge and stop. No table rebuild, no status
		// churn — the user sees nothing flicker.
		if reposEqual(m.repos, msg.repos) {
			m.refreshing = false
			m.lastScanIncludesWorktrees = msg.includedWorktrees
			return m, nil
		}
		m.repos = msg.repos
		m.lastScanIncludesWorktrees = msg.includedWorktrees
		m.refreshing = false
		m.state = StateReady
		// Don't resetPage — the user may have been browsing. Clamp the
		// cursor to a valid page if the repo count shrank.
		if total := m.getTotalPages(); m.currentPage >= total {
			m.currentPage = total - 1
		}
		if m.currentPage < 0 {
			m.currentPage = 0
		}
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
		m.grassRendered = renderGrassPanel(msg.data, m.width/2, m.height-15)
		if msg.data != nil {
			m.statusMsg = fmt.Sprintf("🌿 %d commits in %d weeks", msg.data.TotalCommits, msg.data.WeeksCount)
		}
		return m, nil

	case diskDataLoadedMsg:
		m.diskData = msg.data
		m.diskRendered = renderDiskPanel(msg.data, m.width/2, m.height-15)
		if msg.data != nil {
			m.statusMsg = fmt.Sprintf("💾 %s total across %d repos", stats.FormatBytes(msg.data.TotalSize), msg.data.RepoCount)
		}
		return m, nil

	case timelineDataLoadedMsg:
		m.timelineData = msg.data
		m.timelineRendered = renderTimelinePanel(msg.data, m.width/2, m.height-15)
		if msg.data != nil {
			m.statusMsg = fmt.Sprintf("⏰ %d repos with recent activity", len(msg.data.Entries))
		}
		return m, nil

	case tea.KeyMsg:
		// Handle search mode separately
		if m.state == StateSearching {
			return m.handleSearchMode(msg)
		}

		// Handle workspace picker + add/edit form
		if m.state == StateWorkspaceSwitch {
			return m.handleWorkspaceSwitchMode(msg)
		}
		if m.state == StateWorkspaceEdit {
			return m.handleWorkspaceEditMode(msg)
		}

		// View Settings modal
		if m.state == StateViewSettings {
			return m.handleViewSettingsMode(msg)
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

		case "v":
			// Open the View Settings modal — layout, last-commit format,
			// per-column toggles. Selections persist to state.json.
			if m.state == StateReady {
				m.state = StateViewSettings
				m.viewSettingsCursor = 0
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
					m.applyViewSettings()
					return m, loadGrassDataCmd(m.repos)
				}
				m.applyViewSettings()
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
					m.applyViewSettings()
					return m, loadDiskDataCmd(m.repos)
				}
				m.applyViewSettings()
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
					m.applyViewSettings()
					return m, loadTimelineDataCmd(m.repos)
				}
				m.applyViewSettings()
				return m, nil
			}

		case "esc":
			// Close panel if open
			if m.activePanel != PanelNone {
				m.activePanel = PanelNone
				m.statusMsg = ""
				m.applyViewSettings()
				return m, nil
			}

		case "w":
			// Open the saved-workspaces picker. The list shows an
			// implicit "Default (config.yml roots)" row at index 0 plus
			// every saved workspace; the active one (if any) starts under
			// the cursor.
			if m.state == StateReady {
				m.state = StateWorkspaceSwitch
				m.workspaceConfirmDelete = false
				m.workspaceCursor = m.activeWorkspaceCursor()
				return m, nil
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

// workspaceRowCount returns the number of rows the picker should display:
// the implicit "Default" row plus one row per saved workspace.
func (m Model) workspaceRowCount() int { return 1 + len(m.workspaces) }

// activeWorkspaceCursor returns the row index of the currently active
// workspace, or 0 (Default) when none is active.
func (m Model) activeWorkspaceCursor() int {
	if m.activeWorkspace == "" {
		return 0
	}
	for i, w := range m.workspaces {
		if w.Label == m.activeWorkspace {
			return i + 1
		}
	}
	return 0
}

// handleWorkspaceSwitchMode dispatches keys while the workspace picker is
// open. The row at index 0 is the implicit "Default (config.yml roots)";
// rows 1..N are the saved workspaces.
func (m Model) handleWorkspaceSwitchMode(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	key := msg.String()

	if m.workspaceConfirmDelete {
		m.workspaceConfirmDelete = false
		if key == "y" || key == "Y" {
			return m.deleteSelectedWorkspace()
		}
		// Any other key cancels the confirm; fall through.
	}

	switch key {
	case "esc", "q":
		m.state = StateReady
		m.statusMsg = ""
		return m, nil
	case "up", "k":
		if m.workspaceCursor > 0 {
			m.workspaceCursor--
		}
		return m, nil
	case "down", "j":
		if m.workspaceCursor < m.workspaceRowCount()-1 {
			m.workspaceCursor++
		}
		return m, nil
	case "enter":
		return m.switchToSelectedWorkspace()
	case "a":
		m.openWorkspaceEdit(-1)
		return m, textinput.Blink
	case "e":
		// Default row isn't editable.
		if m.workspaceCursor > 0 {
			m.openWorkspaceEdit(m.workspaceCursor - 1)
			return m, textinput.Blink
		}
		return m, nil
	case "d":
		if m.workspaceCursor > 0 {
			m.workspaceConfirmDelete = true
		}
		return m, nil
	case "ctrl+c":
		return m, tea.Quit
	}
	return m, nil
}

// switchToSelectedWorkspace applies the cursor row to cfg.Roots, persists
// the choice as active, and kicks off a refresh. Default row clears the
// active workspace and restores cfg.Roots from the YAML config.
func (m Model) switchToSelectedWorkspace() (Model, tea.Cmd) {
	var newRoots []string
	var newActive string

	if m.workspaceCursor == 0 {
		if len(m.defaultRoots) == 0 {
			m.statusMsg = "❌ Default roots not available"
			return m, nil
		}
		newRoots = append([]string(nil), m.defaultRoots...)
		newActive = ""
	} else {
		ws := m.workspaces[m.workspaceCursor-1]
		paths := make([]string, 0, len(ws.Paths))
		for _, p := range ws.Paths {
			norm, err := workspace.NormalizeWorkspacePath(p)
			if err != nil {
				m.statusMsg = fmt.Sprintf("❌ %s: %s", p, err.Error())
				return m, nil
			}
			paths = append(paths, norm)
		}
		newRoots = paths
		newActive = ws.Label
	}

	m.cfg.Roots = newRoots
	m.activeWorkspace = newActive
	m.repos = nil
	m.lastScanIncludesWorktrees = false
	m.refreshing = true
	m.state = StateLoading
	if newActive == "" {
		m.statusMsg = "↻ Switching to Default…"
	} else {
		m.statusMsg = "↻ Switching to " + newActive + "…"
	}

	// Persist active workspace so the next launch boots into the same view.
	persistActiveWorkspace(newActive)

	return m, refreshScanCmd(m.cfg, m.includeWorktrees)
}

// deleteSelectedWorkspace removes the workspace under the cursor, rewrites
// state.json, and adjusts state/cursor accordingly. If the deleted entry
// was active, we fall back to Default but DON'T auto-switch the dashboard.
func (m Model) deleteSelectedWorkspace() (Model, tea.Cmd) {
	idx := m.workspaceCursor - 1
	if idx < 0 || idx >= len(m.workspaces) {
		return m, nil
	}
	deleted := m.workspaces[idx]
	m.workspaces = append(m.workspaces[:idx], m.workspaces[idx+1:]...)
	if m.activeWorkspace == deleted.Label {
		m.activeWorkspace = ""
	}
	if m.workspaceCursor >= m.workspaceRowCount() {
		m.workspaceCursor = m.workspaceRowCount() - 1
	}
	if err := persistWorkspaces(m.workspaces, m.activeWorkspace, m.includeWorktrees); err != nil {
		m.statusMsg = "❌ Save failed: " + err.Error()
	} else {
		m.statusMsg = "Removed workspace " + deleted.Label
	}
	return m, nil
}

// openWorkspaceEdit puts the modal in edit mode for the given index. Pass
// -1 to add a new workspace.
func (m *Model) openWorkspaceEdit(idx int) {
	m.state = StateWorkspaceEdit
	m.workspaceEditIndex = idx
	m.workspaceEditField = 0
	m.workspaceEditErr = ""
	if idx >= 0 && idx < len(m.workspaces) {
		m.workspaceLabelInput.SetValue(m.workspaces[idx].Label)
		m.workspacePathsInput.SetValue(strings.Join(m.workspaces[idx].Paths, ", "))
	} else {
		m.workspaceLabelInput.SetValue("")
		m.workspacePathsInput.SetValue("")
	}
	m.workspaceLabelInput.Focus()
	m.workspacePathsInput.Blur()
}

// handleWorkspaceEditMode owns key events while the add/edit form is open.
func (m Model) handleWorkspaceEditMode(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch msg.String() {
	case "esc":
		m.state = StateWorkspaceSwitch
		m.workspaceEditErr = ""
		return m, nil
	case "ctrl+c":
		return m, tea.Quit
	case "tab":
		m.workspaceEditField = (m.workspaceEditField + 1) % 2
		m.focusWorkspaceEditField()
		return m, nil
	case "shift+tab":
		m.workspaceEditField = (m.workspaceEditField + 1) % 2
		m.focusWorkspaceEditField()
		return m, nil
	case "enter":
		return m.saveWorkspaceEdit()
	}

	var cmd tea.Cmd
	switch m.workspaceEditField {
	case 0:
		m.workspaceLabelInput, cmd = m.workspaceLabelInput.Update(msg)
	case 1:
		m.workspacePathsInput, cmd = m.workspacePathsInput.Update(msg)
	}
	if m.workspaceEditErr != "" {
		m.workspaceEditErr = ""
	}
	return m, cmd
}

func (m *Model) focusWorkspaceEditField() {
	m.workspaceLabelInput.Blur()
	m.workspacePathsInput.Blur()
	if m.workspaceEditField == 0 {
		m.workspaceLabelInput.Focus()
	} else {
		m.workspacePathsInput.Focus()
	}
}

// saveWorkspaceEdit validates the form buffer, writes it to cfg.Workspaces
// (in-memory) and to state.json, then drops back to the picker.
func (m Model) saveWorkspaceEdit() (Model, tea.Cmd) {
	label := strings.TrimSpace(m.workspaceLabelInput.Value())
	raw := m.workspacePathsInput.Value()
	parts := strings.Split(raw, ",")
	paths := make([]string, 0, len(parts))
	for _, p := range parts {
		p = strings.TrimSpace(p)
		if p == "" {
			continue
		}
		paths = append(paths, p)
	}
	candidate := config.Workspace{Label: label, Paths: paths}
	if err := config.ValidateWorkspace(candidate, m.workspaces, m.workspaceEditIndex); err != nil {
		m.workspaceEditErr = err.Error()
		return m, nil
	}
	// Validate paths exist — this catches typos before they cost a cold scan.
	for _, p := range paths {
		if _, err := workspace.NormalizeWorkspacePath(p); err != nil {
			m.workspaceEditErr = fmt.Sprintf("%s: %s", p, err.Error())
			return m, nil
		}
	}

	if m.workspaceEditIndex >= 0 && m.workspaceEditIndex < len(m.workspaces) {
		// Editing existing — if the label changed and this entry was
		// active, update the activeWorkspace pointer too.
		oldLabel := m.workspaces[m.workspaceEditIndex].Label
		m.workspaces[m.workspaceEditIndex] = candidate
		if m.activeWorkspace == oldLabel {
			m.activeWorkspace = candidate.Label
		}
	} else {
		m.workspaces = append(m.workspaces, candidate)
		m.workspaceCursor = len(m.workspaces) // 1-indexed (Default at 0)
	}

	if err := persistWorkspaces(m.workspaces, m.activeWorkspace, m.includeWorktrees); err != nil {
		m.workspaceEditErr = "Save failed: " + err.Error()
		return m, nil
	}

	m.state = StateWorkspaceSwitch
	m.statusMsg = "Saved workspace " + candidate.Label
	return m, nil
}

// persistWorkspaces rewrites state.json with the current workspace list,
// preserving the worktree toggle.
func persistWorkspaces(workspaces []config.Workspace, active string, includeWorktrees bool) error {
	return config.SaveState(config.DefaultStatePath(), config.State{
		IncludeWorktrees: includeWorktrees,
		Workspaces:       workspaces,
		ActiveWorkspace:  active,
	})
}

// persistActiveWorkspace updates only the active-workspace pointer in
// state.json. Used when switching from the picker without editing the list.
func persistActiveWorkspace(active string) {
	st, err := config.LoadState(config.DefaultStatePath())
	if err != nil {
		st = config.State{}
	}
	st.ActiveWorkspace = active
	_ = config.SaveState(config.DefaultStatePath(), st)
}

// viewSettingsRows is the ordered list of rows the View Settings modal
// renders. Used by both the renderer and the handler so the cursor index
// agrees with the on-screen position. Column rows are filtered to the
// currently-effective layout so toggling Standard ↔ Compact swaps the
// per-column toggles in real time.
type viewSettingsRow struct {
	kind viewSettingsRowKind
	col  colDef // populated when kind == viewRowColumn
}

type viewSettingsRowKind int

const (
	viewRowSort viewSettingsRowKind = iota
	viewRowFilter
	viewRowLayout
	viewRowLastCommit
	viewRowColumnsHeader
	viewRowColumn
)

func (m Model) viewSettingsRows() []viewSettingsRow {
	rows := []viewSettingsRow{
		{kind: viewRowSort},
		{kind: viewRowFilter},
		{kind: viewRowLayout},
		{kind: viewRowLastCommit},
		{kind: viewRowColumnsHeader},
	}
	// Use the effective layout's column set so the visible toggles match
	// what the user is seeing on screen. Non-toggleable columns (repo,
	// branch) appear as informational rows.
	var base []colDef
	if m.effectiveLayout() == config.LayoutCompact {
		base = compactLayoutCols()
	} else {
		base = standardLayoutCols()
	}
	for _, c := range base {
		rows = append(rows, viewSettingsRow{kind: viewRowColumn, col: c})
	}
	return rows
}

// handleViewSettingsMode dispatches keys while the View Settings modal
// is open. Every change persists to state.json and immediately rebuilds
// the table so feedback is live.
func (m Model) handleViewSettingsMode(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	rows := m.viewSettingsRows()
	key := msg.String()

	switch key {
	case "esc", "q":
		m.state = StateReady
		return m, nil
	case "ctrl+c":
		return m, tea.Quit
	case "up", "k":
		// Skip rows that aren't actionable (the "Columns" header).
		for m.viewSettingsCursor > 0 {
			m.viewSettingsCursor--
			if rows[m.viewSettingsCursor].kind != viewRowColumnsHeader {
				break
			}
		}
		return m, nil
	case "down", "j":
		for m.viewSettingsCursor < len(rows)-1 {
			m.viewSettingsCursor++
			if rows[m.viewSettingsCursor].kind != viewRowColumnsHeader {
				break
			}
		}
		return m, nil
	}

	if m.viewSettingsCursor >= len(rows) {
		return m, nil
	}
	row := rows[m.viewSettingsCursor]

	// Space / Enter activates the focused row. The cycle-rows advance to
	// the next value; column-rows flip the visible/hidden bit.
	if key == " " || key == "enter" || key == "right" || key == "l" {
		return m.applyViewSettingsCycle(row, +1)
	}
	if key == "left" || key == "h" {
		return m.applyViewSettingsCycle(row, -1)
	}

	return m, nil
}

// applyViewSettingsCycle advances (delta=+1) or reverses (delta=-1) the
// focused row's value, then persists and rebuilds.
func (m Model) applyViewSettingsCycle(row viewSettingsRow, delta int) (Model, tea.Cmd) {
	switch row.kind {
	case viewRowSort:
		m.sortMode = SortMode((int(m.sortMode) + delta + 4) % 4)
		m.resetPage()
	case viewRowFilter:
		m.filterMode = FilterMode((int(m.filterMode) + delta + 3) % 3)
		m.resetPage()
	case viewRowLayout:
		layouts := []string{config.LayoutStandard, config.LayoutCompact, config.LayoutAdaptive}
		current := indexOf(layouts, m.viewLayout)
		m.viewLayout = layouts[(current+delta+len(layouts))%len(layouts)]
	case viewRowLastCommit:
		formats := []string{config.LastCommitDate, config.LastCommitDateYear, config.LastCommitAgo}
		current := indexOf(formats, m.lastCommitFormat)
		m.lastCommitFormat = formats[(current+delta+len(formats))%len(formats)]
	case viewRowColumn:
		if !row.col.Toggleable {
			return m, nil
		}
		key := string(row.col.Key)
		if containsString(m.hiddenColumns, key) {
			m.hiddenColumns = removeString(m.hiddenColumns, key)
		} else {
			m.hiddenColumns = append(m.hiddenColumns, key)
		}
	default:
		return m, nil
	}
	m.applyViewSettings()
	m.persistViewSettings()
	return m, nil
}

// persistViewSettings writes the current view block back to state.json,
// preserving the other state fields (workspaces, worktree toggle, etc.).
func (m Model) persistViewSettings() {
	st, err := config.LoadState(config.DefaultStatePath())
	if err != nil {
		st = config.State{}
	}
	st.View = config.ViewSettings{
		Layout:           m.viewLayout,
		LastCommitFormat: m.lastCommitFormat,
		HiddenColumns:    append([]string(nil), m.hiddenColumns...),
	}
	_ = config.SaveState(config.DefaultStatePath(), st)
}

func indexOf(slice []string, v string) int {
	for i, s := range slice {
		if s == v {
			return i
		}
	}
	return 0
}

func containsString(slice []string, v string) bool {
	for _, s := range slice {
		if s == v {
			return true
		}
	}
	return false
}

func removeString(slice []string, v string) []string {
	out := slice[:0]
	for _, s := range slice {
		if s != v {
			out = append(out, s)
		}
	}
	return out
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
