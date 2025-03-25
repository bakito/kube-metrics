package cli

import (
	"context"
	"fmt"
	"log"
	"math"
	"time"

	ui "github.com/gizak/termui/v3"
	"github.com/gizak/termui/v3/widgets"
	tb "github.com/nsf/termbox-go"
	"github.com/spf13/cobra"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	metricsv1beta1 "k8s.io/metrics/pkg/apis/metrics/v1beta1"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

func nodesCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "node <node-name>",
		Short: "Live node usage <node-name>",
		Args:  cobra.MatchAll(cobra.ExactArgs(1)),
		RunE: func(_ *cobra.Command, args []string) error {
			c, _, err := newClient()
			if err != nil {
				return err
			}

			return runNodeMetrics(args[0], c)
		},
	}
}

func init() {
	cmd := nodesCmd()
	rootCmd.AddCommand(cmd)
	cmd.PersistentFlags().DurationVar(&interval, "interval", time.Second, "The interval in seconds to fetch metrics.")
}

func runNodeMetrics(nodeName string, apiReader client.Reader) error {
	if err := ui.Init(); err != nil {
		log.Fatalf("failed to initialize termui: %v", err)
	}
	defer ui.Close()

	headerHeight := 4

	ctx := context.Background()
	node := &corev1.Node{}
	err := apiReader.Get(ctx, client.ObjectKey{Name: nodeName}, node)
	if err != nil {
		return err
	}

	width, height := tb.Size()
	cpuData, memData, title, cpuPlots, memPlots := buildNodeGraphs(node, headerHeight, width, height)

	draw := func() {
		cpu, mem, _ := getNodeMetrics(ctx, apiReader, nodeName)

		plots := []ui.Drawable{title}

		nbrPrinter := numberPrinter()

		cpuData.data = append(cpuData.data[1:], cpu)
		memData.data = append(memData.data[1:], mem)

		cpuData.max = math.Max(cpuData.max, cpu)
		memData.max = math.Max(memData.max, mem)

		cpuPlots.Data[0] = cpuData.data
		memPlots.Data[0] = memData.data

		cpuPlots.Title = fmt.Sprintf(" %s CPU (Cap: %dm / All: %dm / Curr: %s / Max: %s) ",
			node.Name,
			node.Status.Capacity.Cpu().ScaledValue(resource.Milli),
			node.Status.Allocatable.Cpu().ScaledValue(resource.Milli),
			numberPrinter().Sprintf("%.0fm", cpu*1000),
			numberPrinter().Sprintf("%.0fm", cpuData.max*1000),
		)

		memPlots.Title = fmt.Sprintf(" %s Memory (Cap: %dGi / All: %dGi / Curr: %s / Max: %s) ",
			node.Name,
			node.Status.Capacity.Memory().ScaledValue(resource.Giga),
			node.Status.Allocatable.Memory().ScaledValue(resource.Giga),
			nbrPrinter.Sprintf("%.1fGi", mem),
			nbrPrinter.Sprintf("%.1fGi", memData.max),
		)

		plots = append(plots, cpuPlots, memPlots)

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
				payload := e.Payload.(ui.Resize) //nolint:revive,forcetypeassert
				cpuData, memData, title, cpuPlots, memPlots = buildNodeGraphs(node, headerHeight, payload.Width, payload.Height)
				ui.Clear()
				draw()
			}
		case <-ticker:
			draw()
		}
	}
}

func buildNodeGraphs(
	node *corev1.Node,
	headerHeight int,
	width int,
	height int,
) (cpuData, memData *plotData, p *widgets.Paragraph, lc, lc2 *widgets.Plot) {
	height -= headerHeight

	cpuData = &plotData{data: make([]float64, width/2-5)}
	memData = &plotData{data: make([]float64, width/2-5)}

	p = widgets.NewParagraph()
	p.Title = " Node "
	p.Text = fmt.Sprintf(
		" %s / %s / %s \n Press q to quit",
		node.GetName(),
		node.Status.NodeInfo.KubeletVersion,
		node.Status.NodeInfo.OSImage,
	)
	p.SetRect(0, 0, width, headerHeight)
	p.TextStyle.Fg = ui.ColorWhite
	p.BorderStyle.Fg = ui.ColorYellow

	lc = newPlot()
	lc.Data[0] = cpuData.data
	lc.SetRect(0, headerHeight, width/2, height+headerHeight)
	lc.LineColors[0] = ui.ColorGreen

	lc2 = newPlot()
	lc2.Data[0] = memData.data
	lc2.SetRect(width/2, headerHeight, width, height+headerHeight)
	lc2.LineColors[0] = ui.ColorYellow
	return cpuData, memData, p, lc, lc2
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
			metrics.Usage.Memory().ScaledValue(resource.Mega),
		) / 1024, nil
}
