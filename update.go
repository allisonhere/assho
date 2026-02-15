package main

import (
	"fmt"
	"strings"

	"github.com/charmbracelet/bubbles/list"
	"github.com/charmbracelet/bubbles/spinner"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
)

func (m model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	var cmd tea.Cmd
	var cmds []tea.Cmd

	switch msg := msg.(type) {
	case spinner.TickMsg:
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
		// Modal about overlay intercepts all keys when open
		if m.aboutOpen {
			switch msg.String() {
			case "ctrl+c":
				m.quitting = true
				return m, tea.Quit
			case "a", "esc", "q":
				m.aboutOpen = false
				return m, nil
			}
			return m, nil
		}
		if m.state == stateList {
			if m.list.FilterState() == list.Filtering {
				break // Let the list handle input if filtering
			}
			if m.statusMessage != "" {
				m.statusMessage = ""
				m.statusIsError = false
			}
			if m.listDeleteArmed && msg.String() != "d" && msg.String() != "esc" {
				m.clearListDeleteConfirm()
			}
			switch msg.String() {
			case "ctrl+c":
				m.quitting = true
				return m, tea.Quit
			case "esc":
				if m.listDeleteArmed {
					m.clearListDeleteConfirm()
					return m, nil
				}
			case "q":
				if m.list.FilterState() != list.Filtering {
					m.quitting = true
					return m, tea.Quit
				}
			case "n":
				m.clearListDeleteConfirm()
				m.state = stateForm
				m.selectedHost = nil // New host
				m.inputs = newFormInputs()
				m.resetForm()
				m.buildGroupOptions("")
				m.formError = ""
				m.keyPickFocus = false
				m.deleteFocus = false
				m.deleteArmed = false
				return m, m.focusInputs()
			case "enter", "space":
				switch i := m.list.SelectedItem().(type) {
				case groupItem:
					for idx := range m.rawGroups {
						if m.rawGroups[idx].ID == i.ID {
							m.rawGroups[idx].Expanded = !m.rawGroups[idx].Expanded
							m.list.SetItems(flattenHosts(m.rawGroups, m.rawHosts))
							return m, nil
						}
					}
				case Host:
					if i.IsContainer {
						m.clearListDeleteConfirm()
						snapshot := m.snapshot()
						m.history = recordHistory(i.ID, i.Alias, m.history)
						if err := m.save(); err != nil {
							m.restoreSnapshot(snapshot)
							m.statusMessage = fmt.Sprintf("Failed to save history: %v", err)
							m.statusIsError = true
							return m, nil
						}
						m.sshToRun = &i
						return m, tea.Quit
					}
					if msg.String() == "space" {
						for idx, h := range m.rawHosts {
							if h.ID == i.ID {
								m.rawHosts[idx].Expanded = !m.rawHosts[idx].Expanded
								m.list.SetItems(flattenHosts(m.rawGroups, m.rawHosts))
								return m, nil
							}
						}
					}
					if msg.String() == "enter" {
						m.clearListDeleteConfirm()
						snapshot := m.snapshot()
						m.history = recordHistory(i.ID, i.Alias, m.history)
						if err := m.save(); err != nil {
							m.restoreSnapshot(snapshot)
							m.statusMessage = fmt.Sprintf("Failed to save history: %v", err)
							m.statusIsError = true
							return m, nil
						}
						m.sshToRun = &i
						return m, tea.Quit
					}
				}
			case "right":
				if g, ok := m.list.SelectedItem().(groupItem); ok {
					for idx := range m.rawGroups {
						if m.rawGroups[idx].ID == g.ID {
							if !m.rawGroups[idx].Expanded {
								m.rawGroups[idx].Expanded = true
								m.list.SetItems(flattenHosts(m.rawGroups, m.rawHosts))
							}
							return m, nil
						}
					}
				}
				if i, ok := m.list.SelectedItem().(Host); ok && !i.IsContainer {
					for idx, h := range m.rawHosts {
						if h.ID == i.ID {
							if !h.Expanded {
								m.rawHosts[idx].Expanded = true

								// Auto-scan if empty
								if len(h.Containers) == 0 {
									m.scanning = true
									m.list.SetItems(flattenHosts(m.rawGroups, m.rawHosts))
									return m, scanDockerContainers(m.rawHosts[idx], idx)
								}

								m.list.SetItems(flattenHosts(m.rawGroups, m.rawHosts))
							}
							return m, nil
						}
					}
				}
			case "left":
				if g, ok := m.list.SelectedItem().(groupItem); ok {
					for idx := range m.rawGroups {
						if m.rawGroups[idx].ID == g.ID {
							if m.rawGroups[idx].Expanded {
								m.rawGroups[idx].Expanded = false
								m.list.SetItems(flattenHosts(m.rawGroups, m.rawHosts))
							}
							return m, nil
						}
					}
				}
				if i, ok := m.list.SelectedItem().(Host); ok {
					if !i.IsContainer {
						// Collapse if expanded
						for idx, h := range m.rawHosts {
							if h.ID == i.ID {
								if h.Expanded {
									m.rawHosts[idx].Expanded = false
									m.list.SetItems(flattenHosts(m.rawGroups, m.rawHosts))
								}
								return m, nil
							}
						}
					}
				}
			case "ctrl+d":
				// Scan Docker containers for selected host
				if i, ok := m.list.SelectedItem().(Host); ok && !i.IsContainer {
					idx := findHostIndexByID(m.rawHosts, i.ID)
					if idx != -1 {
						m.scanning = true
						return m, scanDockerContainers(m.rawHosts[idx], idx)
					}
				}
			case "e":
				if i, ok := m.list.SelectedItem().(Host); ok && !i.IsContainer {
					m.clearListDeleteConfirm()
					m.state = stateForm
					m.selectedHost = &i
					m.inputs = newFormInputs()
					m.populateForm(i)
					m.formError = ""
					m.keyPickFocus = false
					m.deleteFocus = false
					m.deleteArmed = false
					return m, m.focusInputs()
				}
			case "d":
				// Delete
				if index := m.list.Index(); index >= 0 && len(m.list.Items()) > 0 {
					if g, ok := m.list.SelectedItem().(groupItem); ok {
						if !m.listDeleteArmed || m.listDeleteID != g.ID || m.listDeleteType != "group" {
							m.listDeleteArmed = true
							m.listDeleteID = g.ID
							m.listDeleteType = "group"
							m.listDeleteLabel = g.Name
							return m, nil
						}
						if err := m.deleteGroupByID(g.ID); err != nil {
							m.statusMessage = fmt.Sprintf("Failed to save group deletion: %v", err)
							m.statusIsError = true
							return m, nil
						}
						m.clearListDeleteConfirm()
						return m, nil
					}
					if i, ok := m.list.SelectedItem().(Host); ok {
						if !m.listDeleteArmed || m.listDeleteID != i.ID || m.listDeleteType != "host" {
							m.listDeleteArmed = true
							m.listDeleteID = i.ID
							m.listDeleteType = "host"
							m.listDeleteLabel = i.Alias
							return m, nil
						}
						snapshot := m.snapshot()
						for idx, h := range m.rawHosts {
							if h.ID == i.ID {
								m.rawHosts = append(m.rawHosts[:idx], m.rawHosts[idx+1:]...)
								break
							}
						}
						m.list.SetItems(flattenHosts(m.rawGroups, m.rawHosts))
						if err := m.save(); err != nil {
							m.restoreSnapshot(snapshot)
							m.statusMessage = fmt.Sprintf("Failed to save host deletion: %v", err)
							m.statusIsError = true
							return m, nil
						}
						m.clearListDeleteConfirm()
					}
				}
			case "i":
				imported, skipped, err := importSSHConfig(m.rawHosts)
				if err != nil {
					m.statusMessage = err.Error()
					m.statusIsError = true
					return m, nil
				}
				snapshot := m.snapshot()
				m.rawHosts = append(m.rawHosts, imported...)
				m.list.SetItems(flattenHosts(m.rawGroups, m.rawHosts))
				if err := m.save(); err != nil {
					m.restoreSnapshot(snapshot)
					m.statusMessage = fmt.Sprintf("Imported %d hosts but failed to save: %v", len(imported), err)
					m.statusIsError = true
					return m, nil
				}
				m.statusMessage = fmt.Sprintf("Imported %d hosts (%d skipped)", len(imported), skipped)
				m.statusIsError = false
				return m, nil
			case "h":
				m.rebuildHistoryList()
				m.state = stateHistory
				return m, nil
			case "a":
				m.aboutOpen = true
				m.aboutFrame = 0
				return m, aboutTick()
			case "g":
				m.openGroupPrompt("create", "", "")
				return m, nil
			case "r":
				if g, ok := m.list.SelectedItem().(groupItem); ok {
					m.openGroupPrompt("rename", g.ID, g.Name)
					return m, nil
				}
			case "x":
				if g, ok := m.list.SelectedItem().(groupItem); ok {
					if err := m.deleteGroupByID(g.ID); err != nil {
						m.statusMessage = fmt.Sprintf("Failed to save group deletion: %v", err)
						m.statusIsError = true
					}
					return m, nil
				}
			}
		} else if m.state == stateFilePicker {
			switch msg.String() {
			case "esc", "q":
				m.state = stateForm
				m.keyPickFocus = false
				m.deleteFocus = false
				m.deleteArmed = false
				return m, m.focusInputs()
			}
		} else if m.state == stateForm {
			switch msg.String() {
			case "ctrl+c":
				m.quitting = true
				return m, tea.Quit
			case "ctrl+t":
				h := Host{
					Hostname:     m.inputs[1].Value(),
					User:         m.inputs[2].Value(),
					Port:         m.inputs[3].Value(),
					IdentityFile: m.inputs[4].Value(),
					Password:     m.inputs[5].Value(),
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
				isTab := msg.String() == "tab"
				if m.deleteFocus {
					m.deleteArmed = false
					return m, nil
				}
				if m.focusIndex == 6 && !m.groupCustom && !isTab {
					if len(m.groupOptions) > 0 {
						m.groupIndex = (m.groupIndex + 1) % len(m.groupOptions)
						m.applyGroupSelectionToInput()
					}
					return m, nil
				}
				if m.focusIndex == 4 && !m.keyPickFocus {
					m.keyPickFocus = true
					return m, nil
				}
				if m.focusIndex == 4 && m.keyPickFocus {
					m.keyPickFocus = false
					m.focusIndex = 5
					return m, m.focusInputs()
				}
				m.focusIndex++
				if m.focusIndex >= len(m.inputs) {
					if m.selectedHost != nil && isTab {
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
				isShiftTab := msg.String() == "shift+tab"
				if m.deleteFocus {
					m.deleteFocus = false
					m.deleteArmed = false
					m.focusIndex = len(m.inputs) - 1
					return m, m.focusInputs()
				}
				if m.focusIndex == 6 && !m.groupCustom && !isShiftTab {
					if len(m.groupOptions) > 0 {
						m.groupIndex--
						if m.groupIndex < 0 {
							m.groupIndex = len(m.groupOptions) - 1
						}
						m.applyGroupSelectionToInput()
					}
					return m, nil
				}
				if m.focusIndex == 5 {
					m.focusIndex = 4
					m.keyPickFocus = true
					return m, nil
				}
				if m.focusIndex == 4 && m.keyPickFocus {
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
				if m.focusIndex == 4 && m.keyPickFocus {
					m.state = stateFilePicker
					m.keyPickFocus = false
					return m, m.filepicker.Init()
				}
				if m.focusIndex == 6 && !m.groupCustom {
					if len(m.groupOptions) > 0 && m.groupOptions[m.groupIndex] == "+ New group..." {
						m.groupCustom = true
						m.inputs[6].SetValue("")
						m.inputs[6].Placeholder = "new group name"
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
				if m.focusIndex == 6 && !m.groupCustom {
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
				if m.focusIndex == 6 && !m.groupCustom {
					if len(m.groupOptions) > 0 {
						m.groupIndex = (m.groupIndex + 1) % len(m.groupOptions)
						m.applyGroupSelectionToInput()
					}
					return m, nil
				}
			default:
				if m.focusIndex == 4 && m.keyPickFocus {
					return m, nil
				}
				if m.deleteFocus {
					m.deleteArmed = false
					return m, nil
				}
				if m.focusIndex == 6 && !m.groupCustom {
					return m, nil
				}
				// Forward all other keys (typing, backspace, delete, etc.) to focused input
				if m.focusIndex >= 0 && m.focusIndex < len(m.inputs) {
					m.inputs[m.focusIndex], cmd = m.inputs[m.focusIndex].Update(msg)
					return m, cmd
				}
			}
		} else if m.state == stateGroupPrompt {
			switch msg.String() {
			case "esc":
				m.state = stateList
				m.groupAction = ""
				m.groupTarget = ""
				return m, nil
			case "enter":
				name := strings.TrimSpace(m.groupInput.Value())
				if name == "" {
					m.formError = "group name is required"
					return m, nil
				}
				if idx := findGroupByName(m.rawGroups, name); idx != -1 {
					if m.groupAction == "rename" && m.rawGroups[idx].ID == m.groupTarget {
						// no-op rename to same value
					} else {
						m.formError = "group name already exists"
						return m, nil
					}
				}
				if m.groupAction == "create" {
					snapshot := m.snapshot()
					m.rawGroups = append(m.rawGroups, Group{ID: newHostID(), Name: name, Expanded: true})
					m.list.SetItems(flattenHosts(m.rawGroups, m.rawHosts))
					if err := m.save(); err != nil {
						m.restoreSnapshot(snapshot)
						m.formError = fmt.Sprintf("failed to save group changes: %v", err)
						return m, nil
					}
				} else if m.groupAction == "rename" {
					snapshot := m.snapshot()
					for i := range m.rawGroups {
						if m.rawGroups[i].ID == m.groupTarget {
							m.rawGroups[i].Name = name
							break
						}
					}
					m.list.SetItems(flattenHosts(m.rawGroups, m.rawHosts))
					if err := m.save(); err != nil {
						m.restoreSnapshot(snapshot)
						m.formError = fmt.Sprintf("failed to save group changes: %v", err)
						return m, nil
					}
				}
				m.state = stateList
				m.groupAction = ""
				m.groupTarget = ""
				m.formError = ""
				return m, nil
			default:
				m.groupInput, cmd = m.groupInput.Update(msg)
				return m, cmd
			}
		} else if m.state == stateHistory {
			switch msg.String() {
			case "ctrl+c":
				m.quitting = true
				return m, tea.Quit
			case "h", "esc":
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
			m.historyList, cmd = m.historyList.Update(msg)
			return m, cmd
		}
	}

	if m.state == stateList {
		m.list, cmd = m.list.Update(msg)
		cmds = append(cmds, cmd)
	} else if m.state == stateFilePicker {
		m.filepicker, cmd = m.filepicker.Update(msg)
		cmds = append(cmds, cmd)

		if didSelect, path := m.filepicker.DidSelectFile(msg); didSelect {
			m.inputs[4].SetValue(path)
			m.inputs[4].CursorEnd()
			m.state = stateForm
			m.keyPickFocus = false
			return m, m.focusInputs()
		} else if didSelect, _ := m.filepicker.DidSelectDisabledFile(msg); didSelect {
			m.state = stateForm
			m.keyPickFocus = false
			return m, m.focusInputs()
		}
	} else if m.state == stateForm {
		// Handle non-key messages (cursor blink, etc.) for focused input
		if m.focusIndex >= 0 && m.focusIndex < len(m.inputs) {
			if m.focusIndex == 4 && m.keyPickFocus {
				return m, tea.Batch(cmds...)
			}
			m.inputs[m.focusIndex], cmd = m.inputs[m.focusIndex].Update(msg)
			cmds = append(cmds, cmd)
		}
	} else if m.state == stateGroupPrompt {
		m.groupInput, cmd = m.groupInput.Update(msg)
		cmds = append(cmds, cmd)
	} else if m.state == stateHistory {
		m.historyList, cmd = m.historyList.Update(msg)
		cmds = append(cmds, cmd)
	}

	return m, tea.Batch(cmds...)
}
