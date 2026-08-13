package picker

import (
	"github.com/Digital-Shane/treeview"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
)

// model wraps treeview's TUI to give it a clear accept/cancel contract: Enter
// accepts the focused directory, Esc or Ctrl+C cancels. Everything else (arrows
// to move, ←/→ to collapse/expand) is delegated to the inner tree model.
type model struct {
	inner     *treeview.TuiTreeModel[treeview.FileInfo]
	help      string
	cancelled bool
}

func (m *model) Init() tea.Cmd { return m.inner.Init() }

func (m *model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	if key, ok := msg.(tea.KeyMsg); ok {
		switch key.String() {
		case "enter":
			return m, tea.Quit // accept the focused directory
		case "esc", "ctrl+c":
			m.cancelled = true
			return m, tea.Quit
		}
	}
	// The inner model mutates in place, so we keep our own pointer and forward
	// only the command it wants to run.
	_, cmd := m.inner.Update(msg)
	return m, cmd
}

var hintStyle = lipgloss.NewStyle().Faint(true)

// View shows the prompt and the accept/cancel keys on top, then the tree. The
// tree renders its own navigation navbar underneath, so we don't repeat the
// arrow keys here, that avoids two competing help lines.
func (m *model) View() string {
	header := m.help + "\n" + hintStyle.Render("Enter = select this folder · Esc = cancel")
	return header + "\n\n" + m.inner.View()
}
