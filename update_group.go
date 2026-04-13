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
		m.groupPrompt.action = ""
		m.groupPrompt.target = ""
		m.form.formError = ""
		return m, nil
	case "enter":
		name := strings.TrimSpace(m.groupPrompt.input.Value())
		if name == "" {
			m.form.formError = "group name is required"
			return m, nil
		}
		if idx := findGroupByName(m.rawGroups, name); idx != -1 {
			if m.groupPrompt.action == "rename" && m.rawGroups[idx].ID == m.groupPrompt.target {
				// no-op rename to same value
			} else {
				m.form.formError = "group name already exists"
				return m, nil
			}
		}
		if m.groupPrompt.action == "create" {
			snapshot := m.snapshot()
			m.rawGroups = append(m.rawGroups, Group{ID: newGroupID(), Name: name, Expanded: true})
			m.list.SetItems(flattenHosts(m.rawGroups, m.rawHosts))
			if err := m.save(); err != nil {
				m.restoreSnapshot(snapshot)
				m.form.formError = fmt.Sprintf("failed to save group changes: %v", err)
				return m, nil
			}
		} else if m.groupPrompt.action == "rename" {
			snapshot := m.snapshot()
			for i := range m.rawGroups {
				if m.rawGroups[i].ID == m.groupPrompt.target {
					m.rawGroups[i].Name = name
					break
				}
			}
			m.list.SetItems(flattenHosts(m.rawGroups, m.rawHosts))
			if err := m.save(); err != nil {
				m.restoreSnapshot(snapshot)
				m.form.formError = fmt.Sprintf("failed to save group changes: %v", err)
				return m, nil
			}
		}
		m.state = stateList
		m.groupPrompt.action = ""
		m.groupPrompt.target = ""
		m.form.formError = ""
		return m, nil
	default:
		var cmd tea.Cmd
		m.groupPrompt.input, cmd = m.groupPrompt.input.Update(msg)
		return m, cmd
	}
}
