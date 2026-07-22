package main

import (
	"fmt"
	"strings"

	tea "github.com/charmbracelet/bubbletea"
)

func (m model) updateFilePicker(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch msg.String() {
	case "esc", "q":
		m.returnFromFilePicker(false, "")
		m.form.deleteArmed = false
		return m, nil
	}
	var cmd tea.Cmd
	m.filepicker, cmd = m.filepicker.Update(msg)
	if didSelect, path := m.filepicker.DidSelectFile(msg); didSelect {
		m.returnFromFilePicker(true, path)
		return m, nil
	} else if didSelect, _ := m.filepicker.DidSelectDisabledFile(msg); didSelect {
		m.returnFromFilePicker(false, "")
		return m, nil
	}
	return m, cmd
}

func (m model) updateForm(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch msg.String() {
	case "ctrl+c":
		m.quitting = true
		return m, tea.Quit
	case "?":
		m.helpOpen = true
		return m, nil
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
	case "ctrl+k":
		if m.form.selectedHost != nil {
			return m.openKeyInstall()
		}
		return m, nil
	case "ctrl+s":
		if err := m.saveFromForm(); err != nil {
			m.form.formError = err.Error()
			m.focusFormError(err)
			return m, m.focusInputs()
		}
		m.form.formError = ""
		m.form.deleteArmed = false
		m.state = stateList
		return m, nil
	case "esc":
		if m.form.focus == controlDelete && m.form.deleteArmed {
			m.form.deleteArmed = false
			return m, nil
		}
		m.state = stateList
		m.form.testStatus = ""
		m.form.formError = ""
		m.form.deleteArmed = false
		return m, nil
	case "tab", "down":
		return m.moveFormFocus(1)
	case "shift+tab", "up":
		return m.moveFormFocus(-1)
	case "enter":
		if m.form.focus == controlDelete && m.form.selectedHost != nil {
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
				m.form.deleteArmed = false
				return m, statusClearCmd(m.status.version)
			}
			m.state = stateList
			m.form.deleteArmed = false
			return m, nil
		}
		if m.form.focus == controlKeyPicker {
			m.pickerUse = pickerIdentity
			m.filepicker.AllowedTypes = []string{}
			m.state = stateFilePicker
			return m, m.filepicker.Init()
		}
		if m.form.focus == controlForwardAgent {
			m.toggleForwardAgent()
			return m, nil
		}
		if m.form.focus == controlGroup && !m.form.groupCustom {
			if len(m.form.groupOptions) > 0 && m.form.groupOptions[m.form.groupIndex] == "+ New group..." {
				m.form.groupCustom = true
				m.form.inputs[fieldGroup].SetValue("")
				m.form.inputs[fieldGroup].Placeholder = "new group name"
				return m, m.focusInputs()
			}
		}
		return m.moveFormFocus(1)
	case " ":
		if m.form.focus == controlForwardAgent {
			m.toggleForwardAgent()
			return m, nil
		}
		return m.updateFocusedFormInput(msg)
	case "left":
		if m.form.focus == controlGroup && !m.form.groupCustom {
			if len(m.form.groupOptions) > 0 {
				m.form.groupIndex--
				if m.form.groupIndex < 0 {
					m.form.groupIndex = len(m.form.groupOptions) - 1
				}
				m.applyGroupSelectionToInput()
			}
			return m, nil
		}
		return m.updateFocusedFormInput(msg)
	case "right":
		if m.form.focus == controlGroup && !m.form.groupCustom {
			if len(m.form.groupOptions) > 0 {
				m.form.groupIndex = (m.form.groupIndex + 1) % len(m.form.groupOptions)
				m.applyGroupSelectionToInput()
			}
			return m, nil
		}
		return m.updateFocusedFormInput(msg)
	default:
		if m.form.focus == controlDelete {
			m.form.deleteArmed = false
			return m, nil
		}
		if m.form.focus == controlGroup && !m.form.groupCustom {
			return m, nil
		}
		return m.updateFocusedFormInput(msg)
	}
}

func (m model) updateFocusedFormInput(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	field, ok := fieldForFormControl(m.form.focus)
	if !ok || !m.formControlAcceptsText(m.form.focus) {
		return m, nil
	}
	var cmd tea.Cmd
	m.form.inputs[field], cmd = m.form.inputs[field].Update(msg)
	m.form.formError = ""
	return m, cmd
}

func (m model) moveFormFocus(delta int) (tea.Model, tea.Cmd) {
	last := controlNotes
	if m.form.selectedHost != nil {
		last = controlDelete
	}
	next := int(m.form.focus) + delta
	if next < int(controlAlias) {
		next = int(last)
	}
	if next > int(last) {
		next = int(controlAlias)
	}
	m.form.focus = formControl(next)
	m.form.formError = ""
	m.form.deleteArmed = false
	return m, m.focusInputs()
}

func (m *model) toggleForwardAgent() {
	if forwardAgentEnabled(m.form.inputs[fieldForwardAgent].Value()) {
		m.form.inputs[fieldForwardAgent].SetValue("")
	} else {
		m.form.inputs[fieldForwardAgent].SetValue("yes")
	}
}

func (m *model) focusFormError(err error) {
	message := strings.ToLower(err.Error())
	switch {
	case strings.HasPrefix(message, "alias"):
		m.form.focus = controlAlias
	case strings.HasPrefix(message, "hostname"):
		m.form.focus = controlHostname
	case strings.HasPrefix(message, "port"):
		m.form.focus = controlPort
	case strings.HasPrefix(message, "new group"):
		m.form.focus = controlGroup
	}
}
