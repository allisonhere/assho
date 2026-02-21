package main

import (
	"fmt"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
)

func (m model) updateFilePicker(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch msg.String() {
	case "esc", "q":
		m.state = stateForm
		m.keyPickFocus = false
		m.deleteFocus = false
		m.deleteArmed = false
		return m, m.focusInputs()
	}
	var cmd tea.Cmd
	m.filepicker, cmd = m.filepicker.Update(msg)
	if didSelect, path := m.filepicker.DidSelectFile(msg); didSelect {
		m.inputs[fieldKeyFile].SetValue(path)
		m.inputs[fieldKeyFile].CursorEnd()
		m.state = stateForm
		m.keyPickFocus = false
		return m, m.focusInputs()
	} else if didSelect, _ := m.filepicker.DidSelectDisabledFile(msg); didSelect {
		m.state = stateForm
		m.keyPickFocus = false
		return m, m.focusInputs()
	}
	return m, cmd
}

func (m model) updateForm(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch msg.String() {
	case "ctrl+c":
		m.quitting = true
		return m, tea.Quit
	case "ctrl+t":
		h := Host{
			Hostname:     m.inputs[1].Value(),
			User:         m.inputs[2].Value(),
			Port:         m.inputs[3].Value(),
			ProxyJump:    m.inputs[4].Value(),
			IdentityFile: m.inputs[5].Value(),
			Password:     m.inputs[6].Value(),
		}
		m.testStatus = ""
		m.testing = true
		return m, testConnection(h)
	case "esc":
		if m.deleteFocus && m.deleteArmed {
			m.deleteArmed = false
			return m, nil
		}
		m.state = stateList
		m.testStatus = ""
		m.formError = ""
		m.keyPickFocus = false
		m.deleteFocus = false
		m.deleteArmed = false
		return m, nil
	case "tab", "down":
		if m.deleteFocus {
			m.deleteArmed = false
			return m, nil
		}
		if m.focusIndex == fieldKeyFile && !m.keyPickFocus {
			m.keyPickFocus = true
			return m, nil
		}
		if m.focusIndex == fieldKeyFile && m.keyPickFocus {
			m.keyPickFocus = false
			m.focusIndex = fieldPassword
			return m, m.focusInputs()
		}
		m.focusIndex++
		if m.focusIndex >= len(m.inputs) {
			if m.selectedHost != nil {
				m.focusIndex = len(m.inputs) - 1
				m.deleteFocus = true
				m.deleteArmed = false
				for i := range m.inputs {
					m.inputs[i].Blur()
					m.inputs[i].PromptStyle = lipgloss.NewStyle().Foreground(colorMuted)
					m.inputs[i].TextStyle = lipgloss.NewStyle().Foreground(colorText)
				}
				return m, nil
			}
			m.focusIndex = 0
		}
		m.keyPickFocus = false
		m.deleteFocus = false
		m.deleteArmed = false
		return m, m.focusInputs()
	case "shift+tab", "up":
		if m.deleteFocus {
			m.deleteFocus = false
			m.deleteArmed = false
			m.focusIndex = len(m.inputs) - 1
			return m, m.focusInputs()
		}
		if m.focusIndex == fieldPassword {
			m.focusIndex = fieldKeyFile
			m.keyPickFocus = true
			return m, nil
		}
		if m.focusIndex == fieldKeyFile && m.keyPickFocus {
			m.keyPickFocus = false
			return m, m.focusInputs()
		}
		m.focusIndex--
		if m.focusIndex < 0 {
			m.focusIndex = len(m.inputs) - 1
		}
		m.keyPickFocus = false
		m.deleteFocus = false
		m.deleteArmed = false
		return m, m.focusInputs()
	case "enter":
		if m.deleteFocus && m.selectedHost != nil {
			if !m.deleteArmed {
				m.deleteArmed = true
				return m, nil
			}
			snapshot := m.snapshot()
			for idx, h := range m.rawHosts {
				if h.ID == m.selectedHost.ID {
					m.rawHosts = append(m.rawHosts[:idx], m.rawHosts[idx+1:]...)
					break
				}
			}
			m.list.SetItems(flattenHosts(m.rawGroups, m.rawHosts))
			if err := m.save(); err != nil {
				m.restoreSnapshot(snapshot)
				m.state = stateList
				m.statusMessage = fmt.Sprintf("Failed to save host deletion: %v", err)
				m.statusIsError = true
				m.deleteFocus = false
				m.deleteArmed = false
				return m, nil
			}
			m.state = stateList
			m.deleteFocus = false
			m.deleteArmed = false
			return m, nil
		}
		if m.focusIndex == fieldKeyFile && m.keyPickFocus {
			m.state = stateFilePicker
			m.keyPickFocus = false
			return m, m.filepicker.Init()
		}
		if m.focusIndex == fieldGroup && !m.groupCustom {
			if len(m.groupOptions) > 0 && m.groupOptions[m.groupIndex] == "+ New group..." {
				m.groupCustom = true
				m.inputs[fieldGroup].SetValue("")
				m.inputs[fieldGroup].Placeholder = "new group name"
				return m, m.focusInputs()
			}
		}
		if m.focusIndex == len(m.inputs)-1 {
			if err := m.saveFromForm(); err != nil {
				m.formError = err.Error()
				return m, nil
			}
			m.formError = ""
			m.keyPickFocus = false
			m.deleteFocus = false
			m.deleteArmed = false
			m.state = stateList
			return m, nil
		}
		m.focusIndex++
		m.formError = ""
		m.keyPickFocus = false
		m.deleteFocus = false
		m.deleteArmed = false
		return m, m.focusInputs()
	case "left":
		if m.focusIndex == fieldGroup && !m.groupCustom {
			if len(m.groupOptions) > 0 {
				m.groupIndex--
				if m.groupIndex < 0 {
					m.groupIndex = len(m.groupOptions) - 1
				}
				m.applyGroupSelectionToInput()
			}
			return m, nil
		}
	case "right":
		if m.focusIndex == fieldGroup && !m.groupCustom {
			if len(m.groupOptions) > 0 {
				m.groupIndex = (m.groupIndex + 1) % len(m.groupOptions)
				m.applyGroupSelectionToInput()
			}
			return m, nil
		}
	default:
		if m.focusIndex == fieldKeyFile && m.keyPickFocus {
			return m, nil
		}
		if m.deleteFocus {
			m.deleteArmed = false
			return m, nil
		}
		if m.focusIndex == fieldGroup && !m.groupCustom {
			return m, nil
		}
		if m.focusIndex >= 0 && m.focusIndex < len(m.inputs) {
			var cmd tea.Cmd
			m.inputs[m.focusIndex], cmd = m.inputs[m.focusIndex].Update(msg)
			return m, cmd
		}
	}
	return m, nil
}
