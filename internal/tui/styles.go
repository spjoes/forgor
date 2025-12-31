package tui

import "github.com/charmbracelet/lipgloss"

var (
	// Colors taken from Catppuccin Mocha palette
	primaryColor   = lipgloss.Color("#89b4fa")
	secondaryColor = lipgloss.Color("#a6e3a1")
	dangerColor    = lipgloss.Color("#f38ba8")
	legacyColor    = lipgloss.Color("#fab387")
	mutedColor     = lipgloss.Color("#6c7086")
	bgColor        = lipgloss.Color("#1e1e2e")

	titleStyle = lipgloss.NewStyle().
			Bold(true).
			Foreground(primaryColor).
			MarginBottom(1)

	subtitleStyle = lipgloss.NewStyle().
			Foreground(mutedColor).
			MarginBottom(1)

	selectedStyle = lipgloss.NewStyle().
			Foreground(lipgloss.Color("#f5e0dc")).
			Background(primaryColor).
			Padding(0, 1)

	normalStyle = lipgloss.NewStyle().
			Foreground(lipgloss.Color("#f5e0dc")).
			Padding(0, 1)

	mutedStyle = lipgloss.NewStyle().
			Foreground(mutedColor)

	successStyle = lipgloss.NewStyle().
			Foreground(secondaryColor)

	errorStyle = lipgloss.NewStyle().
			Foreground(dangerColor)

	boxStyle = lipgloss.NewStyle().
			Border(lipgloss.RoundedBorder()).
			BorderForeground(primaryColor).
			Padding(1, 2)

	inputStyle = lipgloss.NewStyle().
			Border(lipgloss.RoundedBorder()).
			BorderForeground(mutedColor).
			Padding(0, 1)

	focusedInputStyle = lipgloss.NewStyle().
				Border(lipgloss.RoundedBorder()).
				BorderForeground(primaryColor).
				Padding(0, 1)

	tabStyle = lipgloss.NewStyle().
			Padding(0, 2).
			Foreground(mutedColor)

	activeTabStyle = lipgloss.NewStyle().
			Padding(0, 2).
			Foreground(lipgloss.Color("#f5e0dc")).
			Background(primaryColor).
			Bold(true)

	helpStyle = lipgloss.NewStyle().
			Foreground(mutedColor).
			MarginTop(1)

	legacyBadgeStyle = lipgloss.NewStyle().
				Foreground(legacyColor).
				Bold(true)

	v2BadgeStyle = lipgloss.NewStyle().
			Foreground(secondaryColor).
			Bold(true)

	logoStyle = lipgloss.NewStyle().
			Bold(true).
			Foreground(primaryColor)
)

const logo = `
 _____ ___  ____   ____  ___  ____  
|  ___/ _ \|  _ \ / ___|/ _ \|  _ \ 
| |_ | | | | |_) | |  _| | | | |_) |
|  _|| |_| |  _ <| |_| | |_| |  _ < 
|_|   \___/|_| \_\\____|\___/|_| \_\
`
