package main

import (
	"github.com/charmbracelet/bubbles/list"
)

func newTestListModel(groups []Group, hosts []Host) list.Model {
	l := list.New(flattenHosts(groups, hosts), hostDelegate{}, 80, 24)
	l.SetShowStatusBar(false)
	l.SetShowTitle(false)
	l.SetShowHelp(false)
	return l
}

func newTestHistoryListModel() list.Model {
	l := list.New([]list.Item{}, hostDelegate{}, 80, 24)
	l.SetShowStatusBar(false)
	l.SetShowTitle(false)
	l.SetShowHelp(false)
	return l
}
