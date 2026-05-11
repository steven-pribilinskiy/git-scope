package tui

import (
	"fmt"
	"strings"
	"time"

	"github.com/Bharath-code/git-scope/internal/config"
	"github.com/Bharath-code/git-scope/internal/model"
	"github.com/Bharath-code/git-scope/internal/stats"
	"github.com/charmbracelet/bubbles/table"
)

// Adaptive layout threshold. Below this terminal width (or whenever a side
// panel is open) the dashboard collapses to the compact column set.
//
// 110 ≈ standard layout total width (status 8 + repo 18 + branch 14 +
// staged 6 + modified 8 + untracked 9 + ahead 7 + behind 7 + last_commit
// 14 + padding/dividers) — anything tighter and columns clip.
const adaptiveThreshold = 110

// colKey is a stable identifier for a column. Persisted to state.json
// under HiddenColumns, so renaming one is a breaking change.
type colKey string

const (
	colStatus     colKey = "status"
	colRepo       colKey = "repo"
	colBranch     colKey = "branch"
	colStaged     colKey = "staged"
	colModified   colKey = "modified"
	colUntracked  colKey = "untracked"
	colAhead      colKey = "ahead"
	colBehind     colKey = "behind"
	colLastCommit colKey = "last_commit"
	// Compact-mode grouped columns.
	colChanges colKey = "changes" // staged/modified/untracked
	colSync    colKey = "sync"    // ahead/behind
)

// colDef describes one column: width, header, how to render a row's cell,
// and whether the user can hide it via the settings modal.
type colDef struct {
	Key        colKey
	Title      string
	Width      int
	Toggleable bool
	Render     func(r model.Repo, viewport viewportInfo) string
}

// viewportInfo carries the bits of model state a cell renderer needs.
// Avoids the renderer taking a full Model pointer (which complicates the
// receiver/value-vs-pointer split in bubbletea).
type viewportInfo struct {
	now              time.Time
	lastCommitFormat string
}

// standardLayoutCols is the full 9-column layout we've shipped to date.
// Ordering matches the dashboard from left to right.
func standardLayoutCols() []colDef {
	return []colDef{
		{Key: colStatus, Title: "Status", Width: 8, Toggleable: true, Render: renderStatusCell},
		{Key: colRepo, Title: "Repository", Width: 18, Toggleable: false, Render: renderRepoCell},
		{Key: colBranch, Title: "Branch", Width: 14, Toggleable: false, Render: renderBranchCell},
		{Key: colStaged, Title: "Staged", Width: 6, Toggleable: true, Render: numCellRenderer(func(r model.Repo) int { return r.Status.Staged })},
		{Key: colModified, Title: "Modified", Width: 8, Toggleable: true, Render: numCellRenderer(func(r model.Repo) int { return r.Status.Unstaged })},
		{Key: colUntracked, Title: "Untracked", Width: 9, Toggleable: true, Render: numCellRenderer(func(r model.Repo) int { return r.Status.Untracked })},
		{Key: colAhead, Title: "Ahead", Width: 7, Toggleable: true, Render: numCellRenderer(func(r model.Repo) int { return r.Status.Ahead })},
		{Key: colBehind, Title: "Behind", Width: 7, Toggleable: true, Render: numCellRenderer(func(r model.Repo) int { return r.Status.Behind })},
		{Key: colLastCommit, Title: "Last Commit", Width: lastCommitColumnWidth(""), Toggleable: true, Render: renderLastCommitCell},
	}
}

// compactLayoutCols is the trimmed layout: change-counts and sync state
// each collapse into a single column, freeing space for wider repo and
// branch names.
func compactLayoutCols() []colDef {
	return []colDef{
		{Key: colStatus, Title: "Status", Width: 8, Toggleable: true, Render: renderStatusCell},
		{Key: colRepo, Title: "Repository", Width: 28, Toggleable: false, Render: renderRepoCell},
		{Key: colBranch, Title: "Branch", Width: 22, Toggleable: false, Render: renderBranchCell},
		{Key: colChanges, Title: "Changes", Width: 11, Toggleable: true, Render: renderChangesCell},
		{Key: colSync, Title: "Sync", Width: 9, Toggleable: true, Render: renderSyncCell},
		{Key: colLastCommit, Title: "Last Commit", Width: lastCommitColumnWidth(""), Toggleable: true, Render: renderLastCommitCell},
	}
}

// lastCommitColumnWidth picks a column width that comfortably fits the
// chosen format string. Called when columns are (re)built.
func lastCommitColumnWidth(format string) int {
	switch format {
	case config.LastCommitDateYear:
		return 18
	case config.LastCommitAgo:
		return 13
	default: // LastCommitDate
		return 14
	}
}

// effectiveLayout resolves the user's chosen layout into the concrete one
// to render right now. "Adaptive" → standard or compact based on width
// and whether a side panel is open.
func (m Model) effectiveLayout() string {
	layout := config.ViewSettings{Layout: m.viewLayout}.NormalizedLayout()
	if layout != config.LayoutAdaptive {
		return layout
	}
	panelOpen := m.activePanel != PanelNone
	usable := m.width
	if panelOpen {
		// renderSplitPane gives the table ~58% of width.
		usable = int(float64(m.width) * 0.58)
	}
	if panelOpen || usable < adaptiveThreshold {
		return config.LayoutCompact
	}
	return config.LayoutStandard
}

// activeColumns returns the columns to display this frame, after applying
// the hidden-columns filter to the effective layout's column list.
func (m Model) activeColumns() []colDef {
	var base []colDef
	if m.effectiveLayout() == config.LayoutCompact {
		base = compactLayoutCols()
	} else {
		base = standardLayoutCols()
	}
	// Adapt last-commit column width to the chosen format.
	for i := range base {
		if base[i].Key == colLastCommit {
			base[i].Width = lastCommitColumnWidth(m.lastCommitFormat)
		}
	}
	hidden := config.ViewSettings{HiddenColumns: m.hiddenColumns}
	out := make([]colDef, 0, len(base))
	for _, c := range base {
		if c.Toggleable && hidden.IsColumnHidden(string(c.Key)) {
			continue
		}
		out = append(out, c)
	}
	return out
}

// applyViewSettings rebuilds the bubbletea table columns and rerenders
// rows to reflect the current layout / format / hidden-columns state.
// Safe to call from any Update handler that changes a view-related field.
//
// Order matters: SetColumns internally re-renders the current rows via
// UpdateViewport. If the row width (cells) doesn't match the new column
// count we panic with an out-of-range index. So we clear rows first,
// swap columns, then rebuild rows shaped to the new column set.
func (m *Model) applyViewSettings() {
	cols := m.activeColumns()
	tableCols := make([]table.Column, len(cols))
	for i, c := range cols {
		tableCols[i] = table.Column{Title: c.Title, Width: c.Width}
	}
	m.table.SetRows(nil)
	m.table.SetColumns(tableCols)
	m.updateTable()
}

// reposToRowsForLayout converts repos to table rows using the currently
// active column set. Replaces the old fixed-9-column reposToRows.
func (m Model) reposToRowsForLayout(repos []model.Repo) []table.Row {
	cols := m.activeColumns()
	vp := viewportInfo{now: time.Now(), lastCommitFormat: m.lastCommitFormat}
	rows := make([]table.Row, 0, len(repos))
	for _, r := range repos {
		row := make(table.Row, len(cols))
		for i, c := range cols {
			row[i] = c.Render(r, vp)
		}
		rows = append(rows, row)
	}
	return rows
}

// --- cell renderers ---

func renderStatusCell(r model.Repo, _ viewportInfo) string {
	if r.Status.IsDirty {
		return "● Dirty"
	}
	return "✓ Clean"
}

func renderRepoCell(r model.Repo, _ viewportInfo) string {
	name := r.Name
	if r.IsWorktree {
		name = "⎇ " + name
	}
	return name
}

func renderBranchCell(r model.Repo, _ viewportInfo) string {
	return r.Status.Branch
}

// numCellRenderer returns a renderer that pulls a single int field and
// formats it with the "—" zero-suppression used everywhere.
func numCellRenderer(pick func(model.Repo) int) func(model.Repo, viewportInfo) string {
	return func(r model.Repo, _ viewportInfo) string {
		return formatNumber(pick(r))
	}
}

// renderChangesCell collapses staged/modified/untracked into a "S/M/U"
// triple — dashes for zero so dirty repos stand out at a glance.
func renderChangesCell(r model.Repo, _ viewportInfo) string {
	parts := []string{
		compactCount(r.Status.Staged),
		compactCount(r.Status.Unstaged),
		compactCount(r.Status.Untracked),
	}
	return strings.Join(parts, "/")
}

// renderSyncCell collapses ahead/behind. Arrows are kept ASCII-renderable
// in the few terminals that don't have full Unicode font coverage.
func renderSyncCell(r model.Repo, _ viewportInfo) string {
	ahead := "—"
	if r.Status.Ahead > 0 {
		ahead = fmt.Sprintf("↑%d", r.Status.Ahead)
	}
	behind := "—"
	if r.Status.Behind > 0 {
		behind = fmt.Sprintf("↓%d", r.Status.Behind)
	}
	if ahead == "—" && behind == "—" {
		return "—"
	}
	return ahead + " " + behind
}

func compactCount(n int) string {
	if n == 0 {
		return "—"
	}
	return fmt.Sprintf("%d", n)
}

func renderLastCommitCell(r model.Repo, vp viewportInfo) string {
	if r.Status.LastCommit.IsZero() {
		return "N/A"
	}
	switch vp.lastCommitFormat {
	case config.LastCommitDateYear:
		return r.Status.LastCommit.Format("2006-01-02 15:04")
	case config.LastCommitAgo:
		return stats.FormatTimeAgo(r.Status.LastCommit, vp.now)
	default:
		return r.Status.LastCommit.Format("Jan 02 15:04")
	}
}
