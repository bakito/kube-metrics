package cli

import (
	"context"
	"fmt"
	"log"
	"time"

	ui "github.com/gizak/termui/v3"
	"github.com/gizak/termui/v3/widgets"
	tb "github.com/nsf/termbox-go"
	"github.com/spf13/cobra"
	corev1 "k8s.io/api/core/v1"
	metricsv1beta1 "k8s.io/metrics/pkg/apis/metrics/v1beta1"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

var (
	containerName string
	interval      time.Duration
)

// podMetricsCmd represents the podResources command
var podMetricsCmd = &cobra.Command{
	Use:   "pod <pod-name>",
	Short: "Show live Pod Metrics",
	Args:  cobra.MatchAll(cobra.ExactArgs(1)),
	RunE: func(cmd *cobra.Command, args []string) error {
		c, defaultNs, err := newClient()
		if err != nil {
			return err
		}

		if namespace == "" {
			namespace = defaultNs
		}

		return run(namespace, args[0], c)
	},
}

func init() {
	rootCmd.AddCommand(podMetricsCmd)

	podMetricsCmd.PersistentFlags().StringVar(&containerName, "container", "", "A container name to show")
	podMetricsCmd.PersistentFlags().DurationVar(&interval, "interval", time.Second, "The interval in seconds to fetch metrics.")
}

func run(ns string, podName string, apiReader client.Reader) error {
	if err := ui.Init(); err != nil {
		log.Fatalf("failed to initialize termui: %v", err)
	}
	defer ui.Close()

	headerHeight := 4

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

	width, height := tb.Size()
	cpuData, memData, title, cpuPlots, memPlots := buildGraphs(ns, podName, headerHeight, pod, selectedContainers, width, height)

	draw := func() {
		cpu, mem, _ := getPodMetrics(ctx, apiReader, ns, podName)

		plots := []ui.Drawable{title}

		for _, container := range selectedContainers {
			n := container.Name
			cpuData[n] = append(cpuData[n][1:], cpu[n])
			memData[n] = append(memData[n][1:], mem[n])

			cpuPlots[n].Data[0] = cpuData[n]
			memPlots[n].Data[0] = memData[n]

			cpuPlots[n].Title = fmt.Sprintf(" %s CPU (Req: %s / Lim: %s / Curr: %.0fm) ",
				container.Name,
				container.Resources.Requests.Cpu(),
				container.Resources.Limits.Cpu(),
				cpu[n]*1000,
			)

			memPlots[n].Title = fmt.Sprintf(" %s Memory (Req: %s / Lim: %s / Curr: %.0fMi) ",
				container.Name,
				container.Resources.Requests.Memory(),
				container.Resources.Limits.Memory(),
				mem[n],
			)

			plots = append(plots, cpuPlots[n], memPlots[n])
		}

		ui.Render(plots...)
	}

	draw()
	uiEvents := ui.PollEvents()
	ticker := time.NewTicker(interval).C
	for {
		select {
		case e := <-uiEvents:
			switch e.ID {
			case "q", "<C-c>":
				return nil
			case "<Resize>":
				payload := e.Payload.(ui.Resize)
				cpuData, memData, title, cpuPlots, memPlots = buildGraphs(ns, podName, headerHeight, pod,
					selectedContainers, payload.Width, payload.Height)
				ui.Clear()
				draw()
			}
		case <-ticker:
			draw()
		}
	}
}

func buildGraphs(ns string, podName string, headerHeight int, pod *corev1.Pod, selectedContainers []corev1.Container, width int, height int) (map[string][]float64, map[string][]float64, *widgets.Paragraph, map[string]*widgets.Plot, map[string]*widgets.Plot) {
	height = height - headerHeight
	if containerName == "" {
		height = height / len(pod.Spec.Containers)
	}

	cpuData := initData(selectedContainers, width/2-5)
	memData := initData(selectedContainers, width/2-5)

	cpuPlots := make(map[string]*widgets.Plot)
	memPlots := make(map[string]*widgets.Plot)

	p := widgets.NewParagraph()
	p.Title = " Namespace / Pod "
	p.Text = fmt.Sprintf(" %s / %s\n Press q to quit", ns, podName)
	p.SetRect(0, 0, width, headerHeight)
	p.TextStyle.Fg = ui.ColorWhite
	p.BorderStyle.Fg = ui.ColorYellow

	for i, container := range selectedContainers {
		lc := newPlot()
		lc.Data[0] = cpuData[container.Name]
		lc.SetRect(0, i*height+headerHeight, width/2, (i+1)*height+headerHeight)
		lc.LineColors[0] = ui.ColorGreen
		cpuPlots[container.Name] = lc

		lc2 := newPlot()
		lc2.Data[0] = memData[container.Name]
		lc2.SetRect(width/2, i*height+headerHeight, width, (i+1)*height+headerHeight)
		lc2.LineColors[0] = ui.ColorYellow
		memPlots[container.Name] = lc2
	}
	return cpuData, memData, p, cpuPlots, memPlots
}

func newPlot() *widgets.Plot {
	p := widgets.NewPlot()
	p.Data = make([][]float64, 1)
	p.AxesColor = ui.ColorWhite
	p.TitleStyle.Fg = ui.ColorCyan
	// clone line colors as both instances have the same instance of the color array
	p.LineColors = append([]ui.Color{}, p.LineColors...)
	return p
}

func getPodMetrics(ctx context.Context, apiReader client.Reader, namespace string, podName string) (map[string]float64, map[string]float64, error) {
	metrics := &metricsv1beta1.PodMetrics{}
	err := apiReader.Get(ctx, client.ObjectKey{Namespace: namespace, Name: podName}, metrics)
	if err != nil {
		return nil, nil, err
	}
	cpu := make(map[string]float64)
	mem := make(map[string]float64)

	for _, c := range metrics.Containers {
		cpuRl := c.Usage[corev1.ResourceCPU]
		cpu[c.Name] = float64(cpuRl.MilliValue()) / 1000
		memRl := c.Usage[corev1.ResourceMemory]
		mem[c.Name] = float64(memRl.Value() / (1024 * 1024))
	}
	return cpu, mem, nil
}

func initData(containers []corev1.Container, size int) map[string][]float64 {
	data := make(map[string][]float64)
	for _, container := range containers {
		if containerName == "" || container.Name == containerName {
			data[container.Name] = make([]float64, size)
		}
	}
	return data
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
