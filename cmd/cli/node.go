package cli

import (
	"context"
	"fmt"
	"math"
	"time"

	tea "charm.land/bubbletea/v2"
	"github.com/charmbracelet/lipgloss"
	"github.com/spf13/cobra"
	"golang.org/x/text/message"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	"k8s.io/client-go/discovery"
	metricsv1beta1 "k8s.io/metrics/pkg/apis/metrics/v1beta1"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

type nodeModel struct {
	nodeName   string
	apiReader  client.Reader
	node       *corev1.Node
	chartGroup ChartGroup
	cpuMax     float64
	memMax     float64
	cpuCurr    float64
	memCurr    float64
	err        error
	width      int
	height     int
	interval   time.Duration
	nbrPrinter *message.Printer
}

func nodesCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "node <node-name>",
		Short: "Live node usage <node-name>",
		Args:  cobra.MatchAll(cobra.ExactArgs(1)),
		RunE: func(_ *cobra.Command, args []string) error {
			c, dc, _, err := newClient()
			if err != nil {
				return err
			}

			return runNodeMetrics(args[0], c, dc)
		},
	}
}

func init() {
	cmd := nodesCmd()
	rootCmd.AddCommand(cmd)
	cmd.PersistentFlags().DurationVar(&interval, "interval", time.Second, "The interval in seconds to fetch metrics.")
}

func (m nodeModel) Init() tea.Cmd {
	return tea.Tick(m.interval, func(t time.Time) tea.Msg {
		return tickMsg(t)
	})
}

func (m nodeModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.KeyMsg:
		return handleKeyMsg(msg, m)
	case tea.WindowSizeMsg:
		m.width = msg.Width
		m.height = msg.Height
		m.chartGroup.Resize((m.width-2)/2, m.height-4)
	case tickMsg:
		cpu, mem, err := getNodeMetrics(context.Background(), m.apiReader, m.nodeName)
		if err != nil {
			m.err = err
			return m, nil
		}

		m.cpuMax = math.Max(m.cpuMax, cpu)
		m.memMax = math.Max(m.memMax, mem)
		m.cpuCurr = cpu
		m.memCurr = mem

		m.chartGroup.Push(cpu, mem)
		m.chartGroup.DrawAll()

		return m, tea.Tick(m.interval, func(t time.Time) tea.Msg {
			return tickMsg(t)
		})
	default:
	}
	return m, nil
}

func (m nodeModel) View() tea.View {
	if m.err != nil {
		return renderError(m.err)
	}

	nodeName := m.node.GetName()
	nodeColor := containerColors[0] // Magenta
	titleStyle := lipgloss.NewStyle().Foreground(lipgloss.Color(nodeColor)).Bold(true)

	header := fmt.Sprintf(" Node: %s / %s / %s \n Press q to quit\n\n",
		titleStyle.Render(nodeName),
		m.node.Status.NodeInfo.KubeletVersion,
		m.node.Status.NodeInfo.OSImage,
	)

	cpuTitle := fmt.Sprintf(" %s CPU (Cap: %dm / All: %dm / Curr: %s / Max: %s) ",
		nodeName,
		m.node.Status.Capacity.Cpu().ScaledValue(resource.Milli),
		m.node.Status.Allocatable.Cpu().ScaledValue(resource.Milli),
		m.nbrPrinter.Sprintf("%.0fm", m.cpuCurr*1000),
		m.nbrPrinter.Sprintf("%.0fm", m.cpuMax*1000),
	)

	memTitle := fmt.Sprintf(" %s Memory (Cap: %dGi / All: %dGi / Curr: %s / Max: %s) ",
		nodeName,
		m.node.Status.Capacity.Memory().ScaledValue(resource.Giga),
		m.node.Status.Allocatable.Memory().ScaledValue(resource.Giga),
		m.nbrPrinter.Sprintf("%.1fGi", m.memCurr/1024),
		m.nbrPrinter.Sprintf("%.1fGi", m.memMax/1024),
	)

	charts := m.chartGroup.Render(m.width, nodeColor, cpuTitle, memTitle)

	v := tea.NewView(header + charts)
	v.AltScreen = true
	return v
}

func runNodeMetrics(nodeName string, apiReader client.Reader, dc *discovery.DiscoveryClient) error {
	// Verify that metrics resource is available
	if err := verifyMetricsAvailable(dc, "nodes"); err != nil {
		return fmt.Errorf("metrics server is not available: %w", err)
	}

	ctx := context.Background()
	node := &corev1.Node{}
	err := apiReader.Get(ctx, client.ObjectKey{Name: nodeName}, node)
	if err != nil {
		return err
	}

	m := nodeModel{
		nodeName:   nodeName,
		apiReader:  apiReader,
		node:       node,
		interval:   interval,
		nbrPrinter: numberPrinter(),
		chartGroup: NewChartGroup(numberPrinter()),
	}

	p := tea.NewProgram(m)
	_, err = p.Run()
	return err
}

func getNodeMetrics(ctx context.Context, apiReader client.Reader, nodeName string) (
	cpu, mem float64, err error,
) {
	metrics := &metricsv1beta1.NodeMetrics{}
	err = apiReader.Get(ctx, client.ObjectKey{Name: nodeName}, metrics)
	if err != nil {
		return 0, 0, err
	}
	return float64(
			metrics.Usage.Cpu().MilliValue(),
		) / 1000, float64(
			metrics.Usage.Memory().Value(),
		) / (1024 * 1024), nil
}
