package main

import (
	"fmt"

	"github.com/charmbracelet/bubbles/list"
	tea "github.com/charmbracelet/bubbletea"
)

func (m model) updateList(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	if m.list.FilterState() == list.Filtering {
		var cmd tea.Cmd
		m.list, cmd = m.list.Update(msg)
		// Filter cancelled — restore actual expansion state.
		if m.list.FilterState() == list.Unfiltered {
			m.list.SetItems(flattenHosts(m.rawGroups, m.rawHosts))
		}
		return m, cmd
	}
	if m.statusMessage != "" {
		m.statusMessage = ""
		m.statusIsError = false
	}
	if m.listDeleteArmed && msg.String() != "d" && msg.String() != "x" && msg.String() != "esc" {
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
		m.quitting = true
		return m, tea.Quit
	case "n":
		m.clearListDeleteConfirm()
		m.state = stateForm
		m.selectedHost = nil
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
		if i, ok := m.list.SelectedItem().(Host); ok && !i.IsContainer {
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
	case "ctrl+d":
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
	case "c":
		if i, ok := m.list.SelectedItem().(Host); ok && !i.IsContainer {
			m.clearListDeleteConfirm()
			clone := i
			clone.Alias = "Copy of " + i.Alias
			clone.Containers = nil
			clone.Expanded = false
			m.state = stateForm
			m.selectedHost = nil
			m.inputs = newFormInputs()
			m.populateForm(clone)
			m.formError = ""
			m.keyPickFocus = false
			m.deleteFocus = false
			m.deleteArmed = false
			return m, m.focusInputs()
		}
	case "d":
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
	case "ctrl+e":
		exported, skipped, err := exportSSHConfig(m.rawHosts)
		if err != nil {
			m.statusMessage = err.Error()
			m.statusIsError = true
			return m, nil
		}
		m.statusMessage = fmt.Sprintf("Exported %d hosts to ~/.ssh/config (%d skipped)", exported, skipped)
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
	case "shift+up":
		if msg := m.moveItem(-1); msg != "" {
			m.statusMessage = msg
			m.statusIsError = true
		}
		return m, nil
	case "shift+down":
		if msg := m.moveItem(+1); msg != "" {
			m.statusMessage = msg
			m.statusIsError = true
		}
		return m, nil
	case "x":
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
	}
	// Unhandled key — forward to the list widget (navigation, search, etc.)
	prevFilterState := m.list.FilterState()
	// Entering filter mode: pre-load all hosts so collapsed groups are searchable.
	if prevFilterState == list.Unfiltered && msg.String() == "/" {
		m.list.SetItems(flattenAll(m.rawGroups, m.rawHosts))
	}
	var cmd tea.Cmd
	m.list, cmd = m.list.Update(msg)
	// Filter cleared from FilterApplied state — restore actual expansion.
	if prevFilterState != list.Unfiltered && m.list.FilterState() == list.Unfiltered {
		m.list.SetItems(flattenHosts(m.rawGroups, m.rawHosts))
	}
	return m, cmd
}
