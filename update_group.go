package main

import (
	"fmt"
	"strings"

	tea "github.com/charmbracelet/bubbletea"
)

func (m model) updateGroupPrompt(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
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
		var cmd tea.Cmd
		m.groupInput, cmd = m.groupInput.Update(msg)
		return m, cmd
	}
}
