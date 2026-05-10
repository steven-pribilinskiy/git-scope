package tui

import (
	"fmt"
	"sort"
	"strings"
	"time"

	"github.com/Bharath-code/git-scope/internal/cache"
	"github.com/Bharath-code/git-scope/internal/config"
	"github.com/Bharath-code/git-scope/internal/model"
	"github.com/Bharath-code/git-scope/internal/stats"
	"github.com/charmbracelet/bubbles/spinner"
	"github.com/charmbracelet/bubbles/table"
	"github.com/charmbracelet/bubbles/textinput"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
)

// State represents the current UI state
type State int

const (
	StateLoading State = iota
	StateReady
	StateError
	StateSearching
	StateWorkspaceSwitch
	StateActionMenu
	StateActionEdit
)

// SortMode represents different sorting options
type SortMode int

const (
	SortByDirty SortMode = iota
	SortByName
	SortByBranch
	SortByLastCommit
)

// FilterMode represents different filter options
type FilterMode int

const (
	FilterAll FilterMode = iota
	FilterDirty
	FilterClean
)

// Model is the Bubbletea model for the TUI
type Model struct {
	cfg           *config.Config
	table         table.Model
	textInput     textinput.Model
	spinner       spinner.Model
	repos         []model.Repo
	filteredRepos []model.Repo // After filter applied
	sortedRepos   []model.Repo // After sort applied
	state         State
	err           error
	statusMsg     string
	width         int
	height        int
	sortMode      SortMode
	filterMode    FilterMode
	searchQuery   string
	// Panel state
	activePanel  PanelType
	grassData    *stats.ContributionData
	diskData     *stats.DiskUsageData
	timelineData *stats.TimelineData
	// Panel render cache. Rendering a panel involves O(repos) styled-string
	// construction and lipgloss ANSI work — doing that on every key press
	// while a panel is open produces visible repaint churn. We render once
	// per data-update / resize and reuse the string everywhere else.
	// Empty string means "rebuild on next read".
	grassRendered    string
	diskRendered     string
	timelineRendered string
	// Workspace switch state
	workspaceInput  textinput.Model
	workspaceError  string
	activeWorkspace string
	// Star nudge state
	showStarNudge         bool
	nudgeShownThisSession bool
	// Pagination state
	currentPage int
	pageSize    int
	// Live toggle for including linked worktrees in the displayed set.
	// Initialised from cfg.IncludeWorktrees; can be flipped at runtime via 'W'.
	includeWorktrees bool
	// Whether the most recent scan that produced m.repos included worktrees.
	// If true and the user toggles worktrees off, we can filter in-memory
	// without rescanning. If false and the user toggles them on, we must rescan.
	lastScanIncludesWorktrees bool
	// True while a stale-while-revalidate background refresh is in flight.
	// Drives a subtle "↻ refreshing" indicator in the stats bar.
	refreshing bool
	// Timestamp of the data currently in m.repos when it came from cache.
	// Used by Init to decide whether to kick off a background refresh.
	cachedAt time.Time
	// Action-menu state (StateActionMenu).
	// actionCursor: index of the highlighted action in cfg.Actions.
	// confirmDelete: true while a delete is awaiting one-keystroke confirm.
	actionCursor  int
	confirmDelete bool
	// Action-edit state (StateActionEdit).
	// actionEditBuf: the in-flight action being edited; on save it replaces
	// cfg.Actions[actionEditIndex], or appends when actionEditIndex == -1.
	actionEditBuf   config.Action
	actionEditIndex int
	actionEditField int // 0=key, 1=label, 2=value (run or clipboard)
	actionKeyInput  textinput.Model
	actionLabelIn   textinput.Model
	actionValueIn   textinput.Model
	actionEditErr   string
}

// NewModel creates a new TUI model
func NewModel(cfg *config.Config) Model {
	columns := []table.Column{
		{Title: "Status", Width: 8},
		{Title: "Repository", Width: 18},
		{Title: "Branch", Width: 14},
		{Title: "Staged", Width: 6},
		{Title: "Modified", Width: 8},
		{Title: "Untracked", Width: 9},
		{Title: "Ahead", Width: 7},
		{Title: "Behind", Width: 7},
		{Title: "Last Commit", Width: 14},
	}

	t := table.New(
		table.WithColumns(columns),
		table.WithRows([]table.Row{}),
		table.WithFocused(true),
		table.WithHeight(12),
	)

	// Apply modern table styles with strong highlighting
	s := table.DefaultStyles()
	s.Header = s.Header.
		BorderStyle(lipgloss.NormalBorder()).
		BorderForeground(lipgloss.Color("#7C3AED")).
		BorderBottom(true).
		Bold(true).
		Foreground(lipgloss.Color("#FFFFFF")).
		Background(lipgloss.Color("#7C3AED")).
		Padding(0, 1)

	// Strong row highlighting
	s.Selected = s.Selected.
		Foreground(lipgloss.Color("#000000")).
		Background(lipgloss.Color("#A78BFA")).
		Bold(true)

	s.Cell = s.Cell.
		Padding(0, 1)

	t.SetStyles(s)

	// Create text input for search
	ti := textinput.New()
	ti.Placeholder = "Search repos..."
	ti.CharLimit = 50
	ti.Width = 30

	// Create text input for workspace switch
	wi := textinput.New()
	wi.Placeholder = "~/projects or /path/to/dir"
	wi.CharLimit = 200
	wi.Width = 40

	// Text inputs for action-edit modal.
	akey := textinput.New()
	akey.Placeholder = "alt+r"
	akey.CharLimit = 16
	akey.Width = 20
	alabel := textinput.New()
	alabel.Placeholder = "Resume Claude"
	alabel.CharLimit = 60
	alabel.Width = 40
	avalue := textinput.New()
	avalue.Placeholder = "command {path}  —  or text to copy"
	avalue.CharLimit = 400
	avalue.Width = 60

	// Create spinner with Braille pattern
	sp := spinner.New()
	sp.Spinner = spinner.Dot
	sp.Style = lipgloss.NewStyle().Foreground(lipgloss.Color("#7C3AED"))

	m := Model{
		cfg:              cfg,
		table:            t,
		textInput:        ti,
		workspaceInput:   wi,
		spinner:          sp,
		state:            StateLoading,
		sortMode:         SortByDirty,
		filterMode:       FilterAll,
		currentPage:      0,
		pageSize:         cfg.PageSize,
		includeWorktrees: cfg.IncludeWorktrees,
		actionKeyInput:   akey,
		actionLabelIn:    alabel,
		actionValueIn:    avalue,
		actionEditIndex:  -1,
	}

	// Pre-load the cache synchronously so warm starts boot straight into
	// the dashboard with no loading flash. The on-disk cache is just a
	// JSON file — reading it costs ~ms. We also decide here whether the
	// cache is stale (a background refresh will be kicked off by Init).
	store := cache.NewFileStore()
	if cached, err := store.Load(); err == nil && store.IsSameRoots(cfg.Roots) {
		m.repos = cached.Repos
		m.lastScanIncludesWorktrees = cached.IncludeWorktrees
		m.state = StateReady
		m.cachedAt = cached.Timestamp
		m.updateTable()
		if time.Since(cached.Timestamp) > cfg.CacheFreshnessDuration() {
			m.refreshing = true
		}
	}
	return m
}

// Init initializes the model. The cache was already loaded synchronously
// in NewModel; here we only schedule work:
//   - state==Loading: no cache — run a cold scan, spinner ticks.
//   - refreshing==true: stale cache — kick off the background refresh.
//   - otherwise: fresh cache, nothing to do.
func (m Model) Init() tea.Cmd {
	if m.state != StateReady {
		return tea.Batch(m.spinner.Tick, refreshScanCmd(m.cfg, m.includeWorktrees))
	}
	if m.refreshing {
		return refreshScanCmd(m.cfg, m.includeWorktrees)
	}
	return nil
}

// GetSelectedRepo returns the currently selected repo
func (m Model) GetSelectedRepo() *model.Repo {
	if m.state != StateReady || len(m.sortedRepos) == 0 {
		return nil
	}

	// Get the cursor position within the current page
	cursor := m.table.Cursor()
	// Calculate the actual index in sortedRepos
	actualIndex := m.currentPage*m.pageSize + cursor

	if actualIndex >= 0 && actualIndex < len(m.sortedRepos) {
		return &m.sortedRepos[actualIndex]
	}
	return nil
}

// applyFilter filters repos based on current filter mode and search query
func (m *Model) applyFilter() {
	m.filteredRepos = make([]model.Repo, 0, len(m.repos))

	for _, r := range m.repos {
		// Worktree visibility — also affects stats totals.
		if !m.includeWorktrees && r.IsWorktree {
			continue
		}

		// Apply filter mode
		switch m.filterMode {
		case FilterDirty:
			if !r.Status.IsDirty {
				continue
			}
		case FilterClean:
			if r.Status.IsDirty {
				continue
			}
		}

		// Apply search query
		if m.searchQuery != "" {
			query := strings.ToLower(m.searchQuery)
			name := strings.ToLower(r.Name)
			branch := strings.ToLower(r.Status.Branch)

			// Only search Name and Branch to avoid matching parent paths
			if !strings.Contains(name, query) &&
				!strings.Contains(branch, query) {
				continue
			}
		}

		m.filteredRepos = append(m.filteredRepos, r)
	}
}

// sortRepos sorts the filtered repos based on current sort mode
func (m *Model) sortRepos() {
	m.sortedRepos = make([]model.Repo, len(m.filteredRepos))
	copy(m.sortedRepos, m.filteredRepos)

	switch m.sortMode {
	case SortByDirty:
		sort.Slice(m.sortedRepos, func(i, j int) bool {
			if m.sortedRepos[i].Status.IsDirty != m.sortedRepos[j].Status.IsDirty {
				return m.sortedRepos[i].Status.IsDirty
			}
			return m.sortedRepos[i].Name < m.sortedRepos[j].Name
		})
	case SortByName:
		sort.Slice(m.sortedRepos, func(i, j int) bool {
			return m.sortedRepos[i].Name < m.sortedRepos[j].Name
		})
	case SortByBranch:
		sort.Slice(m.sortedRepos, func(i, j int) bool {
			return m.sortedRepos[i].Status.Branch < m.sortedRepos[j].Status.Branch
		})
	case SortByLastCommit:
		sort.Slice(m.sortedRepos, func(i, j int) bool {
			return m.sortedRepos[i].Status.LastCommit.After(m.sortedRepos[j].Status.LastCommit)
		})
	}
}

// updateTable refreshes the table with current filtered and sorted repos
func (m *Model) updateTable() {
	m.applyFilter()
	m.sortRepos()
	m.table.SetRows(reposToRows(m.getCurrentPageRepos()))
}

// getTotalPages returns the total number of pages
func (m Model) getTotalPages() int {
	if len(m.sortedRepos) == 0 {
		return 1
	}
	return (len(m.sortedRepos) + m.pageSize - 1) / m.pageSize
}

// getCurrentPageRepos returns repos for the current page
func (m Model) getCurrentPageRepos() []model.Repo {
	if len(m.sortedRepos) == 0 {
		return []model.Repo{}
	}

	start := m.currentPage * m.pageSize
	end := start + m.pageSize

	if start >= len(m.sortedRepos) {
		start = 0
		end = m.pageSize
	}
	if end > len(m.sortedRepos) {
		end = len(m.sortedRepos)
	}

	return m.sortedRepos[start:end]
}

// canGoPrev returns true if there's a previous page
func (m Model) canGoPrev() bool {
	return m.currentPage > 0
}

// canGoNext returns true if there's a next page
func (m Model) canGoNext() bool {
	return m.currentPage < m.getTotalPages()-1
}

// resetPage resets pagination to first page
func (m *Model) resetPage() {
	m.currentPage = 0
}

// GetSortModeName returns the display name of current sort mode
func (m Model) GetSortModeName() string {
	switch m.sortMode {
	case SortByDirty:
		return "Dirty First"
	case SortByName:
		return "Name"
	case SortByBranch:
		return "Branch"
	case SortByLastCommit:
		return "Recent"
	}
	return "Unknown"
}

// GetFilterModeName returns the display name of current filter mode
func (m Model) GetFilterModeName() string {
	switch m.filterMode {
	case FilterAll:
		return "All"
	case FilterDirty:
		return "Dirty Only"
	case FilterClean:
		return "Clean Only"
	}
	return "All"
}

// reposToRows converts repos to table rows with status indicators
func reposToRows(repos []model.Repo) []table.Row {
	rows := make([]table.Row, 0, len(repos))
	for _, r := range repos {
		lastCommit := "N/A"
		if !r.Status.LastCommit.IsZero() {
			lastCommit = r.Status.LastCommit.Format("Jan 02 15:04")
		}

		// Status indicator with text
		status := "✓ Clean"
		if r.Status.IsDirty {
			status = "● Dirty"
		}

		name := r.Name
		if r.IsWorktree {
			name = "⎇ " + name
		}

		rows = append(rows, table.Row{
			status,
			truncateString(name, 18),
			truncateString(r.Status.Branch, 14),
			formatNumber(r.Status.Staged),
			formatNumber(r.Status.Unstaged),
			formatNumber(r.Status.Untracked),
			formatNumber(r.Status.Ahead),
			formatNumber(r.Status.Behind),
			lastCommit,
		})
	}
	return rows
}

// truncateString shortens a string with ellipsis
func truncateString(s string, maxLen int) string {
	if len(s) <= maxLen {
		return s
	}
	return s[:maxLen-1] + "…"
}

// formatNumber formats a number for display
func formatNumber(n int) string {
	if n == 0 {
		return "—"
	}
	return fmt.Sprintf("%d", n)
}

// resizeTable calculates and sets the correct table height based on UI state
func (m *Model) resizeTable() {
	// Header(2) + Stats(1) + gap(1) + table-trailing(1) + status(1) +
	// legend(1) + help(1) + safety(4) = 12.
	usedHeight := 12
	if m.state == StateSearching {
		usedHeight += 3 // Search input
	} else if m.searchQuery != "" {
		usedHeight += 1 // Search badge
	}

	h := m.height - usedHeight
	if h < 1 {
		h = 1
	}
	m.table.SetHeight(h)

	// Ensure page size is at least 5 to avoid too small pages
	if h < 5 {
		h = 5
	}
	m.pageSize = h  // Update page size based on new height
	m.updateTable() // Refresh table to apply new page size
}
