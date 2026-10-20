package tui

import (
	"fmt"
	"regexp"
	"strings"

	"github.com/charmbracelet/lipgloss"
	"github.com/charmbracelet/x/ansi"

	"github.com/91dave/claude-sessions/internal/index"
)

const (
	dateFormat     = "2006-01-02 15:04"
	dateWidth      = 16
	wideLayoutFrom = 100
	headerLines    = 3
)

var (
	accent        = lipgloss.AdaptiveColor{Light: "#C30046", Dark: "#FF5F87"}
	subtle        = lipgloss.AdaptiveColor{Light: "#6C6C6C", Dark: "#8A8A8A"}
	dimStyle      = lipgloss.NewStyle().Foreground(subtle)
	titleStyle    = lipgloss.NewStyle().Bold(true)
	headStyle     = lipgloss.NewStyle().Bold(true).Foreground(accent)
	liveStyle     = lipgloss.NewStyle().Foreground(lipgloss.AdaptiveColor{Light: "#008700", Dark: "#5FD75F"})
	matchStyle    = lipgloss.NewStyle().Foreground(accent).Underline(true)
	cursorStyle   = lipgloss.NewStyle().Foreground(accent).Bold(true)
	selectedStyle = lipgloss.NewStyle().Bold(true).
			Foreground(lipgloss.AdaptiveColor{Light: "#1A1A1A", Dark: "#FFFFFF"}).
			Background(lipgloss.AdaptiveColor{Light: "#E4E3E0", Dark: "#3A3A4A"})
	errorStyle = lipgloss.NewStyle().Foreground(accent)
	noteStyle  = lipgloss.NewStyle().Foreground(subtle).Italic(true)
)

func visibleWidth(s string) int {
	return ansi.StringWidth(s)
}

func (m Model) View() string {
	if m.quitting || m.width == 0 {
		return ""
	}
	lines := []string{
		fit(dimStyle.Render(m.hints()), m.width),
		fit(m.input.View(), m.width),
		fit(m.statusLine(), m.width),
	}
	return strings.Join(append(lines, m.body()...), "\n")
}

func (m Model) hints() string {
	if m.mode == modeAsk {
		return "ask: describe the session, enter to search  •  ctrl+s or ?: back to filtering  •  esc: quit"
	}
	return fmt.Sprintf("enter: %s  •  ctrl+o/alt+enter: %s  •  ctrl+x/alt+d: hide  •  alt+e: end  •  ctrl+s or ?: ask  •  esc: quit  •  ● running",
		m.deps.Config.DefaultAction, m.deps.Config.OtherAction())
}

func (m Model) statusLine() string {
	switch {
	case m.loading:
		return dimStyle.Render("reading sessions…")
	case m.asking:
		return dimStyle.Render("asking Claude…")
	case m.status != "":
		return errorStyle.Render(m.status)
	case m.rankedFor != "":
		return dimStyle.Render(fmt.Sprintf("ranked for: %s  •  ctrl+s twice: all sessions", m.rankedFor))
	default:
		return dimStyle.Render(fmt.Sprintf("%d of %d sessions", len(m.rows), len(m.all)))
	}
}

func (m Model) bodyHeight() int {
	return max(0, m.height-headerLines)
}

func (m Model) wide() bool {
	return m.width >= wideLayoutFrom
}

func (m Model) listWidth() int {
	if m.wide() {
		return m.width * 55 / 100
	}
	return m.width
}

func (m Model) listHeight() int {
	if m.wide() {
		return m.bodyHeight()
	}
	return m.bodyHeight() / 2
}

func (m Model) body() []string {
	h := m.bodyHeight()
	if h == 0 {
		return nil
	}
	list := m.listLines(m.listWidth(), m.listHeight())
	preview := m.previewLines(m.previewSize())
	if m.wide() {
		out := make([]string, h)
		for i := range out {
			out[i] = pad(list[i], m.listWidth()) + dimStyle.Render(" │ ") + preview[i]
		}
		return out
	}
	out := append(list, dimStyle.Render(strings.Repeat("─", m.width)))
	return append(out, preview...)
}

func (m Model) previewSize() (lines, columns int) {
	if m.wide() {
		return m.bodyHeight(), m.width - m.listWidth() - 3
	}
	return m.bodyHeight() - m.listHeight() - 1, m.width
}

func (m Model) listLines(width, height int) []string {
	lines := make([]string, height)
	highlight := highlighter(m.query())
	for i := range lines {
		n := m.offset + i
		if n >= len(m.rows) {
			continue
		}
		gutter := "  "
		if n == m.cursor {
			gutter = cursorStyle.Render("▌ ")
		}
		r := m.rows[n]
		if r.entry == nil {
			lines[i] = gutter + noteStyle.Render(fit(r.note, width-2))
			continue
		}
		lines[i] = gutter + renderRow(*r.entry, width-2, n == m.cursor, highlight)
	}
	return lines
}

type columns struct{ title, project, branch, text int }

const (
	projectWidth  = 16
	branchWidth   = 14
	maxTitleWidth = 50
	columnGap     = 2
)

func layoutColumns(width int) columns {
	avail := width - dateWidth - 3
	withMeta := avail - projectWidth - branchWidth - 2*columnGap
	switch {
	case withMeta < 20:
		return columns{title: max(0, avail)}
	case withMeta < maxTitleWidth+20+columnGap:
		return columns{title: withMeta, project: projectWidth, branch: branchWidth}
	default:
		return columns{
			title:   maxTitleWidth,
			project: projectWidth,
			branch:  branchWidth,
			text:    withMeta - maxTitleWidth - columnGap,
		}
	}
}

type rowStyle struct {
	dim, live, highlight func(string) string
}

func plainText(s string) string { return s }

func render(style lipgloss.Style) func(string) string {
	return func(s string) string { return style.Render(s) }
}

func listRowStyle(highlight func(string) string) rowStyle {
	return rowStyle{dim: render(dimStyle), live: render(liveStyle), highlight: highlight}
}

var selectedRowStyle = rowStyle{dim: plainText, live: plainText, highlight: plainText}

func renderRow(e index.Entry, width int, selected bool, highlight func(string) string) string {
	if selected {
		return selectedStyle.Render(pad(rowText(e, width, selectedRowStyle), width))
	}
	return rowText(e, width, listRowStyle(highlight))
}

func rowText(e index.Entry, width int, style rowStyle) string {
	c := layoutColumns(width)
	mark := " "
	if e.Live {
		mark = style.live("●")
	}
	cell := func(s string, w int) string { return pad(style.highlight(fit(oneLine(s), w)), w) }
	gap := strings.Repeat(" ", columnGap)
	line := style.dim(e.ModTime.Format(dateFormat)) + " " + mark + " " + cell(e.Title(), c.title)
	if c.project > 0 {
		line += gap + cell(e.Project(), c.project) + gap + style.dim(cell(e.Branch, c.branch))
	}
	if c.text > 0 {
		line += gap + style.dim(cell(e.SearchText(), c.text))
	}
	return fit(line, width)
}

func (m Model) previewLines(height, width int) []string {
	lines := make([]string, max(0, height))
	e := m.selected()
	if e == nil || height <= 0 || width <= 0 {
		return lines
	}
	content := m.previewContent(*e, width)
	start := min(m.previewOffset, max(0, len(content)-1))
	for i := range lines {
		if start+i < len(content) {
			lines[i] = fit(content[start+i], width) + ansi.ResetStyle
		}
	}
	return lines
}

func (m Model) previewContent(e index.Entry, width int) []string {
	text, wrap := Summary(e), true
	if result, ok := m.previews[e.ID]; ok && result.ok {
		text, wrap = strings.TrimRight(result.text, "\n"), result.wrap
	}
	if wrap {
		text = lipgloss.NewStyle().Width(width).Render(text)
	}
	return strings.Split(text, "\n")
}

func Summary(e index.Entry) string {
	running := ""
	if e.Live {
		running = "   " + liveStyle.Render("● running")
	}
	started := "?"
	if !e.Started.IsZero() {
		started = e.Started.Local().Format(dateFormat)
	}
	var b strings.Builder
	fmt.Fprintf(&b, "%s\n\n", titleStyle.Render(e.Title()))
	fmt.Fprintf(&b, "%s %s\n", dimStyle.Render("Project    "), e.Cwd)
	fmt.Fprintf(&b, "%s %s   %s %d%s\n", dimStyle.Render("Branch     "), orNone(e.Branch), dimStyle.Render("Messages"), e.Messages, running)
	fmt.Fprintf(&b, "%s %s   %s %s\n\n", dimStyle.Render("Started    "), started, dimStyle.Render("Last active"), e.ModTime.Format(dateFormat))
	fmt.Fprintf(&b, "%s\n%s\n\n", headStyle.Render("Opening prompt"), orNone(e.OpeningPrompt))
	fmt.Fprintf(&b, "%s\n%s", headStyle.Render("Where it left off"), orNone(e.LastPrompt))
	return b.String()
}

func orNone(s string) string {
	if strings.TrimSpace(s) == "" {
		return "(none)"
	}
	return s
}

func oneLine(s string) string {
	return strings.Join(strings.Fields(s), " ")
}

func fit(s string, width int) string {
	if width <= 0 {
		return ""
	}
	return ansi.Truncate(s, width, "…")
}

func pad(s string, width int) string {
	if gap := width - visibleWidth(s); gap > 0 {
		return s + strings.Repeat(" ", gap)
	}
	return s
}

func highlighter(query string) func(string) string {
	terms := strings.Fields(query)
	if len(terms) == 0 {
		return func(s string) string { return s }
	}
	quoted := make([]string, len(terms))
	for i, t := range terms {
		quoted[i] = regexp.QuoteMeta(t)
	}
	pattern := regexp.MustCompile("(?i)" + strings.Join(quoted, "|"))
	return func(s string) string {
		return pattern.ReplaceAllStringFunc(s, func(match string) string { return matchStyle.Render(match) })
	}
}
