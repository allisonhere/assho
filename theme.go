package main

import (
	"fmt"
	"strings"

	"github.com/charmbracelet/bubbles/list"
	"github.com/charmbracelet/lipgloss"
)

// --- Theme ---

var (
	// Core palette
	colorPrimary   = lipgloss.Color("#7C3AED") // Vibrant purple
	colorSecondary = lipgloss.Color("#06B6D4") // Cyan
	colorAccent    = lipgloss.Color("#F59E0B") // Amber
	colorSuccess   = lipgloss.Color("#10B981") // Emerald
	colorDanger    = lipgloss.Color("#EF4444") // Red
	colorMuted     = lipgloss.Color("#6B7280") // Gray
	colorSubtle    = lipgloss.Color("#374151") // Dark gray
	colorText      = lipgloss.Color("#F9FAFB") // Near white
	colorDimText   = lipgloss.Color("#9CA3AF") // Dim text
	colorHighlight = lipgloss.Color("#A78BFA") // Light purple

	// App chrome
	appStyle = lipgloss.NewStyle().Padding(1, 2)

	titleStyle = lipgloss.NewStyle().
			Foreground(colorText).
			Background(colorPrimary).
			Bold(true).
			Padding(0, 1)

	headerDimStyle = lipgloss.NewStyle().
			Foreground(colorMuted)

	// List item styles
	itemNormalTitle = lipgloss.NewStyle().
			Foreground(colorText).
			PaddingLeft(2)

	itemNormalDesc = lipgloss.NewStyle().
			Foreground(colorDimText).
			PaddingLeft(2)

	itemSelectedTitle = lipgloss.NewStyle().
				Foreground(colorPrimary).
				Bold(true).
				PaddingLeft(1).
				Border(lipgloss.NormalBorder(), false, false, false, true).
				BorderForeground(colorPrimary)

	itemSelectedDesc = lipgloss.NewStyle().
				Foreground(colorHighlight).
				PaddingLeft(1).
				Border(lipgloss.NormalBorder(), false, false, false, true).
				BorderForeground(colorPrimary)

	// Form styles
	formBoxStyle = lipgloss.NewStyle().
			Border(lipgloss.RoundedBorder()).
			BorderForeground(colorPrimary).
			Padding(1, 2).
			Width(60)

	formTitleStyle = lipgloss.NewStyle().
			Foreground(colorText).
			Background(colorPrimary).
			Bold(true).
			Padding(0, 1).
			MarginBottom(1)

	formHintStyle = lipgloss.NewStyle().
			Foreground(colorMuted).
			Italic(true)

	formDividerStyle = lipgloss.NewStyle().
				Foreground(colorSubtle)

	// Status bar
	helpBarStyle = lipgloss.NewStyle().
			Foreground(colorDimText).
			Background(lipgloss.Color("#1F2937")).
			Padding(0, 1)

	helpKeyStyle = lipgloss.NewStyle().
			Foreground(colorAccent).
			Bold(true)

	helpDescStyle = lipgloss.NewStyle().
			Foreground(colorDimText)

	helpSepStyle = lipgloss.NewStyle().
			Foreground(colorSubtle)

	// Test result styles
	testSuccessStyle = lipgloss.NewStyle().
				Foreground(colorSuccess).
				Bold(true)

	testFailStyle = lipgloss.NewStyle().
			Foreground(colorDanger).
			Bold(true)

	testPendingStyle = lipgloss.NewStyle().
				Foreground(colorAccent).
				Bold(true)

	// FilePicker Styles
	fpDirStyle      = lipgloss.NewStyle().Foreground(colorSecondary)
	fpFileStyle     = lipgloss.NewStyle().Foreground(colorText)
	fpSelectedStyle = lipgloss.NewStyle().Foreground(colorPrimary).Bold(true)

	// File picker box
	fpBoxStyle = lipgloss.NewStyle().
			Border(lipgloss.RoundedBorder()).
			BorderForeground(colorSecondary).
			Padding(1, 2)

	// Spinner style
	spinnerStyle = lipgloss.NewStyle().Foreground(colorSecondary)
)

// --- ASCII Art Header ---

func renderHeader(frame int, hostCount int, containerCount int) string {
	logo := renderLogo(frame)

	taglinePlain := "Another SSH Organizer"
	tagline := lipgloss.NewStyle().
		Foreground(colorDimText).
		Render("Another " + lipgloss.NewStyle().Italic(true).Render("SSH") + " Organizer")
	// Logo lines are ~44 chars wide; right-align tagline
	taglinePad := 44 - lipgloss.Width(taglinePlain)
	if taglinePad < 0 {
		taglinePad = 0
	}
	tagline = strings.Repeat(" ", taglinePad) + tagline

	stats := headerDimStyle.Render(fmt.Sprintf("  %d hosts", hostCount))
	if containerCount > 0 {
		stats += headerDimStyle.Render(fmt.Sprintf(" · %d containers", containerCount))
	}

	return logo + tagline + "\n" + stats + "\n"
}

// --- Help Bar ---

func helpEntry(key, desc string) string {
	return helpKeyStyle.Render(key) + " " + helpDescStyle.Render(desc)
}

func renderListHelp(selected list.Item) string {
	var contextEntries []string

	switch item := selected.(type) {
	case Host:
		if item.IsContainer {
			contextEntries = []string{
				helpEntry("enter", "conn"),
			}
		} else {
			contextEntries = []string{
				helpEntry("enter", "conn"),
				helpEntry("e", "edit"),
				helpEntry("c", "dup"),
				helpEntry("d", "del"),
				helpEntry("space", "expand"),
				helpEntry("ctrl+d", "scan"),
				helpEntry("⇧↑↓", "move"),
			}
		}
	case groupItem:
		contextEntries = []string{
			helpEntry("enter", "toggle"),
			helpEntry("r", "rename"),
			helpEntry("d", "del"),
			helpEntry("⇧↑↓", "move"),
		}
	}

	baseEntries := []string{
		helpEntry("n", "new"),
		helpEntry("/", "filt"),
		helpEntry("h", "hist"),
		helpEntry("i", "imp"),
		helpEntry("ctrl+e", "exp"),
		helpEntry("a", "about"),
		helpEntry("q", "quit"),
	}

	entries := append(contextEntries, baseEntries...)
	sep := helpSepStyle.Render(" | ")
	return helpBarStyle.Render(strings.Join(entries, sep))
}

func renderFormHelp() string {
	entries := []string{
		helpEntry("tab", "next"),
		helpEntry("enter", "save"),
		helpEntry("ctrl+t", "test"),
		helpEntry("enter", "pick file"),
		helpEntry("arrows", "group"),
		helpEntry("esc", "cancel"),
	}
	sep := helpSepStyle.Render(" | ")
	return helpBarStyle.Render(strings.Join(entries, sep))
}

func renderHistoryHelp() string {
	entries := []string{
		helpEntry("enter", "conn"),
		helpEntry("e", "edit"),
		helpEntry("h", "back"),
		helpEntry("esc", "back"),
		helpEntry("q", "quit"),
	}
	sep := helpSepStyle.Render(" | ")
	return helpBarStyle.Render(strings.Join(entries, sep))
}

func renderFilePickerHelp() string {
	entries := []string{
		helpEntry("arrows", "nav"),
		helpEntry("enter", "select"),
		helpEntry("esc", "cancel"),
	}
	sep := helpSepStyle.Render(" | ")
	return helpBarStyle.Render(strings.Join(entries, sep))
}

