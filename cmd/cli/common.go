package cli

import (
	"fmt"
	"strings"
	"time"

	tea "charm.land/bubbletea/v2"
	"github.com/NimbleMarkets/ntcharts/canvas/runes"
	"github.com/NimbleMarkets/ntcharts/linechart/streamlinechart"
	"github.com/charmbracelet/lipgloss"
	"golang.org/x/text/message"
)

var (
	memFormat = func(v float64) string {
		return fmt.Sprintf("%.2fGi", v/1024)
	}
	cpuFormat = func(v float64) string {
		return fmt.Sprintf("%.2f", v)
	}

	containerColors = []string{
		"5", // Magenta
		"6", // Cyan
		"3", // Yellow
		"1", // Red
		"4", // Blue
		"2", // Green
	}

	cpuStyle   = lipgloss.NewStyle().Foreground(lipgloss.Color("2"))  // Green
	memStyle   = lipgloss.NewStyle().Foreground(lipgloss.Color("12")) // Blue
	axisStyle  = lipgloss.NewStyle().Foreground(lipgloss.Color("8"))  // Gray
	labelStyle = lipgloss.NewStyle().Foreground(lipgloss.Color("7"))  // White
)

type tickMsg time.Time

func handleKeyMsg(msg tea.KeyMsg, m tea.Model) (tea.Model, tea.Cmd) {
	switch msg.String() {
	case "q", "ctrl+c":
		return m, tea.Quit
	}
	return m, nil
}

func renderError(err error) tea.View {
	return tea.NewView(fmt.Sprintf("Error: %v\n", err))
}

func newStreamlineChart(
	nbrPrinter *message.Printer,
	chartStyle, axisStyle, labelStyle lipgloss.Style,
	yFormat func(v float64) string,
) streamlinechart.Model {
	c := streamlinechart.New(20, 10)
	c.AutoMinY = true
	c.AutoMaxY = true
	c.AutoMinX = true
	c.AutoMaxX = true
	c.SetYStep(4)
	c.SetXStep(4)
	c.AxisStyle = axisStyle
	c.LabelStyle = labelStyle
	c.YLabelFormatter = func(_ int, v float64) string {
		return nbrPrinter.Sprint(yFormat(v))
	}
	c.XLabelFormatter = func(_ int, v float64) string { return "" }
	c.SetStyles(runes.ArcLineStyle, chartStyle)
	return c
}

// ChartGroup encapsulates a pair of CPU and Memory charts.
type ChartGroup struct {
	CPU streamlinechart.Model
	Mem streamlinechart.Model
}

func NewChartGroup(nbrPrinter *message.Printer) ChartGroup {
	return ChartGroup{
		CPU: newStreamlineChart(nbrPrinter, cpuStyle, axisStyle, labelStyle, cpuFormat),
		Mem: newStreamlineChart(nbrPrinter, memStyle, axisStyle, labelStyle, memFormat),
	}
}

func (g *ChartGroup) Resize(width, height int) {
	g.CPU.Resize(width, height)
	g.Mem.Resize(width, height)
}

func (g *ChartGroup) Push(cpu, mem float64) {
	g.CPU.Push(cpu)
	g.Mem.Push(mem)
}

func (g *ChartGroup) DrawAll() {
	g.CPU.DrawAll()
	g.Mem.DrawAll()
}

func (g *ChartGroup) Render(width int, color, groupTitle, cpuTitle, memTitle string, selected bool) string {
	chartWidth := (width - 2) / 2
	border := lipgloss.RoundedBorder()
	if selected {
		border = lipgloss.ThickBorder()
	}

	style := lipgloss.NewStyle().
		Border(border).
		BorderForeground(lipgloss.Color(color))

	titleStyle := lipgloss.NewStyle().Foreground(lipgloss.Color(color)).Bold(true).MaxWidth(chartWidth)

	cpuView := lipgloss.JoinVertical(lipgloss.Left, titleStyle.Render(cpuTitle), g.CPU.View())
	memView := lipgloss.JoinVertical(lipgloss.Left, titleStyle.Render(memTitle), g.Mem.View())

	content := lipgloss.JoinHorizontal(lipgloss.Top, cpuView, memView)
	rendered := style.Render(content)

	if groupTitle != "" {
		lines := strings.Split(rendered, "\n")
		if len(lines) > 0 {
			// Construct titled top border
			t := " " + groupTitle + " "
			borderStyle := lipgloss.NewStyle().Foreground(lipgloss.Color(color))
			if selected {
				borderStyle = borderStyle.Bold(true)
			}

			topLeft := borderStyle.Render(border.TopLeft)
			topRight := borderStyle.Render(border.TopRight)
			topChar := borderStyle.Render(border.Top)

			// Total width of the rendered content with borders
			totalWidth := lipgloss.Width(lines[0])

			// We want: [TopLeft][Top][ Title ][Top...][TopRight]
			// The content width is totalWidth - 2 (for the corners)

			titleRendered := borderStyle.Render(t)
			titleWidth := lipgloss.Width(titleRendered)

			if totalWidth > titleWidth+4 {
				prefix := topLeft + topChar
				suffixWidth := totalWidth - lipgloss.Width(prefix) - titleWidth
				suffix := ""
				var suffixSb148 strings.Builder
				for range suffixWidth - 1 {
					suffixSb148.WriteString(border.Top)
				}
				suffix += suffixSb148.String()
				suffix = borderStyle.Render(suffix) + topRight
				lines[0] = prefix + titleRendered + suffix
			}
		}
		rendered = strings.Join(lines, "\n")
	}

	return rendered
}
