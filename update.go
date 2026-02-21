package main

import (
	"fmt"

	"github.com/charmbracelet/bubbles/spinner"
	tea "github.com/charmbracelet/bubbletea"
)

func (m model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case spinner.TickMsg:
		var cmd tea.Cmd
		m.spinner, cmd = m.spinner.Update(msg)
		return m, cmd
	case aboutTickMsg:
		if m.aboutOpen {
			m.aboutFrame++
			return m, aboutTick()
		}
		return m, nil
	case headerTickMsg:
		if m.state == stateList && !m.aboutOpen {
			m.headerFrame++
		}
		return m, headerTick()
	case testConnectionMsg:
		m.testStatus, m.testResult = formatTestStatus(msg.err)
		m.testing = false
		return m, nil
	case scanDockerMsg:
		m.scanning = false
		if msg.err != nil {
			m.statusMessage = fmt.Sprintf("Scan failed: %v", msg.err)
			m.statusIsError = true
		} else {
			if msg.hostIndex >= 0 && msg.hostIndex < len(m.rawHosts) {
				m.rawHosts[msg.hostIndex].Containers = msg.containers
				m.rawHosts[msg.hostIndex].Expanded = true
				m.list.SetItems(flattenHosts(m.rawGroups, m.rawHosts))
			}
		}
		return m, nil
	case tea.WindowSizeMsg:
		m.width = msg.Width
		m.height = msg.Height
		m.list.SetWidth(msg.Width)
		m.list.SetHeight(msg.Height - 12) // Room for header + help bar
		m.historyList.SetWidth(msg.Width)
		m.historyList.SetHeight(msg.Height - 8)
		m.filepicker.Height = msg.Height - 8
		return m, nil
	case tea.KeyMsg:
		if m.aboutOpen {
			return m.updateAbout(msg)
		}
		switch m.state {
		case stateList:
			return m.updateList(msg)
		case stateFilePicker:
			return m.updateFilePicker(msg)
		case stateForm:
			return m.updateForm(msg)
		case stateGroupPrompt:
			return m.updateGroupPrompt(msg)
		case stateHistory:
			return m.updateHistory(msg)
		}
	}
	// Forward non-key messages to the active sub-component (cursor blink, etc.)
	return m.forwardMsg(msg)
}

func (m model) updateAbout(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch msg.String() {
	case "ctrl+c":
		m.quitting = true
		return m, tea.Quit
	case "a", "esc", "q":
		m.aboutOpen = false
	}
	return m, nil
}

// forwardMsg routes non-key messages to the active sub-component so that
// cursor blink, scroll, and other widget-internal ticks are handled correctly.
func (m model) forwardMsg(msg tea.Msg) (tea.Model, tea.Cmd) {
	var cmd tea.Cmd
	switch m.state {
	case stateList:
		m.list, cmd = m.list.Update(msg)
	case stateFilePicker:
		m.filepicker, cmd = m.filepicker.Update(msg)
	case stateForm:
		if m.focusIndex >= 0 && m.focusIndex < len(m.inputs) {
			if !(m.focusIndex == fieldKeyFile && m.keyPickFocus) {
				m.inputs[m.focusIndex], cmd = m.inputs[m.focusIndex].Update(msg)
			}
		}
	case stateGroupPrompt:
		m.groupInput, cmd = m.groupInput.Update(msg)
	case stateHistory:
		m.historyList, cmd = m.historyList.Update(msg)
	}
	return m, cmd
}
