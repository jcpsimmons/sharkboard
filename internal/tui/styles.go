package tui

import "github.com/charmbracelet/lipgloss"

// Palette. The tool is a wire inspector, so the chrome stays quiet and colour
// is reserved for state that matters: link health, robot motion, writes.
var (
	colInk    = lipgloss.Color("#c9d1d9")
	colDim    = lipgloss.Color("#6e7681")
	colFaint  = lipgloss.Color("#3d444d")
	colAccent = lipgloss.Color("#58a6ff")
	colGood   = lipgloss.Color("#3fb950")
	colWarn   = lipgloss.Color("#d29922")
	colBad    = lipgloss.Color("#f85149")
	colLocal  = lipgloss.Color("#a371f7")
	colPanel  = lipgloss.Color("#1b2027")
)

var (
	// base is the default text style for body copy.
	base = lipgloss.NewStyle().Foreground(colInk)
	// dim is for secondary text and hints.
	dim = lipgloss.NewStyle().Foreground(colDim)
	// faint is for rules and inactive tabs.
	faint = lipgloss.NewStyle().Foreground(colFaint)
	// title is the app name in the header.
	title = lipgloss.NewStyle().Foreground(colInk).Bold(true)
	// addr is the broker address chip.
	addr = lipgloss.NewStyle().Foreground(colAccent)
	// good, warn and bad are link and state colours.
	good = lipgloss.NewStyle().Foreground(colGood).Bold(true)
	warn = lipgloss.NewStyle().Foreground(colWarn).Bold(true)
	bad  = lipgloss.NewStyle().Foreground(colBad).Bold(true)
	// local marks frames this tool published.
	local = lipgloss.NewStyle().Foreground(colLocal)

	// tab and tabActive are the top-level view switcher.
	tab       = lipgloss.NewStyle().Foreground(colDim).Padding(0, 1)
	tabActive = lipgloss.NewStyle().Foreground(lipgloss.Color("#0d1117")).Background(colAccent).Padding(0, 1).Bold(true)

	// panel is a bordered box for grouped content.
	panel = lipgloss.NewStyle().
		Border(lipgloss.RoundedBorder()).
		BorderForeground(colFaint).
		Padding(0, 1)
	// panelTitle sits on top of a panel's own content.
	panelTitle = lipgloss.NewStyle().Foreground(colDim).Bold(true)

	// rowSel is a selected list row: background only, no padding, so rows keep
	// their column alignment.
	rowSel = lipgloss.NewStyle().Foreground(lipgloss.Color("#0d1117")).Background(colAccent)

	// splash is the load card: a box that sits over the panes until a key.
	splashBox = lipgloss.NewStyle().
			Border(lipgloss.RoundedBorder()).
			BorderForeground(colAccent).
			Padding(1, 3)

	// toast is a transient confirmation line.
	toastOK   = lipgloss.NewStyle().Foreground(colGood)
	toastErr  = lipgloss.NewStyle().Foreground(colBad)
	toastInfo = lipgloss.NewStyle().Foreground(colWarn)
)
