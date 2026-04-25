package ui

import (
	"fmt"
	"path/filepath"
	"strings"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/knitli/spy/internal/config"
	"github.com/knitli/spy/internal/reader"
	"github.com/knitli/spy/internal/renderer"
)

type Model struct {
	filePath       string
	config         *config.Config
	content        *reader.Content
	renderer       *renderer.Renderer
	viewport       Viewport
	width          int
	height         int
	err            error
	showHelp       bool
	showOpenDialog bool
	searchQuery    string
}

type Viewport struct {
	scrollOffset int
	lineHeight   int
}

func NewModel(filePath string, cfg *config.Config) *Model {
	m := &Model{
		filePath:     filePath,
		config:       cfg,
		viewport:     Viewport{scrollOffset: 0, lineHeight: 1},
		showHelp:     false,
		showOpenDialog: false,
		err:          nil,
	}

	if filePath != "" {
		if err := m.loadFile(filePath); err != nil {
			m.err = err
		}
	}

	return m
}

func (m *Model) loadFile(filePath string) error {
	content, err := reader.ReadFile(filePath)
	if err != nil {
		return err
	}

	m.filePath = filePath
	m.content = content
	m.viewport.scrollOffset = 0

	return nil
}

func (m Model) Init() tea.Cmd {
	return nil
}

func (m Model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.KeyMsg:
		return m.handleKeyPress(msg)

	case tea.WindowSizeMsg:
		m.width = msg.Width
		m.height = msg.Height
		if m.renderer == nil {
			m.renderer = renderer.NewRenderer(m.width, m.height, m.config.Theme)
		} else {
			m.renderer = renderer.NewRenderer(m.width, m.height, m.config.Theme)
		}
		return m, nil

	case errMsg:
		m.err = msg
		return m, nil
	}

	return m, nil
}

func (m Model) View() string {
	if m.content == nil {
		return m.renderWelcome()
	}

	if m.width == 0 || m.height == 0 {
		return "Loading..."
	}

	content := m.renderer.Render(m.content)
	lines := strings.Split(content, "\n")

	viewHeight := m.height
	if m.config.ShowStatusBar {
		viewHeight -= 2
	}

	startLine := m.viewport.scrollOffset
	endLine := startLine + viewHeight

	if endLine > len(lines) {
		endLine = len(lines)
	}

	if startLine > len(lines) {
		startLine = len(lines) - 1
	}

	visibleLines := lines[startLine:endLine]
	mainView := strings.Join(visibleLines, "\n")

	if m.config.ShowStatusBar {
		fileName := filepath.Base(m.filePath)
		if fileName == "" {
			fileName = "spy"
		}
		statusBar := m.renderer.RenderStatusBar(fileName, startLine+1, len(lines))
		helpBar := m.renderer.RenderHelpBar()
		return mainView + "\n" + statusBar + "\n" + helpBar
	}

	return mainView
}

func (m Model) handleKeyPress(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	if m.showOpenDialog {
		return m.handleOpenDialogKey(msg)
	}

	switch msg.String() {
	case "q", "ctrl+c":
		return m, tea.Quit

	case "o":
		m.showOpenDialog = true
		return m, nil

	case "?", "h":
		m.showHelp = !m.showHelp
		return m, nil

	case "up", "k":
		if m.viewport.scrollOffset > 0 {
			m.viewport.scrollOffset--
		}
		return m, nil

	case "down", "j":
		if m.content != nil {
			lines := strings.Split(m.renderer.Render(m.content), "\n")
			maxScroll := len(lines) - (m.height - 2)
			if m.viewport.scrollOffset < maxScroll && maxScroll > 0 {
				m.viewport.scrollOffset++
			}
		}
		return m, nil

	case "home":
		m.viewport.scrollOffset = 0
		return m, nil

	case "end":
		if m.content != nil {
			lines := strings.Split(m.renderer.Render(m.content), "\n")
			m.viewport.scrollOffset = len(lines) - (m.height - 2)
			if m.viewport.scrollOffset < 0 {
				m.viewport.scrollOffset = 0
			}
		}
		return m, nil

	case "pageup":
		m.viewport.scrollOffset -= (m.height - 2)
		if m.viewport.scrollOffset < 0 {
			m.viewport.scrollOffset = 0
		}
		return m, nil

	case "pagedown":
		if m.content != nil {
			lines := strings.Split(m.renderer.Render(m.content), "\n")
			m.viewport.scrollOffset += (m.height - 2)
			maxScroll := len(lines) - (m.height - 2)
			if m.viewport.scrollOffset > maxScroll && maxScroll > 0 {
				m.viewport.scrollOffset = maxScroll
			}
		}
		return m, nil
	}

	return m, nil
}

func (m Model) handleOpenDialogKey(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch msg.String() {
	case "esc":
		m.showOpenDialog = false
		return m, nil

	case "enter":
		if m.searchQuery != "" {
			if err := m.loadFile(m.searchQuery); err != nil {
				m.err = err
			}
			m.searchQuery = ""
			m.showOpenDialog = false
		}
		return m, nil

	case "backspace":
		if len(m.searchQuery) > 0 {
			m.searchQuery = m.searchQuery[:len(m.searchQuery)-1]
		}
		return m, nil

	default:
		if len(msg.Runes) > 0 {
			m.searchQuery += string(msg.Runes[0])
		}
		return m, nil
	}
}

func (m Model) renderWelcome() string {
	return fmt.Sprintf(`
╔════════════════════════════════════════════════════════════════════╗
║                          spy - File Viewer                         ║
║           A syntax-highlighted reader for code and more            ║
╚════════════════════════════════════════════════════════════════════╝

Supported File Types:
  • Source code (Go, Python, Rust, JavaScript, etc.)
  • Markdown files
  • Plain text
  • PDFs
  • Images (PNG, JPG, GIF)

Keyboard Shortcuts:
  [o]      - Open file
  [q]      - Quit
  [↑/↓]    - Scroll up/down
  [k/j]    - Scroll up/down (vim style)
  [Home]   - Jump to start
  [End]    - Jump to end
  [PgUp]   - Page up
  [PgDn]   - Page down
  [?]      - Toggle help

Press 'o' to open a file or drag and drop a file onto this window.
`)
}

type errMsg error
