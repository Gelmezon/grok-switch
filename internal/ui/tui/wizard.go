package tui

import (
	"fmt"
	"net/url"
	"strings"

	"github.com/charmbracelet/bubbles/textinput"
	tea "github.com/charmbracelet/bubbletea"

	"github.com/Gelmezon/grok-switch/internal/models"
	"github.com/Gelmezon/grok-switch/internal/profiles"

	"github.com/Gelmezon/grok-switch/internal/ui/theme"
)

// WizardModel is a single-step create/edit wizard with a privacy toggle.
type WizardModel struct {
	mode                  string // "add" or "edit"
	existing              profiles.Profile
	inputs                []textinput.Model
	disableCodebaseUpload bool
	privacyFocused        bool
	errMsg                string
	width                 int
	onDone                func(profiles.Profile) tea.Cmd
}

func NewWizard(mode string, prefill profiles.Profile, onDone func(profiles.Profile) tea.Cmd) *WizardModel {
	m := &WizardModel{
		mode:                  mode,
		width:                 60,
		existing:              prefill,
		onDone:                onDone,
		disableCodebaseUpload: prefill.CodebaseUploadDisabled(),
	}
	m.inputs = makeInputsSingle()
	m.inputs[0].SetValue(prefill.Name)
	m.inputs[1].SetValue(prefill.BaseURL)
	m.inputs[2].SetValue(prefill.APIKey)
	m.inputs[0].Focus()
	return m
}

func makeInputsSingle() []textinput.Model {
	ins := make([]textinput.Model, 3)
	for i := range ins {
		ti := textinput.New()
		ti.CharLimit = 256
		ti.Width = 48
		ins[i] = ti
	}
	ins[2].EchoMode = textinput.EchoPassword
	ins[2].EchoCharacter = '•'
	return ins
}

func (m *WizardModel) Init() tea.Cmd { return textinput.Blink }

func (m *WizardModel) Update(msg tea.Msg) (Screen, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		if msg.Width > 20 {
			m.width = min(msg.Width-4, 72)
		}
	case tea.KeyMsg:
		switch msg.String() {
		case "ctrl+c":
			return m, func() tea.Msg { return QuitMsg{} }
		case "esc":
			return m, func() tea.Msg { return DoneMsg{} }
		case "tab":
			if m.privacyFocused {
				m.privacyFocused = false
				m.inputs[0].Focus()
				return m, textinput.Blink
			}
			for i := range m.inputs {
				if m.inputs[i].Focused() {
					m.inputs[i].Blur()
					if i == len(m.inputs)-1 {
						m.privacyFocused = true
						return m, nil
					}
					m.inputs[i+1].Focus()
					return m, textinput.Blink
				}
			}
		case " ", "left", "right":
			if m.privacyFocused {
				m.disableCodebaseUpload = !m.disableCodebaseUpload
				return m, nil
			}
		case "enter":
			if err := m.validate(); err != nil {
				m.errMsg = err.Error()
				return m, nil
			}
			m.errMsg = ""
			return m, m.onDone(m.build())
		case "ctrl+u":
			for i := range m.inputs {
				if m.inputs[i].Focused() {
					m.inputs[i].SetValue("")
				}
			}
		}
	}

	var cmd tea.Cmd
	for i := range m.inputs {
		if m.inputs[i].Focused() {
			m.inputs[i], cmd = m.inputs[i].Update(msg)
			return m, cmd
		}
	}
	return m, nil
}

func (m *WizardModel) validate() error {
	// Validate all fields
	for i, input := range m.inputs {
		val := strings.TrimSpace(input.Value())
		if i == 0 {
			// Name
			if val == "" {
				m.errMsg = "供应商名称不能为空"
				return fmt.Errorf("供应商名称不能为空")
			}
			for _, r := range val {
				ok := (r >= 'a' && r <= 'z') || (r >= 'A' && r <= 'Z') ||
					(r >= '0' && r <= '9') || r == '-' || r == '_'
				if !ok {
					m.errMsg = "只允许字母、数字、连字符、下划线"
					return fmt.Errorf("只允许字母、数字、连字符、下划线")
				}
			}
			m.errMsg = ""
		} else if i == 1 {
			// Base URL
			if val == "" {
				m.errMsg = "Base URL 不能为空"
				return fmt.Errorf("Base URL 不能为空")
			}
			parsed, err := url.Parse(val)
			if err != nil || (parsed.Scheme != "http" && parsed.Scheme != "https") || parsed.Host == "" {
				m.errMsg = "Base URL 必须是合法的 http:// 或 https:// URL"
				return fmt.Errorf("Base URL 必须是合法的 http:// 或 https:// URL")
			}
			m.errMsg = ""
		} else if i == 2 {
			// API Key
			if val == "" {
				m.errMsg = "API Key 不能为空"
				return fmt.Errorf("API Key 不能为空")
			}
			m.errMsg = ""
		}
	}
	return nil
}

func (m *WizardModel) build() profiles.Profile {
	p := m.existing
	p.Name = strings.TrimSpace(m.inputs[0].Value())
	p.BaseURL = strings.TrimSpace(m.inputs[1].Value())
	p.APIKey = strings.TrimSpace(m.inputs[2].Value())
	p.SetCodebaseUploadDisabled(m.disableCodebaseUpload)
	// The single-step form intentionally hides model settings. New profiles
	// still need a model to pass store validation and produce a usable config.
	if strings.TrimSpace(p.DefaultModel) == "" {
		p.DefaultModel = models.DefaultModel
	}
	profiles.Normalize(&p)
	return p
}

func (m *WizardModel) View() string {
	title := "添加/编辑供应商"
	if m.mode == "edit" {
		title = "编辑供应商"
	}
	header := fmt.Sprintf("%s  (一步完成：连接信息 + 隐私设置)", title)

	var body strings.Builder
	body.WriteString(theme.Muted.Render("供应商名称\n"))
	body.WriteString(theme.Muted.Render("────────────") + "\n")
	body.WriteString(m.inputs[0].View() + "\n\n")

	body.WriteString(theme.Muted.Render("Base URL（请求地址）\n"))
	body.WriteString(theme.Muted.Render("──────────────────────────") + "\n")
	body.WriteString(m.inputs[1].View() + "\n")
	body.WriteString(theme.Muted.Render("例如 https://relay.example.com/v1") + "\n\n")

	body.WriteString(theme.Muted.Render("API Key（输入时不显示）\n"))
	body.WriteString(theme.Muted.Render("─────────────────────") + "\n")
	body.WriteString(m.inputs[2].View() + "\n")
	body.WriteString(theme.Muted.Render("也可通过环境变量 GROK_SWITCH_API_KEY 传入") + "\n\n")

	privacy := "[ ] 禁止上传源代码"
	privacyHint := "已关闭：允许 Grok 使用默认遥测与代码索引行为"
	if m.disableCodebaseUpload {
		privacy = "[✓] 禁止上传源代码"
		privacyHint = "已开启（默认）：关闭遥测、Trace 上传和代码库索引"
	}
	if m.privacyFocused {
		privacy = theme.Selected.Render(privacy + "  ← Space 切换")
	}
	body.WriteString(theme.Muted.Render("隐私保护\n"))
	body.WriteString(theme.Muted.Render("────────") + "\n")
	body.WriteString(privacy + "\n")
	body.WriteString(theme.Muted.Render(privacyHint) + "\n")

	if m.errMsg != "" {
		body.WriteString("\n" + theme.Error.Render(theme.SymFail+"  "+m.errMsg) + "\n")
	}

	footer := theme.Help.Render("Tab 切换字段  Space 开关隐私  Enter 保存  Esc 返回")
	content := theme.Title.Render(header) + "\n\n" + body.String() + "\n" + footer
	return theme.BoxActive.Width(m.width).Render(content)
}
