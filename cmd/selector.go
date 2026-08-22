package cmd

import (
	"fmt"
	"strings"

	"yoink/internal/ui"

	tea "github.com/charmbracelet/bubbletea"
)

// selectFromList renders an interactive list and returns the index the user
// picked, or -1 if they cancelled.
func selectFromList(items []string) int {
	m := selectorModel{items: items, choice: -1}
	result, err := tea.NewProgram(m).Run()
	if err != nil {
		return -1
	}
	return result.(selectorModel).choice
}

type selectorModel struct {
	cursor int
	items  []string
	done   bool
	choice int
}

func (m selectorModel) Init() tea.Cmd { return nil }

func (m selectorModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	key, ok := msg.(tea.KeyMsg)
	if !ok {
		return m, nil
	}
	switch key.String() {
	case "ctrl+c", "q", "esc":
		m.done, m.choice = true, -1
		return m, tea.Quit
	case "up", "k":
		if m.cursor > 0 {
			m.cursor--
		}
	case "down", "j":
		if m.cursor < len(m.items)-1 {
			m.cursor++
		}
	case "enter":
		m.done, m.choice = true, m.cursor
		return m, tea.Quit
	}
	return m, nil
}

func (m selectorModel) View() string {
	if m.done {
		return ""
	}
	var b strings.Builder
	for i, item := range m.items {
		marker, label := " ", ui.MutedStyle.Render(item)
		if m.cursor == i {
			marker = ui.PrimaryStyle.Render("●")
			label = ui.PrimaryStyle.Render(item)
		}
		fmt.Fprintf(&b, "  %s %s\n", marker, label)
	}
	b.WriteString("\n")
	b.WriteString(ui.DimStyle.Render("Use ↑/↓ to navigate, Enter to select, q to quit"))
	return b.String()
}
