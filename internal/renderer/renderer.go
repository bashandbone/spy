package renderer

import (
	"fmt"
	"strings"

	"github.com/alecthomas/chroma/v2/formatters"
	"github.com/alecthomas/chroma/v2/lexers"
	"github.com/alecthomas/chroma/v2/styles"
	"github.com/charmbracelet/lipgloss"
	"github.com/charmbracelet/glamour"
	"github.com/knitli/spy/internal/reader"
)

type Renderer struct {
	width  int
	height int
	theme  string
}

func NewRenderer(width, height int, theme string) *Renderer {
	return &Renderer{
		width:  width,
		height: height,
		theme:  theme,
	}
}

func (r *Renderer) Render(content *reader.Content) string {
	switch content.Type {
	case reader.FileTypeCode:
		return r.renderCode(content.FilePath, content.RawContent)
	case reader.FileTypeMarkdown:
		return r.renderMarkdown(content.RawContent)
	case reader.FileTypeImage, reader.FileTypePDF:
		return r.renderMetadata(content)
	default:
		return r.renderPlainText(content.RawContent)
	}
}

func (r *Renderer) renderCode(filePath, code string) string {
	_ = lexers.Match(filePath)
	_ = styles.Get(r.theme)
	_ = formatters.Get("terminal256")

	return r.renderPlainText(code)
}

func (r *Renderer) renderMarkdown(md string) string {
	tr, _ := glamour.NewTermRenderer(
		glamour.WithWordWrap(r.width - 4),
	)
	rendered, _ := tr.Render(md)
	return rendered
}

func (r *Renderer) renderPlainText(text string) string {
	lines := strings.Split(text, "\n")
	var result strings.Builder

	for i, line := range lines {
		if i > 0 {
			result.WriteString("\n")
		}
		if len(line) > r.width-4 {
			result.WriteString(line[:r.width-4])
		} else {
			result.WriteString(line)
		}
	}

	return result.String()
}

func (r *Renderer) renderMetadata(content *reader.Content) string {
	infoStyle := lipgloss.NewStyle().
		Foreground(lipgloss.Color("#7D56F4")).
		Bold(true)

	metaStyle := lipgloss.NewStyle().
		Foreground(lipgloss.Color("#9B9B9B"))

	var result strings.Builder
	result.WriteString(infoStyle.Render(fmt.Sprintf("File: %s\n", content.FilePath)))
	result.WriteString(infoStyle.Render(fmt.Sprintf("Type: %v\n\n", content.Type)))

	for key, value := range content.Metadata {
		result.WriteString(metaStyle.Render(fmt.Sprintf("%s: %s\n", key, value)))
	}

	result.WriteString("\n")
	result.WriteString(content.RawContent)

	return result.String()
}

func (r *Renderer) RenderStatusBar(fileName string, lineNum, totalLines int) string {
	statusStyle := lipgloss.NewStyle().
		Background(lipgloss.Color("#1E1E1E")).
		Foreground(lipgloss.Color("#FFFFFF")).
		Padding(0, 1)

	status := fmt.Sprintf(" %s | Line %d/%d ", fileName, lineNum, totalLines)
	padding := r.width - lipgloss.Width(status)
	if padding > 0 {
		status += strings.Repeat(" ", padding)
	}

	return statusStyle.Render(status)
}

func (r *Renderer) RenderHelpBar() string {
	helpStyle := lipgloss.NewStyle().
		Background(lipgloss.Color("#1E1E1E")).
		Foreground(lipgloss.Color("#808080")).
		Padding(0, 1)

	help := " [q]uit [o]pen [↓↑]scroll [h]ome [e]nd [?]help "
	padding := r.width - lipgloss.Width(help)
	if padding > 0 {
		help += strings.Repeat(" ", padding)
	}

	return helpStyle.Render(help)
}
