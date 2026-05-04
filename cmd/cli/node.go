package cli

import (
	"context"
	"fmt"
	"math"
	"time"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
	"github.com/spf13/cobra"
	"golang.org/x/text/message"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/client-go/discovery"
	metricsv1beta1 "k8s.io/metrics/pkg/apis/metrics/v1beta1"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

type nodeModel struct {
	apiReader        client.Reader
	selectedNodes    []corev1.Node
	chartGroups      map[string]ChartGroup
	cpuMax           map[string]float64
	memMax           map[string]float64
	cpuCurr          map[string]float64
	memCurr          map[string]float64
	cpuReq           map[string]float64
	memReq           map[string]float64
	err              error
	availableOptions []string
	width            int
	height           int
	interval         time.Duration
	nbrPrinter       *message.Printer
	selectedIndex    int
	isFocused        bool
}

func nodesCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "node [<node-name>]",
		Short: "Live node usage [<node-name>]",
		Args:  cobra.MaximumNArgs(1),
		RunE: func(_ *cobra.Command, args []string) error {
			nodeName := ""
			if len(args) > 0 {
				nodeName = args[0]
			}
			c, dc, _, err := newClient()
			if err != nil {
				return err
			}

			return runNodeMetrics(nodeName, c, dc)
		},
	}
}

func init() {
	cmd := nodesCmd()
	rootCmd.AddCommand(cmd)
	cmd.PersistentFlags().DurationVar(&interval, "interval", time.Second, "The interval in seconds to fetch metrics.")
}

func (m nodeModel) Init() tea.Cmd {
	if m.err != nil {
		return nil
	}
	return tea.Tick(m.interval, func(t time.Time) tea.Msg {
		return tickMsg(t)
	})
}

func (m nodeModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.KeyMsg:
		switch msg.String() {
		case "q", "ctrl+c":
			if m.isFocused {
				m.isFocused = false
				m = m.recalculateSizes()
				return m, nil
			}
			return m, tea.Quit
		case "enter":
			if m.err != nil {
				return m, nil
			}
			m.isFocused = !m.isFocused
			m = m.recalculateSizes()
			return m, nil
		case "esc", "backspace":
			if m.isFocused {
				m.isFocused = false
				m = m.recalculateSizes()
				return m, nil
			}
		case "up", "k":
			if m.err != nil {
				return m, nil
			}
			cols := m.getCols()
			if m.selectedIndex-cols >= 0 {
				m.selectedIndex -= cols
			}
		case "down", "j":
			if m.err != nil {
				return m, nil
			}
			cols := m.getCols()
			if m.selectedIndex+cols < len(m.selectedNodes) {
				m.selectedIndex += cols
			}
		case "left", "h":
			if m.err != nil {
				return m, nil
			}
			if m.selectedIndex > 0 {
				m.selectedIndex--
			}
		case "right", "l":
			if m.err != nil {
				return m, nil
			}
			if m.selectedIndex < len(m.selectedNodes)-1 {
				m.selectedIndex++
			}
		}
	case tea.WindowSizeMsg:
		m.width = msg.Width
		m.height = msg.Height
		m = m.recalculateSizes()
	case tickMsg:
		if m.err != nil {
			return m, nil
		}

		cpu, mem, err := getNodesMetrics(context.Background(), m.apiReader, "")
		if err != nil {
			m.err = err
			return m, nil
		}
		cpuReq, memReq, err := getNodesRequests(context.Background(), m.apiReader)
		if err != nil {
			m.err = err
			return m, nil
		}

		for _, n := range m.selectedNodes {
			name := n.Name
			m.cpuMax[name] = math.Max(m.cpuMax[name], cpu[name])
			m.memMax[name] = math.Max(m.memMax[name], mem[name])
			m.cpuCurr[name] = cpu[name]
			m.memCurr[name] = mem[name]
			m.cpuReq[name] = cpuReq[name]
			m.memReq[name] = memReq[name]

			group := m.chartGroups[name]
			group.Push(cpu[name], mem[name])
			group.DrawAll()
			m.chartGroups[name] = group
		}

		return m, tea.Tick(m.interval, func(t time.Time) tea.Msg {
			return tickMsg(t)
		})
	default:
	}
	return m, nil
}

func (m nodeModel) getCols() int {
	return 1
}

func (m nodeModel) recalculateSizes() nodeModel {
	if m.err != nil {
		return m
	}
	rowsCount := len(m.selectedNodes)
	if m.isFocused {
		rowsCount = 1
	}

	widthPerGroup := m.width
	chartWidth := (widthPerGroup - 2) / 2
	chartHeight := (m.height-4)/rowsCount - 8

	if m.isFocused {
		chartHeight = m.height - 12
	}

	if chartHeight < 2 {
		chartHeight = 2
	}

	for _, n := range m.selectedNodes {
		group := m.chartGroups[n.Name]
		group.Resize(chartWidth, chartHeight)
		m.chartGroups[n.Name] = group
	}
	return m
}

func (m nodeModel) View() tea.View {
	if m.err != nil {
		return renderError(m.err, m.availableOptions...)
	}

	help := "Press enter to focus, arrows to navigate, q to quit"
	if m.isFocused {
		help = "Press enter/esc to go back, q to quit"
	}

	header := fmt.Sprintf(" Nodes: %s\n\n", help)

	cols := m.getCols()
	widthPerGroup := m.width / cols

	var rows []string
	var currentRow []string
	for i, node := range m.selectedNodes {
		if m.isFocused && i != m.selectedIndex {
			continue
		}

		nodeName := node.Name
		color := "8"
		if i == m.selectedIndex {
			color = containerColors[i%len(containerColors)]
		}

		cpuAll := float64(node.Status.Allocatable.Cpu().MilliValue()) / 1000
		memAll := float64(node.Status.Allocatable.Memory().Value()) / (1024 * 1024)

		cpuUsedPerc := 0.0
		if cpuAll > 0 {
			cpuUsedPerc = m.cpuCurr[nodeName] / cpuAll * 100
		}
		cpuReqPerc := 0.0
		if cpuAll > 0 {
			cpuReqPerc = m.cpuReq[nodeName] / cpuAll * 100
		}
		memUsedPerc := 0.0
		if memAll > 0 {
			memUsedPerc = m.memCurr[nodeName] / memAll * 100
		}
		memReqPerc := 0.0
		if memAll > 0 {
			memReqPerc = m.memReq[nodeName] / memAll * 100
		}

		nodeTitle := fmt.Sprintf("%s (CPU: %.0f%% / %.0f%% | Mem: %.0f%% / %.0f%%)",
			nodeName,
			cpuUsedPerc,
			cpuReqPerc,
			memUsedPerc,
			memReqPerc,
		)

		cpuTitle := RenderInfoBox(m.nbrPrinter, "CPU", color, [][2]string{
			{"Used", m.nbrPrinter.Sprintf("%.0fm", m.cpuCurr[nodeName]*1000)},
			{"Req", m.nbrPrinter.Sprintf("%.0fm", m.cpuReq[nodeName]*1000)},
			{"Max", m.nbrPrinter.Sprintf("%.0fm", m.cpuMax[nodeName]*1000)},
			{"Alloc", m.nbrPrinter.Sprintf("%.0fm", cpuAll*1000)},
		})

		memTitle := RenderInfoBox(m.nbrPrinter, "Memory", color, [][2]string{
			{"Used", m.nbrPrinter.Sprintf("%.1fGi", m.memCurr[nodeName]/1024)},
			{"Req", m.nbrPrinter.Sprintf("%.1fGi", m.memReq[nodeName]/1024)},
			{"Max", m.nbrPrinter.Sprintf("%.1fGi", m.memMax[nodeName]/1024)},
			{"Alloc", m.nbrPrinter.Sprintf("%.1fGi", memAll/1024)},
		})

		group := m.chartGroups[nodeName]
		view := group.Render(widthPerGroup, color, nodeTitle, cpuTitle, memTitle, i == m.selectedIndex)
		currentRow = append(currentRow, view)

		if len(currentRow) == cols || i == len(m.selectedNodes)-1 {
			rows = append(rows, lipgloss.JoinHorizontal(lipgloss.Top, currentRow...))
			currentRow = nil
		}
	}

	v := tea.NewView(header + joinVertical(rows...))
	v.AltScreen = true
	return v
}

func runNodeMetrics(nodeName string, apiReader client.Reader, dc *discovery.DiscoveryClient) error {
	// Verify that a metrics resource is available
	if err := verifyMetricsAvailable(dc, "nodes"); err != nil {
		return fmt.Errorf("metrics server is not available: %w", err)
	}

	ctx := context.Background()
	var selectedNodes []corev1.Node
	var err error
	var availableOptions []string

	if nodeName != "" {
		node := &corev1.Node{}
		err = apiReader.Get(ctx, client.ObjectKey{Name: nodeName}, node)
		if err == nil {
			selectedNodes = append(selectedNodes, *node)
		} else if isNotFound(err) {
			nodeList := &corev1.NodeList{}
			if listErr := apiReader.List(ctx, nodeList); listErr == nil {
				for _, n := range nodeList.Items {
					availableOptions = append(availableOptions, n.Name)
				}
			}
		}
	} else {
		nodeList := &corev1.NodeList{}
		err = apiReader.List(ctx, nodeList)
		if err == nil {
			selectedNodes = nodeList.Items
		}
	}

	m := nodeModel{
		apiReader:        apiReader,
		selectedNodes:    selectedNodes,
		chartGroups:      make(map[string]ChartGroup),
		cpuMax:           make(map[string]float64),
		memMax:           make(map[string]float64),
		cpuCurr:          make(map[string]float64),
		memCurr:          make(map[string]float64),
		cpuReq:           make(map[string]float64),
		memReq:           make(map[string]float64),
		interval:         interval,
		nbrPrinter:       numberPrinter(),
		err:              err,
		availableOptions: availableOptions,
	}

	if err == nil {
		for _, n := range selectedNodes {
			m.chartGroups[n.Name] = NewChartGroup(m.nbrPrinter)
		}
	}

	p := tea.NewProgram(m)
	_, err = p.Run()
	return err
}

func getNodesMetrics(ctx context.Context, apiReader client.Reader, nodeName string) (
	cpu, mem map[string]float64, err error,
) {
	cpu = make(map[string]float64)
	mem = make(map[string]float64)
	if nodeName != "" {
		metrics := &metricsv1beta1.NodeMetrics{}
		err = apiReader.Get(ctx, client.ObjectKey{Name: nodeName}, metrics)
		if err != nil {
			return nil, nil, err
		}
		cpu[nodeName] = float64(metrics.Usage.Cpu().MilliValue()) / 1000
		mem[nodeName] = float64(metrics.Usage.Memory().Value()) / (1024 * 1024)
	} else {
		metricsList := &metricsv1beta1.NodeMetricsList{}
		err = apiReader.List(ctx, metricsList)
		if err != nil {
			return nil, nil, err
		}
		for _, m := range metricsList.Items {
			cpu[m.Name] = float64(m.Usage.Cpu().MilliValue()) / 1000
			mem[m.Name] = float64(m.Usage.Memory().Value()) / (1024 * 1024)
		}
	}
	return cpu, mem, nil
}

func getNodesRequests(ctx context.Context, apiReader client.Reader) (cpu, mem map[string]float64, err error) {
	podList := &corev1.PodList{}
	err = apiReader.List(ctx, podList)
	if err != nil {
		return nil, nil, err
	}
	cpu = make(map[string]float64)
	mem = make(map[string]float64)
	for _, pod := range podList.Items {
		if pod.Spec.NodeName == "" || pod.Status.Phase == corev1.PodSucceeded || pod.Status.Phase == corev1.PodFailed {
			continue
		}
		for _, container := range pod.Spec.Containers {
			cpu[pod.Spec.NodeName] += float64(container.Resources.Requests.Cpu().MilliValue()) / 1000
			mem[pod.Spec.NodeName] += float64(container.Resources.Requests.Memory().Value()) / (1024 * 1024)
		}
	}
	return cpu, mem, nil
}
