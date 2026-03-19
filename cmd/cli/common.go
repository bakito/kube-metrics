package cli

import (
	"fmt"
	"time"

	tea "charm.land/bubbletea/v2"
	"github.com/NimbleMarkets/ntcharts/linechart/streamlinechart"
	"github.com/charmbracelet/lipgloss"
	"golang.org/x/text/message"
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

func newStreamlineChart(nbrPrinter *message.Printer, chartStyle, axisStyle, labelStyle lipgloss.Style) streamlinechart.Model {
	c := streamlinechart.New(20, 10)
	c.AutoMinY = true
	c.AutoMaxY = true
	c.AutoMinX = true
	c.AutoMaxX = true
	c.Style = chartStyle
	c.AxisStyle = axisStyle
	c.LabelStyle = labelStyle
	c.YLabelFormatter = func(_ int, v float64) string {
		return nbrPrinter.Sprintf("%.1f", v)
	}
	c.XLabelFormatter = func(_ int, v float64) string {
		return nbrPrinter.Sprintf("%.1f", v)
	}
	return c
}
