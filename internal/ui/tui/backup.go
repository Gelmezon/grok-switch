package tui

import (
	"fmt"
	"os"
	"strings"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/Gelmezon/grok-switch/internal/switcher"
	"github.com/Gelmezon/grok-switch/internal/ui"
	"github.com/Gelmezon/grok-switch/internal/ui/theme"
)

// BackupModel lists backups and supports restore.
type BackupModel struct {
	backupsDir string
	configPath string
	store      interface {
		// restored via app context callback
	}
	list    []switcher.BackupInfo
	cursor  int
	width   int
	height  int
	status  string
	onRestore func(name string) tea.Cmd
	onPrune   func(keep int) tea.Cmd
}

// NewBackupScreen creates backup manager.
func NewBackupScreen(backupsDir, configPath string, onRestore func(string) tea.Cmd, onPrune func(int) tea.Cmd) *BackupModel {
	list, _ := switcher.ListBackups(backupsDir)
	return &BackupModel{
		backupsDir: backupsDir,
		configPath: configPath,
		list:       list,
		width:      72,
		onRestore:  onRestore,
		onPrune:    onPrune,
	}
}

func (m *BackupModel) Init() tea.Cmd { return nil }

func (m *BackupModel) Update(msg tea.Msg) (Screen, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width = min(msg.Width-2, 100)
		m.height = msg.Height
	case tea.KeyMsg:
		switch msg.String() {
		case "esc", "q", "b":
			return m, func() tea.Msg { return DoneMsg{} }
		case "up", "k":
			if m.cursor > 0 {
				m.cursor--
			}
		case "down", "j":
			if m.cursor < len(m.list)-1 {
				m.cursor++
			}
		case "enter", "r":
			if len(m.list) == 0 {
				return m, nil
			}
			name := m.list[m.cursor].Name
			// push confirm via returning a special message handled by App
			return m, func() tea.Msg {
				return backupRestoreRequest{Name: name}
			}
		case "p":
			return m, m.onPrune(10)
		case "R":
			list, _ := switcher.ListBackups(m.backupsDir)
			m.list = list
			if m.cursor >= len(m.list) {
				m.cursor = max(0, len(m.list)-1)
			}
			m.status = "已刷新"
		}
	case InfoMsg:
		m.status = msg.Text
		list, _ := switcher.ListBackups(m.backupsDir)
		m.list = list
	}
	return m, nil
}

type backupRestoreRequest struct{ Name string }

func (m *BackupModel) View() string {
	var b strings.Builder
	b.WriteString(theme.Title.Render("备份管理") + "  " + theme.Muted.Render(m.backupsDir) + "\n\n")
	if len(m.list) == 0 {
		b.WriteString(theme.Muted.Render("（无备份）") + "\n")
	} else {
		for i, bk := range m.list {
			size := int64(0)
			if fi, err := os.Stat(bk.Path); err == nil {
				size = fi.Size()
			}
			line := fmt.Sprintf("%2d  %-44s  %8s  %s",
				i+1, ui.Truncate(bk.Name, 44), ui.HumanSize(size),
				bk.ModTime.Local().Format("2006-01-02 15:04:05"))
			if i == m.cursor {
				line = theme.Selected.Render(theme.SymCursor+" "+line)
			} else if i == 0 {
				line = theme.Bold.Render("  "+line)
			} else {
				line = "  " + line
			}
			b.WriteString(line + "\n")
		}
	}
	if m.status != "" {
		b.WriteString("\n" + theme.Info.Render(theme.SymInfo+"  "+m.status) + "\n")
	}
	b.WriteString("\n" + theme.Help.Render("↑↓ 选择  Enter 恢复  p 清理(keep=10)  R 刷新  Esc 返回"))
	return theme.BoxNormal.Width(m.width).Render(b.String())
}

func max(a, b int) int {
	if a > b {
		return a
	}
	return b
}
