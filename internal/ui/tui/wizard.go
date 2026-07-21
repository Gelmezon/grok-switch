package tui

import (
	"fmt"
	"net/url"
	"strings"

	"github.com/charmbracelet/bubbles/textinput"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/Gelmezon/grok-switch/internal/profiles"
	"github.com/Gelmezon/grok-switch/internal/secret"
	"github.com/Gelmezon/grok-switch/internal/ui/theme"
)

// WizardMode is add or edit.
type WizardMode int

const (
	WizardAdd WizardMode = iota
	WizardEdit
)

// WizardModel is a multi-step profile form.
type WizardModel struct {
	mode     WizardMode
	step     int // 0..5 (5 = confirm)
	existing profiles.Profile
	inputs   []textinput.Model
	// steps: name, baseURL, apiKey, defaultModel+effort, advanced (web/explore/plan)
	errMsg   string
	width    int
	focusBtn int // 0 confirm, 1 cancel on confirm step
	onDone   func(profiles.Profile) tea.Cmd
}

// NewAddWizard creates an add profile wizard.
func NewAddWizard(prefillName string, onDone func(profiles.Profile) tea.Cmd) *WizardModel {
	m := &WizardModel{mode: WizardAdd, width: 60, onDone: onDone}
	m.inputs = makeInputs()
	if prefillName != "" {
		m.inputs[0].SetValue(prefillName)
	}
	m.inputs[0].Focus()
	return m
}

// NewEditWizard creates an edit wizard prefilled with profile.
func NewEditWizard(p profiles.Profile, onDone func(profiles.Profile) tea.Cmd) *WizardModel {
	m := &WizardModel{mode: WizardEdit, width: 60, existing: p, onDone: onDone}
	m.inputs = makeInputs()
	m.inputs[0].SetValue(p.Name)
	m.inputs[1].SetValue(p.BaseURL)
	m.inputs[2].SetValue(p.APIKey)
	m.inputs[3].SetValue(p.DefaultModel)
	m.inputs[4].SetValue(p.DefaultReasoningEffort)
	m.inputs[5].SetValue(p.WebSearchModel)
	m.inputs[6].SetValue(p.SubagentsModels.Explore)
	m.inputs[7].SetValue(p.SubagentsModels.Plan)
	m.inputs[0].Focus()
	return m
}

func makeInputs() []textinput.Model {
	labels := 8
	ins := make([]textinput.Model, labels)
	for i := range ins {
		ti := textinput.New()
		ti.CharLimit = 256
		ti.Width = 48
		ins[i] = ti
	}
	ins[2].EchoMode = textinput.EchoPassword
	ins[2].EchoCharacter = '•'
	ins[4].Placeholder = "high"
	return ins
}

func (m *WizardModel) Init() tea.Cmd {
	return textinput.Blink
}

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
			if m.step == 0 {
				return m, func() tea.Msg { return DoneMsg{} }
			}
			if m.step == 5 {
				m.step = 4
				return m, nil
			}
			m.step--
			m.errMsg = ""
			m.focusStep()
			return m, textinput.Blink
		case "tab":
			if m.step == 5 {
				m.focusBtn = 1 - m.focusBtn
				return m, nil
			}
			if m.step == 3 {
				// model <-> effort
				if m.inputs[3].Focused() {
					m.inputs[3].Blur()
					m.inputs[4].Focus()
				} else {
					m.inputs[4].Blur()
					m.inputs[3].Focus()
				}
				return m, textinput.Blink
			}
			if m.step == 4 {
				m.cycleAdvanced()
				return m, textinput.Blink
			}
		case "enter":
			if m.step == 5 {
				if m.focusBtn == 0 {
					p := m.build()
					return m, m.onDone(p)
				}
				return m, func() tea.Msg { return DoneMsg{} }
			}
			if err := m.validateStep(); err != nil {
				m.errMsg = err.Error()
				return m, nil
			}
			m.errMsg = ""
			m.step++
			if m.step < 5 {
				m.focusStep()
				return m, textinput.Blink
			}
			return m, nil
		case "ctrl+u":
			for i := range m.inputs {
				if m.inputs[i].Focused() {
					m.inputs[i].SetValue("")
				}
			}
		}
	}

	if m.step < 5 {
		var cmd tea.Cmd
		for i := range m.inputs {
			if m.inputs[i].Focused() {
				m.inputs[i], cmd = m.inputs[i].Update(msg)
				// Live validation feedback while typing (URL / name).
				m.liveValidate()
				return m, cmd
			}
		}
	}
	return m, nil
}

// liveValidate updates errMsg for the current step without blocking navigation.
func (m *WizardModel) liveValidate() {
	switch m.step {
	case 0:
		name := strings.TrimSpace(m.inputs[0].Value())
		if name == "" {
			m.errMsg = ""
			return
		}
		for _, r := range name {
			ok := (r >= 'a' && r <= 'z') || (r >= 'A' && r <= 'Z') ||
				(r >= '0' && r <= '9') || r == '-' || r == '_'
			if !ok {
				m.errMsg = "只允许字母、数字、连字符、下划线"
				return
			}
		}
		m.errMsg = ""
	case 1:
		u := strings.TrimSpace(m.inputs[1].Value())
		if u == "" {
			m.errMsg = ""
			return
		}
		parsed, err := url.Parse(u)
		if err != nil || (parsed.Scheme != "http" && parsed.Scheme != "https") || parsed.Host == "" {
			m.errMsg = "Base URL 必须是合法的 http:// 或 https:// URL"
			return
		}
		m.errMsg = ""
	case 2:
		if strings.TrimSpace(m.inputs[2].Value()) != "" {
			m.errMsg = ""
		}
	case 3:
		if strings.TrimSpace(m.inputs[3].Value()) != "" {
			m.errMsg = ""
		}
	}
}

func (m *WizardModel) focusStep() {
	for i := range m.inputs {
		m.inputs[i].Blur()
	}
	switch m.step {
	case 0:
		m.inputs[0].Focus()
	case 1:
		m.inputs[1].Focus()
	case 2:
		m.inputs[2].Focus()
	case 3:
		m.inputs[3].Focus()
	case 4:
		m.inputs[5].Focus()
	}
}

func (m *WizardModel) cycleAdvanced() {
	order := []int{5, 6, 7}
	cur := -1
	for i, idx := range order {
		if m.inputs[idx].Focused() {
			cur = i
			break
		}
	}
	next := (cur + 1) % len(order)
	for _, idx := range order {
		m.inputs[idx].Blur()
	}
	m.inputs[order[next]].Focus()
}

func (m *WizardModel) validateStep() error {
	switch m.step {
	case 0:
		name := strings.TrimSpace(m.inputs[0].Value())
		if name == "" {
			return fmt.Errorf("名称不能为空")
		}
		for _, r := range name {
			ok := (r >= 'a' && r <= 'z') || (r >= 'A' && r <= 'Z') ||
				(r >= '0' && r <= '9') || r == '-' || r == '_'
			if !ok {
				return fmt.Errorf("只允许字母、数字、连字符、下划线")
			}
		}
	case 1:
		u := strings.TrimSpace(m.inputs[1].Value())
		parsed, err := url.Parse(u)
		if err != nil || (parsed.Scheme != "http" && parsed.Scheme != "https") || parsed.Host == "" {
			return fmt.Errorf("Base URL 必须是合法的 http:// 或 https:// URL")
		}
	case 2:
		if strings.TrimSpace(m.inputs[2].Value()) == "" {
			return fmt.Errorf("API Key 不能为空")
		}
	case 3:
		if strings.TrimSpace(m.inputs[3].Value()) == "" {
			return fmt.Errorf("默认模型不能为空")
		}
	}
	return nil
}

func (m *WizardModel) build() profiles.Profile {
	p := m.existing
	p.Name = strings.TrimSpace(m.inputs[0].Value())
	p.BaseURL = strings.TrimSpace(m.inputs[1].Value())
	p.APIKey = strings.TrimSpace(m.inputs[2].Value())
	p.DefaultModel = strings.TrimSpace(m.inputs[3].Value())
	effort := strings.TrimSpace(m.inputs[4].Value())
	if effort == "" {
		effort = "high"
	}
	p.DefaultReasoningEffort = effort
	p.WebSearchModel = strings.TrimSpace(m.inputs[5].Value())
	p.SubagentsModels.Explore = strings.TrimSpace(m.inputs[6].Value())
	p.SubagentsModels.Plan = strings.TrimSpace(m.inputs[7].Value())
	profiles.Normalize(&p)
	return p
}

func (m *WizardModel) View() string {
	total := 5
	stepLabel := m.step + 1
	if m.step >= 5 {
		stepLabel = 5
	}
	title := "添加 Profile"
	if m.mode == WizardEdit {
		title = "编辑 Profile"
	}
	header := fmt.Sprintf("%s                                  步骤 %d / %d", title, stepLabel, total)
	var body strings.Builder

	switch m.step {
	case 0:
		body.WriteString("Profile 名称\n")
		body.WriteString(theme.Muted.Render("────────────") + "\n")
		body.WriteString(m.inputs[0].View() + "\n")
		body.WriteString(theme.Muted.Render("（只允许字母、数字、连字符、下划线）") + "\n")
	case 1:
		body.WriteString("Base URL（中间站 API 地址）\n")
		body.WriteString(theme.Muted.Render("──────────────────────────") + "\n")
		body.WriteString(m.inputs[1].View() + "\n")
	case 2:
		body.WriteString("API Key（输入时不显示）\n")
		body.WriteString(theme.Muted.Render("─────────────────────") + "\n")
		body.WriteString(m.inputs[2].View() + "\n")
		body.WriteString(theme.Muted.Render("也可通过环境变量 GROK_SWITCH_API_KEY 传入") + "\n")
	case 3:
		body.WriteString("默认模型\n")
		body.WriteString(theme.Muted.Render("────────") + "\n")
		body.WriteString(m.inputs[3].View() + "\n\n")
		body.WriteString("推理等级 [high]\n")
		body.WriteString(m.inputs[4].View() + "\n")
		body.WriteString(theme.Muted.Render("（回车使用默认值 high）") + "\n")
	case 4:
		def := strings.TrimSpace(m.inputs[3].Value())
		body.WriteString("高级模型设置（可选，回车跳过则使用默认模型）\n")
		body.WriteString(theme.Muted.Render("────────────────────────────────────────────") + "\n")
		body.WriteString(fmt.Sprintf("搜索模型  [%s] %s\n", def, m.inputs[5].View()))
		body.WriteString(fmt.Sprintf("Explore   [%s] %s\n", def, m.inputs[6].View()))
		body.WriteString(fmt.Sprintf("Plan      [%s] %s\n", def, m.inputs[7].View()))
	case 5:
		p := m.build()
		body.WriteString(theme.Success.Render(theme.SymOK+"  确认") + "\n\n")
		body.WriteString(theme.Field("名称", p.Name) + "\n")
		body.WriteString(theme.Field("Base URL", p.BaseURL) + "\n")
		body.WriteString(theme.Field("API Key", secret.MaskSecret(p.APIKey)) + "\n")
		body.WriteString(theme.Field("模型", p.DefaultModel+"  (推理: "+p.DefaultReasoningEffort+")") + "\n\n")
		ok := "[ 确认 ]"
		cancel := "[ 取消 ]"
		if m.focusBtn == 0 {
			ok = theme.Selected.Render(ok)
			cancel = theme.Muted.Render(cancel)
		} else {
			ok = theme.Muted.Render(ok)
			cancel = theme.Selected.Render(cancel)
		}
		body.WriteString(ok + "   " + cancel + "\n")
	}

	if m.errMsg != "" {
		body.WriteString("\n" + theme.Error.Render(theme.SymFail+"  "+m.errMsg) + "\n")
	}

	footer := theme.Help.Render("Enter 下一步  Esc 返回  Ctrl+C 退出")
	if m.step == 5 {
		footer = theme.Help.Render("Enter 确认  Tab 切换按钮  Esc 返回")
	}
	content := theme.Title.Render(header) + "\n\n" + body.String() + "\n" + footer
	return theme.BoxActive.Width(m.width).Render(content)
}
