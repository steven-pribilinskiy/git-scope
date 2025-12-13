package tui

import (
	"github.com/charmbracelet/lipgloss"
)

// Tuimorphic Color Palette - Inspired by modern TUI designs
// Dark theme with GitHub-style colors and strong visual hierarchy
var (
	// Primary accent (purple - brand color)
	primaryColor = lipgloss.Color("#7C3AED")
	primaryDim   = lipgloss.Color("#A78BFA")

	// Secondary colors
	secondaryColor = lipgloss.Color("#10B981") // Green
	accentColor    = lipgloss.Color("#F59E0B") // Amber

	// Semantic status colors
	cleanColor = lipgloss.Color("#22c55e") // Green - clean/success
	dirtyColor = lipgloss.Color("#eab308") // Amber/Yellow - dirty/warning
	errorColor = lipgloss.Color("#ef4444") // Red - error

	// Background layers (dark theme - GitHub style)
	bgDark       = lipgloss.Color("#0d1117") // Darkest - main bg
	bgPanel      = lipgloss.Color("#161b22") // Panel backgrounds
	bgSurface    = lipgloss.Color("#21262d") // Elevated surfaces
	borderColor  = lipgloss.Color("#30363d") // Subtle borders
	borderActive = lipgloss.Color("#7C3AED") // Active/focused borders

	// Text hierarchy
	textPrimary   = lipgloss.Color("#f0f6fc") // Primary text
	textSecondary = lipgloss.Color("#8b949e") // Secondary/muted
	textTertiary  = lipgloss.Color("#6e7681") // Tertiary/hints

	// Legacy aliases for compatibility
	bgColor      = bgPanel
	surfaceColor = bgSurface
	textColor    = textPrimary
	mutedColor   = textSecondary
	dangerColor  = errorColor
)

// Application styles
var (
	// App container - darker background
	appStyle = lipgloss.NewStyle().
			Padding(1, 2)

	// Header / Title
	titleStyle = lipgloss.NewStyle().
			Bold(true).
			Foreground(lipgloss.Color("#FFFFFF")).
			Background(primaryColor).
			Padding(0, 2).
			MarginBottom(1)

	// Logo ASCII art style
	logoStyle = lipgloss.NewStyle().
			Foreground(primaryColor).
			Bold(true)

	// Header bar style (logo + version)
	headerBarStyle = lipgloss.NewStyle().
			Foreground(primaryDim).
			Bold(true)

	versionStyle = lipgloss.NewStyle().
			Foreground(textTertiary)

	// Subtitle with stats
	subtitleStyle = lipgloss.NewStyle().
			Foreground(mutedColor).
			MarginBottom(1)

	// Stats badges
	statsBadgeStyle = lipgloss.NewStyle().
			Foreground(textPrimary).
			Background(bgSurface).
			Padding(0, 1).
			MarginRight(1)

	dirtyBadgeStyle = lipgloss.NewStyle().
			Foreground(lipgloss.Color("#000000")).
			Background(dirtyColor).
			Padding(0, 1).
			Bold(true)

	cleanBadgeStyle = lipgloss.NewStyle().
			Foreground(lipgloss.Color("#000000")).
			Background(cleanColor).
			Padding(0, 1).
			Bold(true)

	// Table styles - bordered container
	tableContainerStyle = lipgloss.NewStyle().
				BorderStyle(lipgloss.RoundedBorder()).
				BorderForeground(borderColor).
				Padding(0, 1)

	// Dashboard border style
	dashboardBorderStyle = lipgloss.NewStyle().
				BorderStyle(lipgloss.RoundedBorder()).
				BorderForeground(borderColor).
				Padding(0, 1)

	// Keybindings bar styles (Tuimorphic - always visible at bottom)
	keyBindingsBarStyle = lipgloss.NewStyle().
				Foreground(textSecondary).
				MarginTop(1)

	keyBindingKeyStyle = lipgloss.NewStyle().
				Foreground(primaryDim).
				Bold(true)

	keyBindingSepStyle = lipgloss.NewStyle().
				Foreground(borderColor)

	// Inline hint style
	hintStyle = lipgloss.NewStyle().
			Foreground(textTertiary)

	// Help footer (legacy - now using keybindings bar)
	helpStyle = lipgloss.NewStyle().
			Foreground(mutedColor).
			MarginTop(1)

	helpKeyStyle = lipgloss.NewStyle().
			Foreground(primaryColor).
			Bold(true)

	helpDescStyle = lipgloss.NewStyle().
			Foreground(mutedColor)

	// Status message
	statusStyle = lipgloss.NewStyle().
			Foreground(accentColor).
			MarginTop(1)

	// Error styling
	errorTitleStyle = lipgloss.NewStyle().
			Foreground(errorColor).
			Bold(true)

	errorBoxStyle = lipgloss.NewStyle().
			BorderStyle(lipgloss.RoundedBorder()).
			BorderForeground(errorColor).
			Padding(1, 2).
			MarginTop(1)

	// Loading styling
	loadingStyle = lipgloss.NewStyle().
			Foreground(secondaryColor).
			Bold(true)

	loadingSpinnerStyle = lipgloss.NewStyle().
				Foreground(primaryColor)

	// Scanning paths list
	pathStyle = lipgloss.NewStyle().
			Foreground(textColor).
			PaddingLeft(2)

	pathBulletStyle = lipgloss.NewStyle().
			Foreground(primaryColor).
			Bold(true)

	// Repo row indicators
	dirtyIndicator = lipgloss.NewStyle().
			Foreground(dirtyColor).
			Bold(true).
			Render("●")

	cleanIndicator = lipgloss.NewStyle().
			Foreground(cleanColor).
			Render("○")

	// Compact legend styles
	dirtyDotStyle = lipgloss.NewStyle().
			Foreground(dirtyColor).
			Bold(true)

	cleanDotStyle = lipgloss.NewStyle().
			Foreground(cleanColor)

	legendStyle = lipgloss.NewStyle().
			Foreground(textTertiary)
)

// Help item creates a styled help key-description pair
func helpItem(key, desc string) string {
	return helpKeyStyle.Render(key) + helpDescStyle.Render(" "+desc)
}

// Logo returns the ASCII art logo
func logo() string {
	return logoStyle.Render(`
  ┏━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━┓
  ┃   ╔═╗╦╔╦╗  ╔═╗╔═╗╔═╗╔═╗╔═╗   ┃
  ┃   ║ ╦║ ║───╚═╗║  ║ ║╠═╝║╣    ┃
  ┃   ╚═╝╩ ╩   ╚═╝╚═╝╚═╝╩  ╚═╝   ┃
  ┗━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━┛`)
}

// Simpler logo for compact mode
func compactLogo() string {
	return titleStyle.Render(" 🔍 git-scope ")
}
