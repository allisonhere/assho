package main

import (
	"fmt"
	"io"
	"strings"

	"github.com/charmbracelet/bubbles/list"
	tea "github.com/charmbracelet/bubbletea"
)

// --- Custom List Delegate ---

type hostDelegate struct{}

func (d hostDelegate) Height() int                             { return 2 }
func (d hostDelegate) Spacing() int                            { return 1 }
func (d hostDelegate) Update(_ tea.Msg, _ *list.Model) tea.Cmd { return nil }

func (d hostDelegate) Render(w io.Writer, m list.Model, index int, listItem list.Item) {
	isSelected := index == m.Index()

	if g, ok := listItem.(groupItem); ok {
		icon := " ▶ "
		if g.Expanded {
			icon = " ▼ "
		}
		title := "📁 " + g.Name
		style := itemNormalTitle
		if isSelected {
			style = itemSelectedTitle
		}
		fmt.Fprintf(w, "%s\n", style.Render(strings.TrimLeft(icon+title, " ")))
		return
	}

	h, ok := listItem.(Host)
	if !ok {
		return
	}

	// Build the icon and title
	var icon, title, desc string
	indent := strings.Repeat("  ", h.ListIndent)

	if h.IsContainer {
		icon = "📦 "
		title = h.Alias
		desc = fmt.Sprintf("container %s", h.Hostname)
	} else {
		if h.Expanded {
			icon = "▼ "
		} else {
			icon = "▶ "
		}

		// Auth indicator
		authIcon := "🌐 " // globe - no specific auth
		if h.IdentityFile != "" {
			authIcon = "🔑 " // key
		} else if h.Password != "" {
			authIcon = "🔒 " // lock
		}

		title = authIcon + h.Alias

		desc = ""
		connStr := fmt.Sprintf("%s@%s", h.User, h.Hostname)
		if h.Port != "" && h.Port != "22" {
			connStr += fmt.Sprintf(":%s", h.Port)
		}
		desc = connStr

		if h.ProxyJump != "" {
			desc += " via " + h.ProxyJump
		}
		if len(h.Containers) > 0 {
			desc += fmt.Sprintf(" [%d containers]", len(h.Containers))
		}
	}

	if isSelected {
		fmt.Fprintf(w, "%s", itemSelectedTitle.Render(indent+icon+title))
		fmt.Fprintf(w, "\n%s", itemSelectedDesc.Render(indent+"  "+desc))
	} else {
		fmt.Fprintf(w, "%s", itemNormalTitle.Render(indent+icon+title))
		fmt.Fprintf(w, "\n%s", itemNormalDesc.Render(indent+"  "+desc))
	}
}
