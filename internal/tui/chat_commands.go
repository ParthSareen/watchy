package tui

import (
	"fmt"
	"strings"
)

func (m *Model) handleSaveCommand(text string) {
	parts := strings.Fields(text)
	if len(parts) < 2 {
		m.appendChatMessage("agent", "usage: /save <name> [command]\n  /save <name> <command>  save a specific command\n  /save <name>            save the last command the agent started")
		return
	}

	name := parts[1]
	command := ""
	if len(parts) >= 3 {
		command = strings.Join(parts[2:], " ")
	} else {
		command = m.chat.LastStartTaskCommand()
		if command == "" {
			m.appendChatMessage("agent", "no start_task found in chat history")
			return
		}
	}
	if err := m.tickStore.Save(name, command, ""); err != nil {
		m.appendChatMessage("agent", fmt.Sprintf("error: %s", err))
		return
	}
	m.appendChatMessage("agent", fmt.Sprintf("saved tick %q: %s", name, command))
}
