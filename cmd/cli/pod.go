package cli

import (
	"context"
	"fmt"
	"math"
	"time"

	tea "charm.land/bubbletea/v2"
	"github.com/NimbleMarkets/ntcharts/linechart/streamlinechart"
	"github.com/charmbracelet/lipgloss"
	"github.com/spf13/cobra"
	"golang.org/x/text/message"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/client-go/discovery"
	metricsv1beta1 "k8s.io/metrics/pkg/apis/metrics/v1beta1"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

var (
	containerName string
	interval      time.Duration
)

type podModel struct {
	ns                 string
	podName            string
	apiReader          client.Reader
	pod                *corev1.Pod
	selectedContainers []corev1.Container
	cpuCharts          map[string]streamlinechart.Model
	memCharts          map[string]streamlinechart.Model
	cpuMax             map[string]float64
	memMax             map[string]float64
	cpuCurr            map[string]float64
	memCurr            map[string]float64
	err                error
	width              int
	height             int
	interval           time.Duration
	nbrPrinter         *message.Printer
}

func (m podModel) Init() tea.Cmd {
	return tea.Tick(m.interval, func(t time.Time) tea.Msg {
		return tickMsg(t)
	})
}

func (m podModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.KeyMsg:
		return handleKeyMsg(msg, m)
	case tea.WindowSizeMsg:
		m.width = msg.Width
		m.height = msg.Height
		chartWidth := (m.width - 2) / 2
		chartHeight := m.height - 6
		if len(m.selectedContainers) > 0 {
			chartHeight /= len(m.selectedContainers)
		}
		for _, c := range m.selectedContainers {
			cpu := m.cpuCharts[c.Name]
			cpu.Resize(chartWidth, chartHeight)
			m.cpuCharts[c.Name] = cpu
			mem := m.memCharts[c.Name]
			mem.Resize(chartWidth, chartHeight)
			m.memCharts[c.Name] = mem
		}
	case tickMsg:
		cpu, mem, err := getPodMetrics(context.Background(), m.apiReader, m.ns, m.podName)
		if err != nil {
			m.err = err
			return m, nil
		}

		for _, c := range m.selectedContainers {
			n := c.Name
			m.cpuMax[n] = math.Max(m.cpuMax[n], cpu[n])
			m.memMax[n] = math.Max(m.memMax[n], mem[n])
			m.cpuCurr[n] = cpu[n]
			m.memCurr[n] = mem[n]

			cpuChart := m.cpuCharts[n]
			cpuChart.Push(cpu[n])
			cpuChart.DrawAll()
			m.cpuCharts[n] = cpuChart

			memChart := m.memCharts[n]
			memChart.Push(mem[n])
			memChart.DrawAll()
			m.memCharts[n] = memChart
		}

		return m, tea.Tick(m.interval, func(t time.Time) tea.Msg {
			return tickMsg(t)
		})
	default:
	}
	return m, nil
}

func (m podModel) View() tea.View {
	if m.err != nil {
		return renderError(m.err)
	}

	header := fmt.Sprintf(" Namespace / Pod: %s / %s\n Press q to quit\n\n", m.ns, m.podName)

	var rows []string
	chartWidth := (m.width - 2) / 2
	for i, container := range m.selectedContainers {
		n := container.Name
		color := containerColors[i%len(containerColors)]
		style := lipgloss.NewStyle().Border(lipgloss.RoundedBorder()).BorderForeground(lipgloss.Color(color))
		titleStyle := lipgloss.NewStyle().Foreground(lipgloss.Color(color)).Bold(true)

		cpuTitle := titleStyle.Render(fmt.Sprintf(" %s CPU (Req: %s / Lim: %s / Curr: %s / Max: %s) ",
			container.Name,
			container.Resources.Requests.Cpu(),
			container.Resources.Limits.Cpu(),
			m.nbrPrinter.Sprintf("%.0fm", m.cpuCurr[n]*1000),
			m.nbrPrinter.Sprintf("%.0fm", m.cpuMax[n]*1000),
		))
		cpuTitle = lipgloss.NewStyle().MaxWidth(chartWidth).Render(cpuTitle)

		memTitle := titleStyle.Render(fmt.Sprintf(" %s Memory (Req: %s / Lim: %s / Curr: %s / Max: %s) ",
			container.Name,
			container.Resources.Requests.Memory(),
			container.Resources.Limits.Memory(),
			m.nbrPrinter.Sprintf("%.0fMi", m.memCurr[n]),
			m.nbrPrinter.Sprintf("%.0fMi", m.memMax[n]),
		))
		memTitle = lipgloss.NewStyle().MaxWidth(chartWidth).Render(memTitle)

		cpuView := lipgloss.JoinVertical(lipgloss.Left, cpuTitle, m.cpuCharts[n].View())
		memView := lipgloss.JoinVertical(lipgloss.Left, memTitle, m.memCharts[n].View())

		row := lipgloss.JoinHorizontal(lipgloss.Top, cpuView, memView)
		rows = append(rows, style.Render(row))
	}

	v := tea.NewView(header + lipgloss.JoinVertical(lipgloss.Left, rows...))
	v.AltScreen = true
	return v
}

var containerColors = []string{
	"5", // Magenta
	"6", // Cyan
	"3", // Yellow
	"1", // Red
	"4", // Blue
	"2", // Green
}

func runPodMetrics(ns, podName string, apiReader client.Reader, dc *discovery.DiscoveryClient) error {
	// Verify that metrics resource is available
	if err := verifyMetricsAvailable(dc, "pods"); err != nil {
		return fmt.Errorf("metrics server is not available: %w", err)
	}

	ctx := context.Background()

	pod := &corev1.Pod{}
	err := apiReader.Get(ctx, client.ObjectKey{Namespace: ns, Name: podName}, pod)
	if err != nil {
		return err
	}

	selectedContainers := selectContainers(pod.Spec.Containers)

	if len(selectedContainers) == 0 {
		return fmt.Errorf(`selected container %q not found in pod "%s/%s"`, containerName, ns, podName)
	}

	m := podModel{
		ns:                 ns,
		podName:            podName,
		apiReader:          apiReader,
		pod:                pod,
		selectedContainers: selectedContainers,
		cpuCharts:          make(map[string]streamlinechart.Model),
		memCharts:          make(map[string]streamlinechart.Model),
		cpuMax:             make(map[string]float64),
		memMax:             make(map[string]float64),
		cpuCurr:            make(map[string]float64),
		memCurr:            make(map[string]float64),
		interval:           interval,
		nbrPrinter:         numberPrinter(),
	}

	cpuStyle := lipgloss.NewStyle().Foreground(lipgloss.Color("2"))   // Green
	memStyle := lipgloss.NewStyle().Foreground(lipgloss.Color("12"))  // Blue
	axisStyle := lipgloss.NewStyle().Foreground(lipgloss.Color("8"))  // Gray
	labelStyle := lipgloss.NewStyle().Foreground(lipgloss.Color("7")) // White

	for _, c := range selectedContainers {
		m.cpuCharts[c.Name] = newStreamlineChart(m.nbrPrinter, cpuStyle, axisStyle, labelStyle, cpuFormat)
		m.memCharts[c.Name] = newStreamlineChart(m.nbrPrinter, memStyle, axisStyle, labelStyle, memFormat)
	}

	p := tea.NewProgram(m)
	_, err = p.Run()
	return err
}

func getPodMetrics(ctx context.Context, apiReader client.Reader, namespace, podName string) (
	cpu, mem map[string]float64, err error,
) {
	metrics := &metricsv1beta1.PodMetrics{}
	err = apiReader.Get(ctx, client.ObjectKey{Namespace: namespace, Name: podName}, metrics)
	if err != nil {
		return nil, nil, err
	}
	cpu = make(map[string]float64)
	mem = make(map[string]float64)

	for _, c := range metrics.Containers {
		cpuRl := c.Usage[corev1.ResourceCPU]
		cpu[c.Name] = float64(cpuRl.MilliValue()) / 1000
		memRl := c.Usage[corev1.ResourceMemory]
		mem[c.Name] = float64(memRl.Value() / (1024 * 1024))
	}
	return cpu, mem, nil
}

func selectContainers(containers []corev1.Container) []corev1.Container {
	var filtered []corev1.Container
	for _, c := range containers {
		if containerName == "" || c.Name == containerName {
			filtered = append(filtered, c)
		}
	}
	return filtered
}

func podCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "pod <pod-name>",
		Short: "Live pod metrics",
		Args:  cobra.MatchAll(cobra.ExactArgs(1)),
		RunE: func(_ *cobra.Command, args []string) error {
			c, dc, ns, err := newClient()
			if err != nil {
				return err
			}

			return runPodMetrics(ns, args[0], c, dc)
		},
	}
}

func init() {
	cmd := podCmd()
	rootCmd.AddCommand(cmd)

	cmd.PersistentFlags().StringVar(&containerName, "container", "", "A container name to show")
	cmd.PersistentFlags().DurationVar(&interval, "interval", time.Second, "The interval in seconds to fetch metrics.")
}
