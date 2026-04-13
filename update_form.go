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
		m.form.keyPickFocus = false
		m.form.deleteFocus = false
		m.form.deleteArmed = false
		return m, m.focusInputs()
	}
	var cmd tea.Cmd
	m.filepicker, cmd = m.filepicker.Update(msg)
	if didSelect, path := m.filepicker.DidSelectFile(msg); didSelect {
		m.form.inputs[fieldKeyFile].SetValue(path)
		m.form.inputs[fieldKeyFile].CursorEnd()
		m.state = stateForm
		m.form.keyPickFocus = false
		return m, m.focusInputs()
	} else if didSelect, _ := m.filepicker.DidSelectDisabledFile(msg); didSelect {
		m.state = stateForm
		m.form.keyPickFocus = false
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
			Hostname:     m.form.inputs[fieldHostname].Value(),
			User:         m.form.inputs[fieldUser].Value(),
			Port:         m.form.inputs[fieldPort].Value(),
			ProxyJump:    m.form.inputs[fieldProxyJump].Value(),
			IdentityFile: m.form.inputs[fieldKeyFile].Value(),
			Password:     m.form.inputs[fieldPassword].Value(),
		}
		m.form.testStatus = ""
		m.form.testing = true
		return m, testConnection(h)
	case "esc":
		if m.form.deleteFocus && m.form.deleteArmed {
			m.form.deleteArmed = false
			return m, nil
		}
		m.state = stateList
		m.form.testStatus = ""
		m.form.formError = ""
		m.form.keyPickFocus = false
		m.form.deleteFocus = false
		m.form.deleteArmed = false
		return m, nil
	case "tab", "down":
		if m.form.deleteFocus {
			m.form.deleteArmed = false
			return m, nil
		}
		if m.form.focusIndex == fieldKeyFile && !m.form.keyPickFocus {
			m.form.keyPickFocus = true
			return m, nil
		}
		if m.form.focusIndex == fieldKeyFile && m.form.keyPickFocus {
			m.form.keyPickFocus = false
			m.form.focusIndex = fieldNotes
			return m, m.focusInputs()
		}
		m.form.focusIndex++
		if m.form.focusIndex >= len(m.form.inputs) {
			if m.form.selectedHost != nil {
				m.form.focusIndex = len(m.form.inputs) - 1
				m.form.deleteFocus = true
				m.form.deleteArmed = false
				for i := range m.form.inputs {
					m.form.inputs[i].Blur()
					m.form.inputs[i].PromptStyle = lipgloss.NewStyle().Foreground(colorMuted)
					m.form.inputs[i].TextStyle = lipgloss.NewStyle().Foreground(colorText)
				}
				return m, nil
			}
			m.form.focusIndex = 0
		}
		m.form.keyPickFocus = false
		m.form.deleteFocus = false
		m.form.deleteArmed = false
		return m, m.focusInputs()
	case "shift+tab", "up":
		if m.form.deleteFocus {
			m.form.deleteFocus = false
			m.form.deleteArmed = false
			m.form.focusIndex = len(m.form.inputs) - 1
			return m, m.focusInputs()
		}
		if m.form.focusIndex == fieldNotes {
			m.form.focusIndex = fieldKeyFile
			m.form.keyPickFocus = true
			return m, nil
		}
		if m.form.focusIndex == fieldKeyFile && m.form.keyPickFocus {
			m.form.keyPickFocus = false
			return m, m.focusInputs()
		}
		m.form.focusIndex--
		if m.form.focusIndex < 0 {
			m.form.focusIndex = len(m.form.inputs) - 1
		}
		m.form.keyPickFocus = false
		m.form.deleteFocus = false
		m.form.deleteArmed = false
		return m, m.focusInputs()
	case "enter":
		if m.form.deleteFocus && m.form.selectedHost != nil {
			if !m.form.deleteArmed {
				m.form.deleteArmed = true
				return m, nil
			}
			snapshot := m.snapshot()
			for idx, h := range m.rawHosts {
				if h.ID == m.form.selectedHost.ID {
					m.rawHosts = append(m.rawHosts[:idx], m.rawHosts[idx+1:]...)
					break
				}
			}
			m.list.SetItems(flattenHosts(m.rawGroups, m.rawHosts))
			if err := m.save(); err != nil {
				m.restoreSnapshot(snapshot)
				m.state = stateList
				m.status.message = fmt.Sprintf("Failed to save host deletion: %v", err)
				m.status.isError = true
				m.status.version++
				m.form.deleteFocus = false
				m.form.deleteArmed = false
				return m, statusClearCmd(m.status.version)
			}
			m.state = stateList
			m.form.deleteFocus = false
			m.form.deleteArmed = false
			return m, nil
		}
		if m.form.focusIndex == fieldKeyFile && m.form.keyPickFocus {
			m.state = stateFilePicker
			m.form.keyPickFocus = false
			return m, m.filepicker.Init()
		}
		if m.form.focusIndex == fieldGroup && !m.form.groupCustom {
			if len(m.form.groupOptions) > 0 && m.form.groupOptions[m.form.groupIndex] == "+ New group..." {
				m.form.groupCustom = true
				m.form.inputs[fieldGroup].SetValue("")
				m.form.inputs[fieldGroup].Placeholder = "new group name"
				return m, m.focusInputs()
			}
		}
		if m.form.focusIndex == len(m.form.inputs)-1 {
			if err := m.saveFromForm(); err != nil {
				m.form.formError = err.Error()
				return m, nil
			}
			m.form.formError = ""
			m.form.keyPickFocus = false
			m.form.deleteFocus = false
			m.form.deleteArmed = false
			m.state = stateList
			return m, nil
		}
		m.form.focusIndex++
		m.form.formError = ""
		m.form.keyPickFocus = false
		m.form.deleteFocus = false
		m.form.deleteArmed = false
		return m, m.focusInputs()
	case "left":
		if m.form.focusIndex == fieldGroup && !m.form.groupCustom {
			if len(m.form.groupOptions) > 0 {
				m.form.groupIndex--
				if m.form.groupIndex < 0 {
					m.form.groupIndex = len(m.form.groupOptions) - 1
				}
				m.applyGroupSelectionToInput()
			}
			return m, nil
		}
	case "right":
		if m.form.focusIndex == fieldGroup && !m.form.groupCustom {
			if len(m.form.groupOptions) > 0 {
				m.form.groupIndex = (m.form.groupIndex + 1) % len(m.form.groupOptions)
				m.applyGroupSelectionToInput()
			}
			return m, nil
		}
	default:
		if m.form.focusIndex == fieldKeyFile && m.form.keyPickFocus {
			return m, nil
		}
		if m.form.deleteFocus {
			m.form.deleteArmed = false
			return m, nil
		}
		if m.form.focusIndex == fieldGroup && !m.form.groupCustom {
			return m, nil
		}
		if m.form.focusIndex >= 0 && m.form.focusIndex < len(m.form.inputs) {
			var cmd tea.Cmd
			m.form.inputs[m.form.focusIndex], cmd = m.form.inputs[m.form.focusIndex].Update(msg)
			return m, cmd
		}
	}
	return m, nil
}
