package consoleui

import (
	"fmt"
	"strings"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/charmbracelet/x/ansi"
)

const maxLogEntries = 2000

type Guide struct {
	Status       string
	Action       string
	URL          string
	Version      string
	Host         string
	Listen       string
	BaseURL      string
	AdminURL     string
	ConfigPath   string
	DataDir      string
	BackupDir    string
	DatabasePath string
	Details      []string
}

type logLinesMsg []string
type guideMsg Guide
type replaceStateMsg struct {
	guide Guide
	logs  []string
}

type model struct {
	guide            Guide
	logs             []string
	width            int
	height           int
	scrollFromBottom int
	onInterrupt      func()
	ready            chan struct{}
}

func newModel(guide Guide, logs []string, onInterrupt func(), ready chan struct{}) model {
	return model{guide: guide, logs: append([]string(nil), logs...), width: 100, height: 30, onInterrupt: onInterrupt, ready: ready}
}

func (m model) Init() tea.Cmd {
	if m.ready == nil {
		return nil
	}
	return func() tea.Msg {
		close(m.ready)
		return nil
	}
}

func (m model) Update(message tea.Msg) (tea.Model, tea.Cmd) {
	switch message := message.(type) {
	case tea.WindowSizeMsg:
		m.width = message.Width
		m.height = message.Height
	case replaceStateMsg:
		m.guide = message.guide
		m.logs = append([]string(nil), message.logs...)
		m.trimLogs()
	case guideMsg:
		m.guide = Guide(message)
	case logLinesMsg:
		if m.scrollFromBottom > 0 {
			for _, line := range message {
				m.scrollFromBottom += len(wrappedLines(line, m.contentWidth()))
			}
		}
		m.logs = append(m.logs, message...)
		m.trimLogs()
	case tea.KeyMsg:
		switch message.String() {
		case "ctrl+c":
			if m.onInterrupt != nil {
				m.onInterrupt()
			}
			return m, tea.Quit
		case "up", "k":
			m.scrollFromBottom += 1
		case "down", "j":
			m.scrollFromBottom -= 1
		case "pgup":
			m.scrollFromBottom += m.logPageHeight()
		case "pgdown":
			m.scrollFromBottom -= m.logPageHeight()
		case "home":
			m.scrollFromBottom = len(m.renderedLogRows())
		case "end":
			m.scrollFromBottom = 0
		}
	}
	m.clampScroll()
	return m, nil
}

func (m model) View() string {
	width := m.width
	if width < 24 {
		width = 24
	}
	height := m.height
	if height < 10 {
		height = 10
	}
	panelWidth := width - 4
	contentWidth := width - 6
	title := lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("212")).Render("Prods")

	guideRows := m.guideRows(contentWidth)
	// Keep enough room for a useful log viewport while preferring complete host
	// and resource information in ordinary 24+ row terminals.
	maxGuideRows := height - 9
	if maxGuideRows < 4 {
		maxGuideRows = 4
	}
	if len(guideRows) > maxGuideRows {
		guideRows = append(guideRows[:maxGuideRows-1], "…")
	}
	panel := lipgloss.NewStyle().Border(lipgloss.RoundedBorder()).Padding(0, 1).Width(panelWidth)
	guidePanel := panel.Render(strings.Join(guideRows, "\n"))
	guideHeight := lipgloss.Height(guidePanel)

	logInnerHeight := height - 1 - guideHeight - 2
	if logInnerHeight < 3 {
		logInnerHeight = 3
	}
	logRows := m.visibleLogRows(contentWidth, logInnerHeight-1)
	logHeader := fmt.Sprintf("Log  ↑/↓ PgUp/PgDn Home/End  •  %d entries", len(m.logs))
	logBody := append([]string{logHeader}, logRows...)
	for len(logBody) < logInnerHeight {
		logBody = append(logBody, "")
	}
	logPanel := panel.Height(logInnerHeight).Render(strings.Join(logBody, "\n"))
	return lipgloss.JoinVertical(lipgloss.Left, title, guidePanel, logPanel)
}

func (m model) guideRows(width int) []string {
	rows := []string{"服務狀態與操作指引"}
	appendField := func(label, value string) {
		if strings.TrimSpace(value) == "" {
			return
		}
		rows = append(rows, wrappedLines(label+value, width)...)
	}
	appendField("操作：", m.guide.Action)
	appendField("網址：", terminalHyperlink(m.guide.URL))
	appendField("狀態：", m.guide.Status)
	for _, detail := range m.guide.Details {
		appendField("", detail)
	}
	appendField("版本：", m.guide.Version)
	appendField("主機：", m.guide.Host)
	appendField("監聽：", m.guide.Listen)
	appendField("服務網址：", terminalHyperlink(m.guide.BaseURL))
	appendField("Admin：", terminalHyperlink(m.guide.AdminURL))
	appendField("設定檔：", m.guide.ConfigPath)
	appendField("資料目錄：", m.guide.DataDir)
	appendField("備份目錄：", m.guide.BackupDir)
	appendField("資料庫：", m.guide.DatabasePath)
	return rows
}

func terminalHyperlink(value string) string {
	value = strings.TrimSpace(value)
	if value == "" {
		return ""
	}
	return ansi.SetHyperlink(value) + value + ansi.ResetHyperlink()
}

func (m model) visibleLogRows(width, height int) []string {
	rows := m.renderedLogRowsWithWidth(width)
	maximum := len(rows) - height
	if maximum < 0 {
		maximum = 0
	}
	scroll := m.scrollFromBottom
	if scroll > maximum {
		scroll = maximum
	}
	if scroll < 0 {
		scroll = 0
	}
	end := len(rows) - scroll
	start := end - height
	if start < 0 {
		start = 0
	}
	return append([]string(nil), rows[start:end]...)
}

func (m model) renderedLogRows() []string {
	return m.renderedLogRowsWithWidth(m.contentWidth())
}

func (m model) renderedLogRowsWithWidth(width int) []string {
	rows := make([]string, 0, len(m.logs))
	for _, line := range m.logs {
		rows = append(rows, wrappedLines(line, width)...)
	}
	return rows
}

func (m model) contentWidth() int {
	width := m.width - 6
	if width < 20 {
		return 20
	}
	return width
}

func (m model) logPageHeight() int {
	height := m.height / 2
	if height < 3 {
		return 3
	}
	return height
}

func (m *model) trimLogs() {
	if len(m.logs) <= maxLogEntries {
		return
	}
	dropped := len(m.logs) - maxLogEntries
	m.logs = append([]string(nil), m.logs[dropped:]...)
}

func (m *model) clampScroll() {
	maximum := len(m.renderedLogRows()) - 1
	if maximum < 0 {
		maximum = 0
	}
	if m.scrollFromBottom < 0 {
		m.scrollFromBottom = 0
	}
	if m.scrollFromBottom > maximum {
		m.scrollFromBottom = maximum
	}
}

func wrappedLines(value string, width int) []string {
	value = strings.ReplaceAll(value, "\t", "    ")
	value = strings.TrimRight(value, "\r\n")
	if value == "" {
		return []string{""}
	}
	if width < 1 {
		width = 1
	}
	return strings.Split(ansi.Hardwrap(value, width, true), "\n")
}
