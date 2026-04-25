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
	"k8s.io/apimachinery/pkg/labels"
	"k8s.io/client-go/discovery"
	metricsv1beta1 "k8s.io/metrics/pkg/apis/metrics/v1beta1"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

var (
	containerName string
	interval      time.Duration
	selector      string
)

type podModel struct {
	ns                 string
	podName            string
	apiReader          client.Reader
	pod                *corev1.Pod
	selectedContainers []corev1.Container
	chartGroups        map[string]ChartGroup
	cpuMax             map[string]float64
	memMax             map[string]float64
	cpuCurr            map[string]float64
	memCurr            map[string]float64
	err                error
	availableOptions   []string
	width              int
	height             int
	interval           time.Duration
	nbrPrinter         *message.Printer
	selectedIndex      int
	isFocused          bool
}

func podCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "pod [<pod-name>]",
		Short: "Live pod metrics",
		Args:  cobra.MaximumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			podName := ""
			if len(args) > 0 {
				podName = args[0]
			}

			if podName == "" && selector == "" {
				return cmd.Help()
			}

			c, dc, ns, err := newClient()
			if err != nil {
				return err
			}

			return runPodMetrics(ns, podName, c, dc)
		},
	}
}

func init() {
	cmd := podCmd()
	rootCmd.AddCommand(cmd)

	cmd.PersistentFlags().StringVar(&containerName, "container", "", "A container name to show")
	cmd.PersistentFlags().DurationVar(&interval, "interval", time.Second, "The interval in seconds to fetch metrics.")
	cmd.PersistentFlags().
		StringVarP(&selector, "selector", "l", "", "Selector (label query) to filter on, supports '=', '==', '!=', 'in', 'notin'.(e.g. -l key1=value1,key2=value2,key3 in (value3)). Matching objects must satisfy all of the specified label constraints.")
}

func (m podModel) Init() tea.Cmd {
	if m.err != nil {
		return nil
	}
	return tea.Tick(m.interval, func(t time.Time) tea.Msg {
		return tickMsg(t)
	})
}

func (m podModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
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
			if m.selectedIndex+cols < len(m.selectedContainers) {
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
			if m.selectedIndex < len(m.selectedContainers)-1 {
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

			group := m.chartGroups[n]
			group.Push(cpu[n], mem[n])
			group.DrawAll()
			m.chartGroups[n] = group
		}

		return m, tea.Tick(m.interval, func(t time.Time) tea.Msg {
			return tickMsg(t)
		})
	default:
	}
	return m, nil
}

func (m podModel) getCols() int {
	return 1
}

func (m podModel) recalculateSizes() podModel {
	if m.err != nil {
		return m
	}
	rowsCount := len(m.selectedContainers)
	if m.isFocused {
		rowsCount = 1
	}

	widthPerGroup := m.width
	chartWidth := (widthPerGroup - 2) / 2
	chartHeight := (m.height-3)/rowsCount - 6

	if m.isFocused {
		chartHeight = m.height - 9
	}

	if chartHeight < 2 {
		chartHeight = 2
	}

	for _, c := range m.selectedContainers {
		group := m.chartGroups[c.Name]
		group.Resize(chartWidth, chartHeight)
		m.chartGroups[c.Name] = group
	}
	return m
}

func (m podModel) View() tea.View {
	if m.err != nil {
		return renderError(m.err, m.availableOptions...)
	}

	help := "Press enter to focus, arrows to navigate, q to quit"
	if m.isFocused {
		help = "Press enter/esc to go back, q to quit"
	}

	header := fmt.Sprintf(" Namespace / Pod: %s / %s\n %s\n\n", m.ns, m.podName, help)

	cols := m.getCols()
	widthPerGroup := m.width / cols

	var rows []string
	var currentRow []string
	for i, container := range m.selectedContainers {
		if m.isFocused && i != m.selectedIndex {
			continue
		}

		n := container.Name
		color := "8"
		if i == m.selectedIndex {
			color = containerColors[i%len(containerColors)]
		}

		cpuLim := float64(container.Resources.Limits.Cpu().MilliValue()) / 1000
		memLim := float64(container.Resources.Limits.Memory().Value()) / (1024 * 1024)
		cpuReq := float64(container.Resources.Requests.Cpu().MilliValue()) / 1000
		memReq := float64(container.Resources.Requests.Memory().Value()) / (1024 * 1024)

		cpuUsedPerc := 0.0
		if cpuLim > 0 {
			cpuUsedPerc = m.cpuCurr[n] / cpuLim * 100
		}
		cpuReqPerc := 0.0
		if cpuLim > 0 {
			cpuReqPerc = cpuReq / cpuLim * 100
		}
		memUsedPerc := 0.0
		if memLim > 0 {
			memUsedPerc = m.memCurr[n] / memLim * 100
		}
		memReqPerc := 0.0
		if memLim > 0 {
			memReqPerc = memReq / memLim * 100
		}

		containerTitle := fmt.Sprintf("%s (CPU: %.0f%% / %.0f%% | Mem: %.0f%% / %.0f%%)",
			n,
			cpuUsedPerc,
			cpuReqPerc,
			memUsedPerc,
			memReqPerc,
		)

		cpuTitle := RenderInfoBox(m.nbrPrinter, "CPU", color, [][2]string{
			{"Used", m.nbrPrinter.Sprintf("%.0fm", m.cpuCurr[n]*1000)},
			{"Req", container.Resources.Requests.Cpu().String()},
			{"Max", m.nbrPrinter.Sprintf("%.0fm", m.cpuMax[n]*1000)},
			{"Lim", container.Resources.Limits.Cpu().String()},
		})

		memTitle := RenderInfoBox(m.nbrPrinter, "Memory", color, [][2]string{
			{"Used", m.nbrPrinter.Sprintf("%.0fMi", m.memCurr[n])},
			{"Req", container.Resources.Requests.Memory().String()},
			{"Max", m.nbrPrinter.Sprintf("%.0fMi", m.memMax[n])},
			{"Lim", container.Resources.Limits.Memory().String()},
		})

		group := m.chartGroups[n]
		view := group.Render(widthPerGroup, color, containerTitle, cpuTitle, memTitle, i == m.selectedIndex)
		currentRow = append(currentRow, view)

		if len(currentRow) == cols || i == len(m.selectedContainers)-1 {
			rows = append(rows, lipgloss.JoinHorizontal(lipgloss.Top, currentRow...))
			currentRow = nil
		}
	}

	v := tea.NewView(header + joinVertical(rows...))
	v.AltScreen = true
	return v
}

func runPodMetrics(ns, podName string, apiReader client.Reader, dc *discovery.DiscoveryClient) error {
	// Verify that a metrics resource is available
	if err := verifyMetricsAvailable(dc, "pods"); err != nil {
		return fmt.Errorf("metrics server is not available: %w", err)
	}

	ctx := context.Background()

	var pod *corev1.Pod
	var err error
	var availableOptions []string

	if selector != "" {
		sel, parseErr := labels.Parse(selector)
		if parseErr != nil {
			return fmt.Errorf("invalid selector: %w", parseErr)
		}
		podList := &corev1.PodList{}
		err = apiReader.List(ctx, podList, client.InNamespace(ns), client.MatchingLabelsSelector{Selector: sel})
		if err == nil {
			switch {
			case len(podList.Items) == 0:
				err = fmt.Errorf("no pods matching selector %q found in namespace %q", selector, ns)
			case len(podList.Items) > 1 && podName == "":
				err = fmt.Errorf("multiple pods matching selector %q found in namespace %q", selector, ns)
				for _, p := range podList.Items {
					availableOptions = append(availableOptions, p.Name)
				}
			case podName != "": // If podName is provided, find it in the list
				found := false
				for i := range podList.Items {
					if podList.Items[i].Name == podName {
						pod = &podList.Items[i]
						found = true
						break
					}
				}
				if !found {
					err = fmt.Errorf("pod %q not found in pods matching selector %q", podName, selector)
					for _, p := range podList.Items {
						availableOptions = append(availableOptions, p.Name)
					}
				}
			default:
				pod = &podList.Items[0]
				podName = pod.Name
			}
		}
	} else {
		pod = &corev1.Pod{}
		err = apiReader.Get(ctx, client.ObjectKey{Namespace: ns, Name: podName}, pod)
		if err != nil && isNotFound(err) {
			podList := &corev1.PodList{}
			if listErr := apiReader.List(ctx, podList, client.InNamespace(ns)); listErr == nil {
				for _, p := range podList.Items {
					availableOptions = append(availableOptions, p.Name)
				}
			}
		}
	}

	var selectedContainers []corev1.Container
	if err == nil && pod != nil {
		selectedContainers = selectContainers(pod.Spec.Containers)
		if len(selectedContainers) == 0 {
			err = fmt.Errorf(`selected container %q not found in pod "%s/%s"`, containerName, ns, podName)
		}
	}

	m := podModel{
		ns:                 ns,
		podName:            podName,
		apiReader:          apiReader,
		pod:                pod,
		selectedContainers: selectedContainers,
		chartGroups:        make(map[string]ChartGroup),
		cpuMax:             make(map[string]float64),
		memMax:             make(map[string]float64),
		cpuCurr:            make(map[string]float64),
		memCurr:            make(map[string]float64),
		interval:           interval,
		nbrPrinter:         numberPrinter(),
		err:                err,
		availableOptions:   availableOptions,
	}

	if err == nil {
		for _, c := range selectedContainers {
			m.chartGroups[c.Name] = NewChartGroup(m.nbrPrinter)
		}
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
