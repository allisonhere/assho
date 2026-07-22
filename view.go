package main

import (
	"fmt"
	"strings"
	"time"

	"github.com/charmbracelet/bubbles/viewport"
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
	var view string
	if m.helpOpen {
		view = m.renderHelpView()
	} else if m.about.open {
		view = m.renderAboutView()
	} else {
		switch m.state {
		case stateList:
			view = m.renderListView()
		case stateFilePicker:
			view = m.renderFilePickerView()
		case stateHistory:
			view = m.renderHistoryView()
		case stateGroupPrompt:
			view = m.renderGroupPromptView()
		case stateForm:
			view = m.renderFormView()
		case stateKeyInstall:
			view = m.renderKeyInstallView()
		case stateRotation:
			view = m.renderRotationView()
		}
	}
	if m.hostTrust.open {
		return m.renderHostTrustOverlay(view)
	}
	return view
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

func (m model) renderHelpView() string {
	var base string
	if m.state == stateForm {
		base = dimBase(m.renderFormView())
	} else {
		base = dimBase(m.renderListView())
	}
	modal := renderHelpModal()
	return overlayCenter(base, modal, m.width, m.height)
}

func renderHelpModal() string {
	const modalBg = lipgloss.Color("#0D0D0D")

	sp := lipgloss.NewStyle().Background(modalBg)
	titleStyle := lipgloss.NewStyle().Foreground(colorText).Background(modalBg).Bold(true)
	keyStyle := helpKeyStyle.Background(modalBg)
	descStyle := helpDescStyle.Background(modalBg)
	sectionStyle := lipgloss.NewStyle().Foreground(colorSecondary).Bold(true).Background(modalBg)
	divStyle := lipgloss.NewStyle().Foreground(colorSubtle).Background(modalBg)

	sep := divStyle.Render("  ")
	entrySep := divStyle.Render("   ")

	row := func(key, desc string) string {
		return keyStyle.Render(key) + sp.Render(" ") + descStyle.Render(desc)
	}

	div := divStyle.Render(strings.Repeat("─", 52))

	var b strings.Builder

	b.WriteString(titleStyle.Render("Keyboard Reference") + "\n")
	b.WriteString(div + "\n\n")

	// Dashboard section
	b.WriteString(sectionStyle.Render("DASHBOARD") + "\n")
	b.WriteString(row("enter", "connect") + sep + row("n", "new host") + sep + row("e", "edit") + "\n")
	b.WriteString(row("c", "duplicate") + sep + row("d/d", "delete") + sep + row("p", "pin/unpin") + "\n")
	b.WriteString(row("space/→", "expand") + sep + row("←", "collapse") + sep + row("ctrl+d", "force scan") + "\n")
	b.WriteString(row("/", "filter") + sep + row("h", "history") + sep + row("i", "import SSH config") + "\n")
	b.WriteString(row("K", "staged key rotation") + "\n")
	b.WriteString(row("g", "new group") + sep + row("r", "rename group") + sep + row("⇧↑↓", "reorder") + "\n")
	b.WriteString(row("a", "about") + sep + row("?", "help") + sep + row("q", "quit") + "\n")
	b.WriteString("\n")

	// Form section
	b.WriteString(sectionStyle.Render("FORM (add / edit)") + "\n")
	b.WriteString(row("tab/↓", "next field") + entrySep + row("⇧tab/↑", "prev field") + "\n")
	b.WriteString(row("enter", "advance / activate") + entrySep + row("←→", "cycle group") + "\n")
	b.WriteString(row("ctrl+s", "save") + entrySep + row("ctrl+t", "test connection") + entrySep + row("esc", "cancel") + "\n")
	b.WriteString(row("ctrl+k", "install public key (edit mode)") + "\n")
	b.WriteString("\n")

	// History section
	b.WriteString(sectionStyle.Render("HISTORY") + "\n")
	b.WriteString(row("enter", "connect") + sep + row("e", "edit") + sep + row("h/esc/q", "back") + "\n")
	b.WriteString("\n")

	// Field reference (for narrow terminals where the sidebar isn't shown)
	b.WriteString(sectionStyle.Render("FIELD REFERENCE") + "\n")
	fieldRef := []struct{ name, desc string }{
		{"Key File", "Path to SSH private key (e.g. ~/.ssh/id_rsa)"},
		{"Password", "Stored in OS keychain, not written to disk"},
		{"Fwd. Agent", "Toggle forwarding of local SSH keys to the remote (-A)"},
		{"ProxyJump", "Jump/bastion host: user@host:port — SSH tunnels through it"},
		{"LocalFwd", "Port tunnel: local_port:remote_host:remote_port"},
		{"Group", "Collapsible group; use ← → in form to cycle"},
	}
	for _, f := range fieldRef {
		b.WriteString(keyStyle.Render(fmt.Sprintf("%-12s", f.name)) + sp.Render(" ") + descStyle.Render(f.desc) + "\n")
	}
	b.WriteString("\n")

	b.WriteString(div + "\n")
	b.WriteString(keyStyle.Render("?") + sp.Render(" ") + descStyle.Render("close") + sp.Render("   ") + keyStyle.Render("esc") + sp.Render(" ") + descStyle.Render("close"))

	return lipgloss.NewStyle().
		Border(lipgloss.RoundedBorder()).
		BorderForeground(colorPrimary).
		Padding(1, 3).
		Background(modalBg).
		Render(b.String())
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
		baseLine := out[i]
		// Truncate or pad base line to exactly `left` cells so the modal
		// starts at the centered column regardless of base line width.
		baseLineWidth := lipgloss.Width(baseLine)
		switch {
		case baseLineWidth > left:
			out[i] = ansi.Truncate(baseLine, left, "")
		case baseLineWidth < left:
			out[i] += strings.Repeat(" ", left-baseLineWidth)
		}
		modalLine := modalLines[mi]
		if modalLineWidth := lipgloss.Width(modalLine); modalLineWidth < modalW {
			modalLine += strings.Repeat(" ", modalW-modalLineWidth)
		}
		out[i] += modalLine
		if baseLineWidth > left+modalW {
			out[i] += ansi.Cut(baseLine, left+modalW, baseLineWidth)
		}
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
	width, height := m.width, m.height
	if width <= 0 {
		width = 80
	}
	if height <= 0 {
		height = 24
	}
	if width >= 100 && height >= 28 {
		return m.renderFormModal(width, height)
	}
	return m.renderFormWorkspace(width, height, false)
}

func (m model) renderFormModal(width, height int) string {
	modalWidth := min(96, width-6)
	modalHeight := min(34, height-4)
	innerWidth := max(modalWidth-2, 1)
	innerHeight := max(modalHeight-2, 1)

	workspace := m.renderFormWorkspace(innerWidth, innerHeight, true)
	panel := lipgloss.NewStyle().
		Border(lipgloss.RoundedBorder()).
		BorderForeground(colorPrimary).
		Width(innerWidth).
		Height(innerHeight).
		Render(workspace)

	backdrop := fitViewToBounds(dimBase(m.renderListView()), width, height)
	return fitViewToBounds(overlayCenter(backdrop, panel, width, height), width, height)
}

func (m model) renderFormWorkspace(width, height int, forceWide bool) string {
	if width < 36 || height < 12 {
		return renderFormTooSmall(width, height)
	}

	padX := 1
	if width >= 60 {
		padX = 2
	}
	padY := 0
	if height >= 16 {
		padY = 1
	}
	contentWidth := max(width-padX*2, 1)
	contentHeight := max(height-padY, 4)
	bodyHeight := max(contentHeight-3, 1) // header, status, and footer
	wide := forceWide || width >= 100
	compact := width < 60

	mainWidth := contentWidth
	sidebarWidth := 0
	if wide {
		sidebarWidth = 30
		mainWidth = max(contentWidth-sidebarWidth-2, 1)
	}

	document := m.renderFormDocument(mainWidth, wide, compact)
	formViewport := m.form.viewport
	formViewport.Width = mainWidth
	formViewport.Height = bodyHeight
	formViewport.Style = lipgloss.NewStyle()
	formViewport.SetContent(document)
	m.ensureFormFocusVisible(&formViewport, document)

	body := formViewport.View()
	if wide {
		sidebar := m.renderFormContextRail(sidebarWidth, bodyHeight, formViewport.ScrollPercent())
		body = lipgloss.JoinHorizontal(lipgloss.Top, body, "  ", sidebar)
	}

	content := strings.Join([]string{
		m.renderFormHeader(contentWidth),
		body,
		m.renderFormStatusLine(contentWidth),
		m.renderFormFooter(contentWidth),
	}, "\n")

	return lipgloss.NewStyle().
		PaddingTop(padY).
		PaddingLeft(padX).
		PaddingRight(padX).
		MaxWidth(width).
		MaxHeight(height).
		Render(content)
}

func fitViewToBounds(view string, width, height int) string {
	if width <= 0 || height <= 0 {
		return ""
	}
	lines := strings.Split(view, "\n")
	if len(lines) > height {
		lines = lines[:height]
	}
	for i := range lines {
		lines[i] = ansi.Truncate(lines[i], width, "")
	}
	return strings.Join(lines, "\n")
}

var formFieldHints = [fieldCount]string{
	fieldAlias:        "Friendly name shown in the host list. Connect directly via `assho connect <alias>`.",
	fieldHostname:     "IP address or domain name of the server (e.g. 192.168.1.50 or db.example.com).",
	fieldUser:         "SSH username to log in as (e.g. root, ubuntu, deploy).",
	fieldPort:         "SSH port. Standard is 22 — only change if the server uses a non-default port.",
	fieldKeyFile:      "Path to your SSH private key file (e.g. ~/.ssh/id_rsa). Key-based auth is preferred over passwords.",
	fieldPassword:     "SSH password — stored securely in your OS keychain, not written to the config file.",
	fieldForwardAgent: "SSH agent forwarding (-A) lets the remote server use your local SSH keys, which is useful when hopping through a bastion.",
	fieldProxyJump:    "A bastion or jump host used to reach this server. SSH tunnels through it transparently. Format: user@host:port",
	fieldLocalForward: "Creates a local port tunnel into the remote network. Format: local_port:remote_host:remote_port — e.g. 5432:localhost:5432 to reach a remote database as if it were local.",
	fieldGroup:        "Assign to a collapsible group (prod, staging, homelab…). Use ← → to cycle through existing groups.",
	fieldNotes:        "Free-text note shown beneath the alias in the host list.",
}

func renderFormTooSmall(width, height int) string {
	if width <= 0 || height <= 0 {
		return ""
	}
	lines := []string{
		formSectionStyle.Render("assho · edit host"),
		"Terminal too small",
		formHintStyle.Render("Resize to at least 36 × 12."),
		formHintStyle.Render("Esc cancels · Ctrl+C quits"),
	}
	if len(lines) > height {
		lines = lines[:height]
	}
	for i := range lines {
		lines[i] = ansi.Truncate(lines[i], width, "")
	}
	return strings.Join(lines, "\n")
}

func (m model) renderFormHeader(width int) string {
	title := "NEW SSH HOST"
	if m.form.selectedHost != nil {
		title = "EDIT SSH HOST · " + m.form.selectedHost.Alias
	}
	left := lipgloss.NewStyle().Foreground(colorText).Bold(true).Render(title)
	right := formHintStyle.Render(m.formProgressLabel())
	leftWidth, rightWidth := lipgloss.Width(left), lipgloss.Width(right)
	if leftWidth+rightWidth+1 > width {
		left = ansi.Truncate(left, max(width-rightWidth-1, 1), "")
		leftWidth = lipgloss.Width(left)
	}
	gap := max(width-leftWidth-rightWidth, 1)
	return ansi.Truncate(left+strings.Repeat(" ", gap)+right, width, "")
}

func (m model) formProgressLabel() string {
	last := controlNotes
	if m.form.selectedHost != nil {
		last = controlDelete
	}
	return fmt.Sprintf("%d/%d · %s", int(m.form.focus)+1, int(last)+1, formControlLabel(m.form.focus))
}

func formControlLabel(control formControl) string {
	switch control {
	case controlAlias:
		return "Alias"
	case controlHostname:
		return "Hostname"
	case controlUser:
		return "User"
	case controlPort:
		return "Port"
	case controlKeyFile:
		return "Key file"
	case controlKeyPicker:
		return "Browse key"
	case controlPassword:
		return "Password"
	case controlForwardAgent:
		return "Agent forwarding"
	case controlProxyJump:
		return "ProxyJump"
	case controlLocalForward:
		return "Local forward"
	case controlGroup:
		return "Group"
	case controlNotes:
		return "Notes"
	case controlDelete:
		return "Delete host"
	default:
		return "Form"
	}
}

func (m model) renderFormDocument(width int, twoColumn, compact bool) string {
	type section struct {
		title string
		rows  [][]formControl
	}
	sections := []section{
		{title: "Endpoint", rows: [][]formControl{{controlAlias, controlHostname}, {controlUser, controlPort}}},
		{title: "Authentication", rows: [][]formControl{{controlKeyFile}, {controlPassword, controlForwardAgent}}},
		{title: "Routing", rows: [][]formControl{{controlProxyJump, controlLocalForward}}},
		{title: "Details", rows: [][]formControl{{controlGroup, controlNotes}}},
	}
	var lines []string
	for sectionIndex, item := range sections {
		if sectionIndex > 0 {
			lines = append(lines, "")
		}
		lines = append(lines, renderFormSectionHeading(item.title, width, compact))
		for _, row := range item.rows {
			if !twoColumn && len(row) == 2 {
				for _, control := range row {
					lines = append(lines, strings.Split(m.renderFormControlBlock(control, width, true), "\n")...)
					lines = append(lines, "")
				}
				continue
			}
			if len(row) == 1 {
				lines = append(lines, strings.Split(m.renderFormControlBlock(row[0], width, !twoColumn), "\n")...)
				lines = append(lines, "")
				continue
			}
			gap := 3
			leftWidth := max((width-gap)/2, 1)
			rightWidth := max(width-gap-leftWidth, 1)
			joined := lipgloss.JoinHorizontal(
				lipgloss.Top,
				m.renderFormControlBlock(row[0], leftWidth, false),
				strings.Repeat(" ", gap),
				m.renderFormControlBlock(row[1], rightWidth, false),
			)
			lines = append(lines, strings.Split(joined, "\n")...)
			lines = append(lines, "")
		}
	}
	if m.form.selectedHost != nil && !twoColumn {
		lines = append(lines, renderFormSectionHeading("Danger zone", width, compact))
		lines = append(lines, strings.Split(m.renderFormControlBlock(controlDelete, width, true), "\n")...)
	}
	return strings.TrimRight(strings.Join(lines, "\n"), "\n")
}

func renderFormSectionHeading(title string, width int, compact bool) string {
	label := formSectionStyle.Render(title)
	if compact {
		return label
	}
	ruleWidth := max(width-lipgloss.Width(label)-1, 0)
	return label + " " + formDividerStyle.Render(strings.Repeat("─", ruleWidth))
}

func (m model) renderFormControlBlock(control formControl, width int, inlineHint bool) string {
	label := formControlLabel(control)
	focused := m.form.focus == control
	if control == controlKeyFile {
		focused = m.form.focus == controlKeyFile || m.form.focus == controlKeyPicker
	}
	labelStyle := lipgloss.NewStyle().Foreground(colorMuted)
	if focused {
		labelStyle = lipgloss.NewStyle().Foreground(colorPrimary).Bold(true)
	}
	if control == controlAlias || control == controlHostname {
		label += " *"
	}

	var value string
	switch control {
	case controlKeyFile:
		buttonStyle := lipgloss.NewStyle().Foreground(colorDimText).Background(colorSubtle).Padding(0, 1)
		if m.form.focus == controlKeyPicker {
			buttonStyle = buttonStyle.Foreground(colorText).Background(colorPrimary).Bold(true)
		}
		button := buttonStyle.Render("Browse")
		input := m.form.inputs[fieldKeyFile]
		input.Width = max(width-lipgloss.Width(button)-1, 1)
		value = lipgloss.JoinHorizontal(lipgloss.Top, input.View(), " ", button)
	case controlForwardAgent:
		enabled := forwardAgentEnabled(m.form.inputs[fieldForwardAgent].Value())
		toggle := "○ OFF"
		if enabled {
			toggle = "● ON"
		}
		toggleStyle := lipgloss.NewStyle().Foreground(colorDimText).Background(colorSubtle).Padding(0, 1)
		if focused {
			toggleStyle = toggleStyle.Foreground(colorText).Background(colorPrimary).Bold(true)
		}
		value = toggleStyle.Render(toggle) + " " + formHintStyle.Render("Space or Enter")
	case controlGroup:
		if m.form.groupCustom {
			input := m.form.inputs[fieldGroup]
			input.Width = max(width, 1)
			value = input.View()
		} else {
			groupValue := "(none)"
			if len(m.form.groupOptions) > 0 && m.form.groupIndex >= 0 && m.form.groupIndex < len(m.form.groupOptions) {
				groupValue = m.form.groupOptions[m.form.groupIndex]
			}
			groupValue = ansi.Truncate(groupValue, max(width-4, 1), "…")
			selectorStyle := lipgloss.NewStyle().Foreground(colorDimText)
			if focused {
				selectorStyle = selectorStyle.Foreground(colorText).Bold(true)
			}
			value = selectorStyle.Render("◀ " + groupValue + " ▶")
		}
	case controlDelete:
		text := "Delete host"
		if m.form.deleteArmed {
			text = "Press Enter again to delete"
		}
		buttonStyle := lipgloss.NewStyle().Foreground(colorDimText).Background(colorSubtle).Padding(0, 1)
		if focused {
			buttonStyle = buttonStyle.Foreground(colorText).Background(colorDanger).Bold(true)
		}
		value = buttonStyle.Render(text)
	default:
		if field, ok := fieldForFormControl(control); ok {
			input := m.form.inputs[field]
			input.Width = max(width, 1)
			value = input.View()
		}
	}

	block := labelStyle.Render(label) + "\n" + ansi.Truncate(value, width, "")
	if inlineHint && focused {
		if field, ok := fieldForFormControl(control); ok {
			block += "\n" + lipgloss.NewStyle().Foreground(colorDimText).Italic(true).Width(width).Render(formFieldHints[field])
		} else if control == controlDelete && m.form.deleteArmed {
			block += "\n" + formHintStyle.Render("Esc cancels deletion")
		}
	}
	return lipgloss.NewStyle().Width(width).MaxWidth(width).Render(block)
}

func forwardAgentEnabled(value string) bool {
	switch strings.ToLower(strings.TrimSpace(value)) {
	case "yes", "1", "true", "on":
		return true
	default:
		return false
	}
}

func (m model) ensureFormFocusVisible(formViewport *viewport.Model, document string) {
	needle := formControlLabel(m.form.focus)
	if m.form.focus == controlKeyPicker {
		needle = formControlLabel(controlKeyFile)
	}
	lines := strings.Split(document, "\n")
	lineIndex := -1
	for i, line := range lines {
		if strings.Contains(ansi.Strip(line), needle) {
			lineIndex = i
			break
		}
	}
	if lineIndex == -1 {
		return
	}
	top := max(lineIndex-1, 0)
	bottom := min(lineIndex+3, len(lines))
	if top < formViewport.YOffset {
		formViewport.SetYOffset(top)
	} else if bottom > formViewport.YOffset+formViewport.Height {
		formViewport.SetYOffset(bottom - formViewport.Height)
	}
}

func (m model) renderFormContextRail(width, height int, scrollPercent float64) string {
	innerWidth := max(width-2, 1)
	var b strings.Builder
	b.WriteString(formSectionStyle.Render("Current field") + "\n")
	if field, ok := fieldForFormControl(m.form.focus); ok {
		b.WriteString(lipgloss.NewStyle().Foreground(colorDimText).Italic(true).Width(innerWidth).Render(formFieldHints[field]))
	} else {
		b.WriteString(formHintStyle.Render("Press Enter twice to confirm deletion."))
	}
	b.WriteString("\n\n")
	b.WriteString(formSectionStyle.Render("Actions") + "\n")
	b.WriteString(helpEntry("Ctrl+S", "save") + "\n")
	b.WriteString(helpEntry("Ctrl+T", "test connection") + "\n")
	if m.form.selectedHost != nil {
		b.WriteString(helpEntry("Ctrl+K", "install public key") + "\n")
	}
	b.WriteString(helpEntry("Esc", "cancel") + "\n")
	b.WriteString("\n")
	b.WriteString(formHintStyle.Render(fmt.Sprintf("Form position %d%%", int(scrollPercent*100))))
	if m.form.selectedHost != nil {
		b.WriteString("\n\n")
		b.WriteString(formSectionStyle.Render("Danger zone") + "\n")
		b.WriteString(m.renderFormControlBlock(controlDelete, innerWidth, false))
	}
	return lipgloss.NewStyle().
		Border(lipgloss.NormalBorder(), false, false, false, true).
		BorderForeground(colorSubtle).
		PaddingLeft(1).
		Width(width - 1).
		MaxWidth(width - 1).
		Height(height).
		MaxHeight(height).
		Render(b.String())
}

func (m model) renderFormStatusLine(width int) string {
	status := m.renderFormStatus()
	if status == "" {
		status = lipgloss.NewStyle().Foreground(colorMuted).Render("● Ready")
	}
	return ansi.Truncate(status, width, "")
}

func (m model) renderFormFooter(width int) string {
	var footer string
	if width < 58 {
		footer = helpEntry("^S", "save") + "  " + helpEntry("^T", "test")
		if m.form.selectedHost != nil {
			footer += "  " + helpEntry("^K", "install key")
		}
		footer += "  " + helpEntry("esc", "cancel")
	} else {
		sep := helpSepStyle.Render("  ·  ")
		footer = strings.Join([]string{
			helpEntry("ctrl+s", "save"),
			helpEntry("ctrl+t", "test"),
			helpEntry("tab", "next"),
			helpEntry("esc", "cancel"),
			helpEntry("?", "help"),
		}, sep)
		if m.form.selectedHost != nil {
			footer = helpEntry("ctrl+k", "install key") + sep + footer
		}
	}
	return ansi.Truncate(footer, width, "")
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
