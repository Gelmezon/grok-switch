package tui

import (
	"fmt"
	"net/url"
	"strings"

	"github.com/charmbracelet/bubbles/textinput"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/Gelmezon/grok-switch/internal/models"
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

// Wizard steps for ADD (no advanced settings):
//
//	0 name → 1 baseURL → 2 apiKey → 3 model select → 4 confirm
//
// EDIT keeps advanced as an extra step before confirm:
//
//	0 name → 1 baseURL → 2 apiKey → 3 model → 4 advanced → 5 confirm
type WizardModel struct {
	mode       WizardMode
	step       int
	existing   profiles.Profile
	inputs     []textinput.Model // 0 name 1 url 2 key 3 customModel 4 effort-custom 5 web 6 explore 7 plan
	errMsg     string
	width      int
	focusBtn   int // confirm step buttons
	onDone     func(profiles.Profile) tea.Cmd
	// model picker
	modelIdx   int
	effortIdx  int
	customMode bool // free-type model instead of catalog
	// advanced field focus (edit only)
	advFocus int
}

// NewAddWizard creates a create-provider wizard (no advanced step).
func NewAddWizard(prefillName string, onDone func(profiles.Profile) tea.Cmd) *WizardModel {
	m := &WizardModel{
		mode:      WizardAdd,
		width:     64,
		onDone:    onDone,
		modelIdx:  models.IndexOf(models.DefaultModel),
		effortIdx: models.IndexOfEffort(models.DefaultReasoningEffort),
	}
	if m.modelIdx < 0 {
		m.modelIdx = 0
	}
	m.inputs = makeInputs()
	if prefillName != "" {
		m.inputs[0].SetValue(prefillName)
	}
	m.inputs[0].Focus()
	return m
}

// NewEditWizard creates an edit wizard (includes advanced models).
func NewEditWizard(p profiles.Profile, onDone func(profiles.Profile) tea.Cmd) *WizardModel {
	m := &WizardModel{
		mode:      WizardEdit,
		width:     64,
		existing:  p,
		onDone:    onDone,
		modelIdx:  models.IndexOf(p.DefaultModel),
		effortIdx: models.IndexOfEffort(p.DefaultReasoningEffort),
	}
	m.inputs = makeInputs()
	m.inputs[0].SetValue(p.Name)
	m.inputs[1].SetValue(p.BaseURL)
	m.inputs[2].SetValue(p.APIKey)
	if m.modelIdx < 0 {
		m.customMode = true
		m.inputs[3].SetValue(p.DefaultModel)
		m.modelIdx = 0
	}
	m.inputs[5].SetValue(p.WebSearchModel)
	m.inputs[6].SetValue(p.SubagentsModels.Explore)
	m.inputs[7].SetValue(p.SubagentsModels.Plan)
	m.inputs[0].Focus()
	return m
}

func makeInputs() []textinput.Model {
	ins := make([]textinput.Model, 8)
	for i := range ins {
		ti := textinput.New()
		ti.CharLimit = 256
		ti.Width = 48
		ins[i] = ti
	}
	ins[2].EchoMode = textinput.EchoPassword
	ins[2].EchoCharacter = '•'
	ins[3].Placeholder = "自定义模型 ID"
	return ins
}

func (m *WizardModel) confirmStep() int {
	if m.mode == WizardAdd {
		return 4
	}
	return 5
}

func (m *WizardModel) totalSteps() int {
	if m.mode == WizardAdd {
		return 4 // user-facing steps before confirm
	}
	return 5
}

func (m *WizardModel) Init() tea.Cmd { return textinput.Blink }

func (m *WizardModel) Update(msg tea.Msg) (Screen, tea.Cmd) {
	cs := m.confirmStep()
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		if msg.Width > 20 {
			m.width = min(msg.Width-4, 76)
		}
	case tea.KeyMsg:
		key := msg.String()
		switch key {
		case "ctrl+c":
			return m, func() tea.Msg { return QuitMsg{} }
		case "esc":
			if m.step == 0 {
				return m, func() tea.Msg { return DoneMsg{} }
			}
			if m.step == 3 && m.customMode {
				m.customMode = false
				m.errMsg = ""
				return m, nil
			}
			m.step--
			m.errMsg = ""
			m.focusStep()
			return m, textinput.Blink
		case "tab":
			if m.step == cs {
				m.focusBtn = 1 - m.focusBtn
				return m, nil
			}
			if m.step == 3 {
				// cycle effort
				m.effortIdx = (m.effortIdx + 1) % len(models.ReasoningEfforts)
				return m, nil
			}
			if m.mode == WizardEdit && m.step == 4 {
				m.advFocus = (m.advFocus + 1) % 3
				m.focusAdvanced()
				return m, textinput.Blink
			}
		case "up", "k":
			if m.step == 3 && !m.customMode {
				if m.modelIdx > 0 {
					m.modelIdx--
				}
				return m, nil
			}
		case "down", "j":
			if m.step == 3 && !m.customMode {
				if m.modelIdx < len(models.Catalog) { // last = 自定义
					m.modelIdx++
				}
				return m, nil
			}
		case "c":
			if m.step == 3 && !m.customMode {
				m.customMode = true
				m.inputs[3].SetValue("")
				m.inputs[3].Focus()
				return m, textinput.Blink
			}
		case "enter":
			if m.step == cs {
				if m.focusBtn == 0 {
					return m, m.onDone(m.build())
				}
				return m, func() tea.Msg { return DoneMsg{} }
			}
			if m.step == 3 && !m.customMode && m.modelIdx == len(models.Catalog) {
				// selected "自定义"
				m.customMode = true
				m.inputs[3].Focus()
				return m, textinput.Blink
			}
			if err := m.validateStep(); err != nil {
				m.errMsg = err.Error()
				return m, nil
			}
			m.errMsg = ""
			m.step++
			if m.step < cs {
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

	// Text input updates for steps that use them
	if m.step < m.confirmStep() {
		if m.step == 3 && m.customMode {
			var cmd tea.Cmd
			m.inputs[3], cmd = m.inputs[3].Update(msg)
			return m, cmd
		}
		if m.step <= 2 || (m.mode == WizardEdit && m.step == 4) {
			var cmd tea.Cmd
			for i := range m.inputs {
				if m.inputs[i].Focused() {
					m.inputs[i], cmd = m.inputs[i].Update(msg)
					m.liveValidate()
					return m, cmd
				}
			}
		}
	}
	return m, nil
}

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
		if m.customMode {
			m.inputs[3].Focus()
		}
	case 4:
		if m.mode == WizardEdit {
			m.focusAdvanced()
		}
	}
}

func (m *WizardModel) focusAdvanced() {
	for i := range m.inputs {
		m.inputs[i].Blur()
	}
	idx := []int{5, 6, 7}[m.advFocus]
	m.inputs[idx].Focus()
}

func (m *WizardModel) selectedModel() string {
	if m.customMode {
		return strings.TrimSpace(m.inputs[3].Value())
	}
	if m.modelIdx >= 0 && m.modelIdx < len(models.Catalog) {
		return models.Catalog[m.modelIdx]
	}
	return ""
}

func (m *WizardModel) selectedEffort() string {
	if m.effortIdx >= 0 && m.effortIdx < len(models.ReasoningEfforts) {
		return models.ReasoningEfforts[m.effortIdx]
	}
	return models.DefaultReasoningEffort
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
		if m.selectedModel() == "" {
			return fmt.Errorf("请选择或输入模型")
		}
	}
	return nil
}

func (m *WizardModel) build() profiles.Profile {
	p := m.existing
	p.Name = strings.TrimSpace(m.inputs[0].Value())
	p.BaseURL = strings.TrimSpace(m.inputs[1].Value())
	p.APIKey = strings.TrimSpace(m.inputs[2].Value())
	p.DefaultModel = m.selectedModel()
	p.DefaultReasoningEffort = m.selectedEffort()
	// Advanced only from edit (or empty → Normalize fills defaults)
	if m.mode == WizardEdit {
		p.WebSearchModel = strings.TrimSpace(m.inputs[5].Value())
		p.SubagentsModels.Explore = strings.TrimSpace(m.inputs[6].Value())
		p.SubagentsModels.Plan = strings.TrimSpace(m.inputs[7].Value())
	} else {
		// Create: leave advanced empty → Normalize copies default model
		p.WebSearchModel = ""
		p.SubagentsModels = profiles.SubagentsModels{}
	}
	if p.AvailableModels == nil {
		p.AvailableModels = []string{}
	}
	// Remember chosen model in available list
	found := false
	for _, am := range p.AvailableModels {
		if am == p.DefaultModel {
			found = true
			break
		}
	}
	if !found && p.DefaultModel != "" {
		p.AvailableModels = append(p.AvailableModels, p.DefaultModel)
	}
	profiles.Normalize(&p)
	return p
}

func (m *WizardModel) View() string {
	cs := m.confirmStep()
	total := m.totalSteps()
	stepLabel := m.step + 1
	if m.step >= cs {
		stepLabel = total
	} else if stepLabel > total {
		stepLabel = total
	}
	title := "添加供应商"
	if m.mode == WizardEdit {
		title = "编辑供应商"
	}
	header := fmt.Sprintf("%s                              步骤 %d / %d", title, stepLabel, total)
	var body strings.Builder

	switch {
	case m.step == 0:
		body.WriteString("供应商名称\n")
		body.WriteString(theme.Muted.Render("────────────") + "\n")
		body.WriteString(m.inputs[0].View() + "\n")
		body.WriteString(theme.Muted.Render("（只允许字母、数字、连字符、下划线）") + "\n")
	case m.step == 1:
		body.WriteString("Base URL（中间站 API 地址）\n")
		body.WriteString(theme.Muted.Render("──────────────────────────") + "\n")
		body.WriteString(m.inputs[1].View() + "\n")
		body.WriteString(theme.Muted.Render("例如 https://relay.example.com/v1") + "\n")
	case m.step == 2:
		body.WriteString("API Key（输入时不显示）\n")
		body.WriteString(theme.Muted.Render("─────────────────────") + "\n")
		body.WriteString(m.inputs[2].View() + "\n")
		body.WriteString(theme.Muted.Render("也可通过环境变量 GROK_SWITCH_API_KEY 传入") + "\n")
	case m.step == 3:
		body.WriteString("选择默认模型\n")
		body.WriteString(theme.Muted.Render("────────────────") + "\n")
		if m.customMode {
			body.WriteString(m.inputs[3].View() + "\n")
			body.WriteString(theme.Muted.Render("Esc 返回列表选择") + "\n")
		} else {
			for i, name := range models.Catalog {
				line := "  " + name
				if i == m.modelIdx {
					line = theme.Selected.Render(theme.SymCursor + " " + name)
				} else {
					line = theme.Muted.Render(line)
				}
				body.WriteString(line + "\n")
			}
			// custom entry
			customLabel := "  自定义…"
			if m.modelIdx == len(models.Catalog) {
				customLabel = theme.Selected.Render(theme.SymCursor + " 自定义…")
			} else {
				customLabel = theme.Muted.Render(customLabel)
			}
			body.WriteString(customLabel + "\n")
		}
		body.WriteString("\n推理等级: ")
		for i, e := range models.ReasoningEfforts {
			if i == m.effortIdx {
				body.WriteString(theme.Accent.Render("[" + e + "] "))
			} else {
				body.WriteString(theme.Muted.Render(e + " "))
			}
		}
		body.WriteString("\n" + theme.Muted.Render("↑↓ 选模型  Tab 切换推理等级  c 自定义  Enter 下一步") + "\n")
	case m.mode == WizardEdit && m.step == 4:
		def := m.selectedModel()
		body.WriteString("高级模型设置（可选，回车跳过使用默认）\n")
		body.WriteString(theme.Muted.Render("────────────────────────────────") + "\n")
		body.WriteString(fmt.Sprintf("搜索模型  [%s] %s\n", def, m.inputs[5].View()))
		body.WriteString(fmt.Sprintf("Explore   [%s] %s\n", def, m.inputs[6].View()))
		body.WriteString(fmt.Sprintf("Plan      [%s] %s\n", def, m.inputs[7].View()))
		body.WriteString(theme.Muted.Render("Tab 切换字段  Enter 下一步") + "\n")
	case m.step == cs:
		p := m.build()
		body.WriteString(theme.Success.Render(theme.SymOK+"  确认") + "\n\n")
		body.WriteString(theme.Field("名称", p.Name) + "\n")
		body.WriteString(theme.Field("Base URL", p.BaseURL) + "\n")
		body.WriteString(theme.Field("API Key", secret.MaskSecret(p.APIKey)) + "\n")
		body.WriteString(theme.Field("模型", p.DefaultModel+"  (推理: "+p.DefaultReasoningEffort+")") + "\n")
		if m.mode == WizardAdd {
			body.WriteString(theme.Muted.Render("\n高级模型（搜索/Explore/Plan）创建后可在编辑中设置") + "\n")
		}
		body.WriteString("\n")
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
	if m.step == cs {
		footer = theme.Help.Render("Enter 确认  Tab 切换按钮  Esc 返回")
	}
	content := theme.Title.Render(header) + "\n\n" + body.String() + "\n" + footer
	return theme.BoxActive.Width(m.width).Render(content)
}
