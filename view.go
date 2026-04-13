package main

import (
	"strings"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
)

type aboutTickMsg struct{}

func aboutTick() tea.Cmd {
	return tea.Tick(150*time.Millisecond, func(_ time.Time) tea.Msg {
		return aboutTickMsg{}
	})
}

type headerTickMsg struct{}

func headerTick() tea.Cmd {
	return tea.Tick(150*time.Millisecond, func(_ time.Time) tea.Msg {
		return headerTickMsg{}
	})
}

type dockerRefreshTickMsg struct{}

func dockerRefreshTick() tea.Cmd {
	return tea.Tick(30*time.Second, func(_ time.Time) tea.Msg {
		return dockerRefreshTickMsg{}
	})
}

func (m model) View() string {
	if m.quitting {
		return ""
	}
	if m.state == stateList || m.about.open {
		header := renderHeader(m.headerFrame, len(m.rawHosts), countContainers(m.rawHosts))

		var scanStatus string
		if m.scanning {
			scanStatus = "\n " + m.spinner.View() + " " +
				lipgloss.NewStyle().Foreground(colorSecondary).Render("Scanning containers...") + "\n"
		}
		var deleteStatus string
		if m.listDelete.armed {
			deleteStatus = "\n " + testFailStyle.Render("Press again to confirm delete "+m.listDelete.kind+": "+m.listDelete.label+" (Esc to cancel)") + "\n"
		}

		var importStatus string
		if m.statusMessage != "" {
			style := testSuccessStyle
			marker := "✔"
			if m.statusIsError {
				style = testFailStyle
				marker = "✘"
			}
			importStatus = "\n " + style.Render(marker+" "+m.statusMessage) + "\n"
		}

		content := header + m.list.View() + scanStatus + deleteStatus + importStatus
		if m.err != nil {
			content += "\n" + testFailStyle.Render(" Config warning: "+m.err.Error())
		}
		help := "\n" + renderListHelp(m.list.SelectedItem())

		if m.about.open {
			modal := renderAboutModal(m.about.frame)
			overlay := lipgloss.Place(
				m.width, m.height,
				lipgloss.Center, lipgloss.Center,
				modal,
				lipgloss.WithWhitespaceChars(" "),
				lipgloss.WithWhitespaceForeground(lipgloss.Color("#000000")),
			)
			return overlay
		}

		return appStyle.Render(content + help)
	}
	if m.state == stateFilePicker {
		title := formTitleStyle.Render("📂 Select Identity File")
		content := fpBoxStyle.Render(m.filepicker.View())
		help := "\n" + renderFilePickerHelp()
		return appStyle.Render(title + "\n\n" + content + help)
	}
	if m.state == stateHistory {
		title := formTitleStyle.Render("Recent Connections")
		content := title + "\n\n" + m.historyList.View()
		help := "\n" + renderHistoryHelp()
		return appStyle.Render(content + help)
	}
	if m.state == stateGroupPrompt {
		title := "New Group"
		if m.groupPrompt.action == "rename" {
			title = "Rename Group"
		}
		box := formBoxStyle.Render(formTitleStyle.Render(title) + "\n\n" + m.groupPrompt.input.View())
		help := "\n" + helpBarStyle.Render(helpEntry("enter", "save")+" | "+helpEntry("esc", "cancel"))
		return appStyle.Render(box + help)
	}
	// Form View
	var formTitle string
	if m.selectedHost == nil {
		formTitle = formTitleStyle.Render("✨ New Session")
	} else {
		formTitle = formTitleStyle.Render("✎ Edit Session")
	}

	formWidth := 60
	if m.width > 0 {
		available := m.width - 8 // subtract appStyle padding + border
		if available < 40 {
			available = 40
		}
		if available < formWidth {
			formWidth = available
		}
	}
	dividerWidth := formWidth - 4
	if dividerWidth < 8 {
		dividerWidth = 8
	}
	divider := formDividerStyle.Render(strings.Repeat("─", dividerWidth))
	activeFormBoxStyle := formBoxStyle.Width(formWidth)

	// Build form content
	var formContent strings.Builder
	formContent.WriteString(formTitle + "\n\n")

	// Connection section
	formContent.WriteString(formSectionStyle.Render("  CONNECTION") + "\n")
	formContent.WriteString(divider + "\n")
	for i := 0; i < 6; i++ {
		formContent.WriteString(m.inputs[i].View() + "\n")
	}

	formContent.WriteString("\n")
	// Auth section
	formContent.WriteString(formSectionStyle.Render("  AUTHENTICATION") + "\n")
	formContent.WriteString(divider + "\n")
	pickStyle := lipgloss.NewStyle().
		Foreground(colorText).
		Background(colorSecondary).
		Bold(true).
		Padding(0, 1)
	if m.focusIndex == fieldKeyFile && m.keyPickFocus {
		pickStyle = pickStyle.Background(colorPrimary)
	}
	formContent.WriteString(lipgloss.JoinHorizontal(lipgloss.Top, m.inputs[fieldKeyFile].View(), "  ", pickStyle.Render("Pick")) + "\n")
	formContent.WriteString(m.inputs[fieldNotes].View() + "\n")
	formContent.WriteString(m.inputs[fieldPassword].View() + "\n")
	formContent.WriteString(m.inputs[fieldForwardAgent].View() + "\n")

	formContent.WriteString("\n")
	formContent.WriteString(formSectionStyle.Render("  GROUPS") + "\n")
	formContent.WriteString(divider + "\n")
	if m.groupCustom {
		formContent.WriteString(m.inputs[fieldGroup].View() + "\n")
	} else {
		groupLabelStyle := lipgloss.NewStyle().Foreground(colorMuted)
		groupValueStyle := lipgloss.NewStyle().Foreground(colorDimText)
		if m.focusIndex == fieldGroup {
			groupLabelStyle = lipgloss.NewStyle().Foreground(colorPrimary).Bold(true)
			groupValueStyle = lipgloss.NewStyle().Foreground(colorText)
		}
		groupValue := "(none)"
		if len(m.groupOptions) > 0 {
			groupValue = m.groupOptions[m.groupIndex]
		}
		formContent.WriteString(groupLabelStyle.Render("  Group       ") + groupValueStyle.Render("◀ "+groupValue+" ▶") + "\n")
	}

	if m.selectedHost != nil {
		label := "Delete Host"
		if m.deleteArmed {
			label = "Press Enter to Confirm Delete"
		}
		deleteStyle := lipgloss.NewStyle().
			Foreground(colorText).
			Background(colorDanger).
			Bold(true).
			Padding(0, 1)
		if !m.deleteFocus {
			deleteStyle = lipgloss.NewStyle().
				Foreground(colorDimText).
				Background(colorSubtle).
				Padding(0, 1)
		}
		formContent.WriteString("\n  " + deleteStyle.Render(label) + "\n")
		if m.deleteArmed {
			formContent.WriteString("  " + formHintStyle.Render("Esc to cancel") + "\n")
		}
	}

	// Test status
	if m.testing {
		formContent.WriteString("\n " + m.spinner.View() + " " +
			testPendingStyle.Render("Testing connection..."))
	} else if m.testStatus != "" {
		if m.testResult {
			formContent.WriteString("\n  " + testSuccessStyle.Render("✔ "+m.testStatus))
		} else {
			formContent.WriteString("\n  " + testFailStyle.Render("✘ "+m.testStatus))
		}
	}
	if m.formError != "" {
		formContent.WriteString("\n  " + testFailStyle.Render("✘ "+m.formError))
	}

	form := activeFormBoxStyle.Render(formContent.String())
	help := "\n" + renderFormHelp()

	return appStyle.Render(form + help)
}

func renderLogo(frame int) string {
	var b strings.Builder

	// Gradient colors: hot pink -> violet -> blue -> cyan (from anim.sh)
	c1 := lipgloss.Color("#FF50DC")
	c2 := lipgloss.Color("#DC5AFF")
	c3 := lipgloss.Color("#AA6EFF")
	c4 := lipgloss.Color("#788CFF")
	c5 := lipgloss.Color("#50BEFF")
	c6 := lipgloss.Color("#46EBFF")

	// Eye animation cycle (24 frames total):
	//   0-14: open eye, glow alternating
	//  15-20: open eye, charge alternating
	//     21: half eye
	//     22: closed eye
	//     23: half eye
	cycleFrame := frame % 24
	eye := "<_>"
	var eyeColor lipgloss.Color
	switch {
	case cycleFrame <= 14:
		if cycleFrame%2 == 0 {
			eyeColor = lipgloss.Color("#FFFFFF")
		} else {
			eyeColor = lipgloss.Color("#AAFFFF")
		}
	case cycleFrame <= 20:
		if cycleFrame%2 == 0 {
			eyeColor = lipgloss.Color("#FFFFB4")
		} else {
			eyeColor = lipgloss.Color("#FFFFFF")
		}
	case cycleFrame == 21 || cycleFrame == 23:
		eye = "-_-"
		eyeColor = lipgloss.Color("#F5F5F5")
	case cycleFrame == 22:
		eye = "---"
		eyeColor = lipgloss.Color("#F5F5F5")
	}

	// Logo lines matching anim.sh
	l1 := `   _____                  ___ ___         `
	l2 := `  /  _  \   ______ ______/   |   \  ____  `
	l3 := ` /  /_\  \ /  ___//  ___/    ~    \/  _ \ `
	l4pre := `/     |    \___ \ \___\      Y    `
	l5 := `\____|__  /____  >____  >\___|_  / \____/ `
	l6 := `        \/     \/     \/       \/         `

	eyeStyle := lipgloss.NewStyle().Foreground(eyeColor).Bold(true)
	l4 := l4pre + "(  " + eyeStyle.Render(eye) + lipgloss.NewStyle().Foreground(c4).Bold(true).Render(" )")

	render := func(text string, color lipgloss.Color) string {
		return lipgloss.NewStyle().Foreground(color).Bold(true).Render(text)
	}

	b.WriteString(render(l1, c1) + "\n")
	b.WriteString(render(l2, c2) + "\n")
	b.WriteString(render(l3, c3) + "\n")
	b.WriteString(render(l4, c4) + "\n")
	b.WriteString(render(l5, c5) + "\n")
	b.WriteString(render(l6, c6) + "\n")

	return b.String()
}

func renderAboutModal(frame int) string {
	var b strings.Builder

	const modalBg = lipgloss.Color("#0D0D0D")

	b.WriteString(renderLogo(frame))

	// Tagline
	tagline := lipgloss.NewStyle().Foreground(colorDimText).Italic(true).Background(modalBg).
		Render("          Another SSH Host Organizer")
	b.WriteString("\n" + tagline + "\n")

	// Divider
	divider := lipgloss.NewStyle().Foreground(colorSubtle).Background(modalBg).Render(strings.Repeat("━", 44))
	b.WriteString("\n" + divider + "\n\n")

	// Info rows
	sp := lipgloss.NewStyle().Background(modalBg)
	labelStyle := lipgloss.NewStyle().Foreground(colorSecondary).Bold(true).Width(14).Align(lipgloss.Right).Background(modalBg)
	valueStyle := lipgloss.NewStyle().Foreground(colorText).Background(modalBg)
	mutedStyle := lipgloss.NewStyle().Foreground(colorDimText).Background(modalBg)

	row := func(label, value string) string {
		return labelStyle.Render(label) + sp.Render("  ") + valueStyle.Render(value) + "\n"
	}

	b.WriteString(row("Version", version))
	b.WriteString(row("Author", "Allison"))
	b.WriteString(row("License", "MIT"))
	b.WriteString("\n")

	linkStyle := lipgloss.NewStyle().Foreground(colorHighlight).Underline(true).Background(modalBg)
	b.WriteString(labelStyle.Render("Source") + sp.Render("  ") + linkStyle.Render("github.com/allisonhere/assho") + "\n")
	b.WriteString("\n" + divider + "\n\n")

	// Built with
	b.WriteString(mutedStyle.Render("Built with") + sp.Render(" "))
	techs := []struct {
		name  string
		color lipgloss.Color
	}{
		{"Go", lipgloss.Color("#00ADD8")},
		{"Bubble Tea", colorPrimary},
		{"Lip Gloss", lipgloss.Color("#F472B6")},
	}
	for i, t := range techs {
		b.WriteString(lipgloss.NewStyle().Foreground(t.color).Bold(true).Background(modalBg).Render(t.name))
		if i < len(techs)-1 {
			b.WriteString(mutedStyle.Render(" · "))
		}
	}
	b.WriteString("\n\n")

	help := helpKeyStyle.Background(modalBg).Render("esc") + sp.Render(" ") + helpDescStyle.Background(modalBg).Render("close")
	b.WriteString(help)

	// Wrap in a bordered box
	modalBox := lipgloss.NewStyle().
		Border(lipgloss.RoundedBorder()).
		BorderForeground(colorPrimary).
		Padding(1, 3).
		Background(modalBg).
		Render(b.String())

	return modalBox
}
