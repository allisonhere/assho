package main

import (
	"strings"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/charmbracelet/x/ansi"
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
	if m.about.open {
		return m.renderAboutView()
	}
	switch m.state {
	case stateList:
		return m.renderListView()
	case stateFilePicker:
		return m.renderFilePickerView()
	case stateHistory:
		return m.renderHistoryView()
	case stateGroupPrompt:
		return m.renderGroupPromptView()
	case stateForm:
		return m.renderFormView()
	}
	return ""
}

func (m model) renderListView() string {
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
	if m.status.message != "" {
		style := testSuccessStyle
		marker := "✔"
		if m.status.isError {
			style = testFailStyle
			marker = "✘"
		}
		importStatus = "\n " + style.Render(marker+" "+m.status.message) + "\n"
	}

	content := header + m.list.View() + scanStatus + deleteStatus + importStatus
	if m.err != nil {
		content += "\n" + testFailStyle.Render(" Config warning: "+m.err.Error())
	}
	help := "\n" + renderListHelp(m.list.SelectedItem())
	return appStyle.Render(content + help)
}

func (m model) renderAboutView() string {
	base := dimBase(m.renderListView())
	modal := renderAboutModal(m.about.frame)
	return overlayCenter(base, modal, m.width, m.height)
}

// dimBase strips existing ANSI styling from each line and re-renders it in a
// muted gray, producing a scrim effect for modal overlays.
func dimBase(s string) string {
	dim := lipgloss.NewStyle().Foreground(colorMuted)
	lines := strings.Split(s, "\n")
	for i, l := range lines {
		lines[i] = dim.Render(ansi.Strip(l))
	}
	return strings.Join(lines, "\n")
}

// overlayCenter composites modal lines centered over base lines. Base lines
// that fall outside the modal's bounding box pass through unchanged, so the
// list view remains visible around the edges instead of being blacked out.
func overlayCenter(base, modal string, width, height int) string {
	baseLines := strings.Split(base, "\n")
	modalLines := strings.Split(modal, "\n")

	modalH := len(modalLines)
	modalW := 0
	for _, l := range modalLines {
		if w := lipgloss.Width(l); w > modalW {
			modalW = w
		}
	}

	baseH := len(baseLines)
	if height > baseH {
		baseH = height
	}
	baseW := width
	for _, l := range baseLines {
		if w := lipgloss.Width(l); w > baseW {
			baseW = w
		}
	}

	top := (baseH - modalH) / 2
	if top < 0 {
		top = 0
	}
	left := (baseW - modalW) / 2
	if left < 0 {
		left = 0
	}

	out := make([]string, baseH)
	for i := 0; i < baseH; i++ {
		if i < len(baseLines) {
			out[i] = baseLines[i]
		}
		mi := i - top
		if mi < 0 || mi >= modalH {
			continue
		}
		// Truncate or pad base line to exactly `left` cells so the modal
		// starts at the centered column regardless of base line width.
		baseW := lipgloss.Width(out[i])
		switch {
		case baseW > left:
			out[i] = ansi.Truncate(out[i], left, "")
		case baseW < left:
			out[i] += strings.Repeat(" ", left-baseW)
		}
		out[i] += modalLines[mi]
	}
	return strings.Join(out, "\n")
}

func (m model) renderFilePickerView() string {
	title := formTitleStyle.Render("📂 Select Identity File")
	content := fpBoxStyle.Render(m.filepicker.View())
	help := "\n" + renderFilePickerHelp()
	return appStyle.Render(title + "\n\n" + content + help)
}

func (m model) renderHistoryView() string {
	title := formTitleStyle.Render("Recent Connections")
	content := title + "\n\n" + m.historyList.View()
	help := "\n" + renderHistoryHelp()
	return appStyle.Render(content + help)
}

func (m model) renderGroupPromptView() string {
	title := "New Group"
	if m.groupPrompt.action == "rename" {
		title = "Rename Group"
	}
	box := formBoxStyle.Render(formTitleStyle.Render(title) + "\n\n" + m.groupPrompt.input.View())
	help := "\n" + helpBarStyle.Render(helpEntry("enter", "save")+" | "+helpEntry("esc", "cancel"))
	return appStyle.Render(box + help)
}

func (m model) renderFormView() string {
	available := 72
	if m.width > 0 {
		available = m.width - 8
	}
	if available < 44 {
		available = 44
	}

	var content string
	if available >= 96 {
		mainWidth := available - 28
		if mainWidth < 54 {
			mainWidth = 54
		}
		sidebarWidth := available - mainWidth - 2
		if sidebarWidth < 24 {
			sidebarWidth = 24
			mainWidth = available - sidebarWidth - 2
		}
		content = lipgloss.JoinHorizontal(
			lipgloss.Top,
			m.renderFormMainPanel(mainWidth),
			"  ",
			m.renderFormSidebar(sidebarWidth),
		)
	} else {
		mainWidth := available
		content = lipgloss.JoinVertical(
			lipgloss.Left,
			m.renderFormTitle(),
			m.renderFormMainPanel(mainWidth),
			m.renderFormSidebar(mainWidth),
		)
	}

	help := "\n" + renderFormHelp()
	return appStyle.Render(content + help)
}

func (m model) renderFormTitle() string {
	if m.form.selectedHost == nil {
		return formTitleStyle.Render("✨ New Session")
	}
	return formTitleStyle.Render("✎ Edit Session")
}

func (m model) renderFormMainPanel(width int) string {
	var b strings.Builder
	b.WriteString(m.renderFormSection("ENDPOINT", width,
		m.form.inputs[fieldAlias].View(),
		m.form.inputs[fieldHostname].View(),
		m.form.inputs[fieldUser].View(),
		m.form.inputs[fieldPort].View(),
	))
	b.WriteString("\n\n")
	b.WriteString(m.renderFormSection("AUTH", width,
		m.renderKeyFileRow(),
		m.form.inputs[fieldPassword].View(),
		m.form.inputs[fieldForwardAgent].View(),
	))
	b.WriteString("\n\n")
	b.WriteString(m.renderFormSection("ADVANCED", width,
		m.form.inputs[fieldProxyJump].View(),
		m.form.inputs[fieldLocalForward].View(),
	))
	b.WriteString("\n\n")
	b.WriteString(m.renderFormSection("ORGANIZATION", width, m.renderGroupRow()))
	b.WriteString("\n\n")
	b.WriteString(m.renderFormSection("METADATA", width, m.form.inputs[fieldNotes].View()))
	return formBoxStyle.Width(width).Render(b.String())
}

func (m model) renderFormSection(title string, width int, rows ...string) string {
	dividerWidth := width - 6
	if dividerWidth < 8 {
		dividerWidth = 8
	}
	var b strings.Builder
	b.WriteString(formSectionStyle.Render("  "+title) + "\n")
	b.WriteString(formDividerStyle.Render(strings.Repeat("─", dividerWidth)) + "\n")
	for i, row := range rows {
		b.WriteString(row)
		if i < len(rows)-1 {
			b.WriteString("\n")
		}
	}
	return b.String()
}

func (m model) renderKeyFileRow() string {
	pickStyle := lipgloss.NewStyle().
		Foreground(colorText).
		Background(colorSecondary).
		Bold(true).
		Padding(0, 1)
	if m.form.focusIndex == fieldKeyFile && m.form.keyPickFocus {
		pickStyle = pickStyle.Background(colorPrimary)
	}
	return lipgloss.JoinHorizontal(
		lipgloss.Top,
		m.form.inputs[fieldKeyFile].View(),
		"  ",
		pickStyle.Render("Pick"),
	)
}

func (m model) renderGroupRow() string {
	if m.form.groupCustom {
		return m.form.inputs[fieldGroup].View()
	}
	groupLabelStyle := lipgloss.NewStyle().Foreground(colorMuted)
	groupValueStyle := lipgloss.NewStyle().Foreground(colorDimText)
	if m.form.focusIndex == fieldGroup {
		groupLabelStyle = lipgloss.NewStyle().Foreground(colorPrimary).Bold(true)
		groupValueStyle = lipgloss.NewStyle().Foreground(colorText)
	}
	groupValue := "(none)"
	if len(m.form.groupOptions) > 0 {
		groupValue = m.form.groupOptions[m.form.groupIndex]
	}
	return groupLabelStyle.Render("  Group       ") + groupValueStyle.Render("◀ "+groupValue+" ▶")
}

func (m model) renderFormSidebar(width int) string {
	var b strings.Builder
	b.WriteString(m.renderFormTitle() + "\n\n")
	if m.form.selectedHost == nil {
		b.WriteString(formHintStyle.Render("Create a new SSH session profile.") + "\n")
	} else {
		b.WriteString(formHintStyle.Render("Editing "+m.form.selectedHost.Alias) + "\n")
	}
	b.WriteString("\n")
	b.WriteString(formSectionStyle.Render("  ACTIONS") + "\n")
	b.WriteString(formDividerStyle.Render(strings.Repeat("─", max(width-6, 8))) + "\n")
	b.WriteString("  " + helpKeyStyle.Render("Ctrl+T") + " " + helpDescStyle.Render("test connection") + "\n")
	b.WriteString("  " + helpKeyStyle.Render("Enter") + " " + helpDescStyle.Render("next field / save on Notes") + "\n")
	b.WriteString("  " + helpKeyStyle.Render("Esc") + " " + helpDescStyle.Render("cancel") + "\n")

	if status := m.renderFormStatus(); status != "" {
		b.WriteString("\n")
		b.WriteString(formSectionStyle.Render("  STATUS") + "\n")
		b.WriteString(formDividerStyle.Render(strings.Repeat("─", max(width-6, 8))) + "\n")
		b.WriteString(status + "\n")
	}

	if m.form.selectedHost != nil {
		b.WriteString("\n")
		label := "Delete Host"
		if m.form.deleteArmed {
			label = "Press Enter to Confirm Delete"
		}
		deleteStyle := lipgloss.NewStyle().
			Foreground(colorText).
			Background(colorDanger).
			Bold(true).
			Padding(0, 1)
		if !m.form.deleteFocus {
			deleteStyle = lipgloss.NewStyle().
				Foreground(colorDimText).
				Background(colorSubtle).
				Padding(0, 1)
		}
		b.WriteString("  " + deleteStyle.Render(label))
		if m.form.deleteArmed {
			b.WriteString("\n  " + formHintStyle.Render("Esc to cancel"))
		}
	}

	return formBoxStyle.Width(width).Render(b.String())
}

func (m model) renderFormStatus() string {
	if m.form.testing {
		return " " + m.spinner.View() + " " + testPendingStyle.Render("Testing connection...")
	}
	if m.form.formError != "" {
		return "  " + testFailStyle.Render("✘ "+m.form.formError)
	}
	if m.form.testStatus != "" {
		if m.form.testResult {
			return "  " + testSuccessStyle.Render("✔ "+m.form.testStatus)
		}
		return "  " + testFailStyle.Render("✘ "+m.form.testStatus)
	}
	return ""
}

func max(a, b int) int {
	if a > b {
		return a
	}
	return b
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
