package main

import (
	"fmt"

	tea "github.com/charmbracelet/bubbletea"
)

func (m model) updateHistory(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch msg.String() {
	case "ctrl+c":
		m.quitting = true
		return m, tea.Quit
	case "h", "esc", "q":
		m.state = stateList
		return m, nil
	case "enter":
		if i, ok := m.historyList.SelectedItem().(Host); ok {
			if i.Hostname == "" {
				// Deleted host — cannot connect
				return m, nil
			}
			snapshot := m.snapshot()
			m.history = recordHistory(i.ID, i.Alias, m.history)
			if err := m.save(); err != nil {
				m.restoreSnapshot(snapshot)
				m.state = stateList
				m.statusMessage = fmt.Sprintf("Failed to save history: %v", err)
				m.statusIsError = true
				return m, nil
			}
			m.sshToRun = &i
			return m, tea.Quit
		}
	case "e":
		if i, ok := m.historyList.SelectedItem().(Host); ok {
			idx := findHostIndexByID(m.rawHosts, i.ID)
			if idx != -1 {
				m.state = stateForm
				h := m.rawHosts[idx]
				m.selectedHost = &h
				m.inputs = newFormInputs()
				m.populateForm(h)
				m.formError = ""
				m.keyPickFocus = false
				m.deleteFocus = false
				m.deleteArmed = false
				return m, m.focusInputs()
			}
		}
	}
	var cmd tea.Cmd
	m.historyList, cmd = m.historyList.Update(msg)
	return m, cmd
}
