package main

import (
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
				m.state = stateList
				m.status.message = "Host no longer exists"
				m.status.isError = true
				m.status.version++
				return m, statusClearCmd(m.status.version)
			}
			return m.connectToHost(i)
		}
	case "e":
		if i, ok := m.historyList.SelectedItem().(Host); ok {
			idx := findHostIndexByID(m.rawHosts, i.ID)
			if idx == -1 {
				m.status.message = "Host no longer exists"
				m.status.isError = true
				m.status.version++
				return m, statusClearCmd(m.status.version)
			}
			m.state = stateForm
			h := m.rawHosts[idx]
			m.form.selectedHost = &h
			m.form.inputs = newFormInputs()
			m.populateForm(h)
			return m, m.focusInputs()
		}
	}
	var cmd tea.Cmd
	m.historyList, cmd = m.historyList.Update(msg)
	return m, cmd
}
