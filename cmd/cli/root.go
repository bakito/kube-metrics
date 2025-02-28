package cli

import (
	"fmt"
	"os"

	"github.com/bakito/kube-metrics/version"
	"github.com/spf13/cobra"
	"golang.org/x/text/language"
	"golang.org/x/text/message"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/runtime"
	utilruntime "k8s.io/apimachinery/pkg/util/runtime"
	"k8s.io/cli-runtime/pkg/genericclioptions"
	metricsv1beta1 "k8s.io/metrics/pkg/apis/metrics/v1beta1"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

var (
	scheme            = runtime.NewScheme()
	namespace         string
	nbrFormatLanguage string

	// rootCmd represents the base command when called without any subcommands
	rootCmd = &cobra.Command{
		Use:     "metrics",
		Short:   "Metrics",
		Version: version.Version,
	}
)

// Execute adds all child commands to the root command and sets flags appropriately.
// This is called by main.main(). It only needs to happen once to the rootCmd.
func Execute() {
	if err := rootCmd.Execute(); err != nil {
		fmt.Println(err)
		os.Exit(1)
	}
}

func init() {
	rootCmd.SetOut(os.Stdout)
	utilruntime.Must(corev1.AddToScheme(scheme))
	utilruntime.Must(metricsv1beta1.AddToScheme(scheme))

	rootCmd.PersistentFlags().StringVarP(&namespace, "namespace", "n", "",
		"If present, the namespace scope for this CLI request. Otherwise the cube context namespace is used.")
	rootCmd.PersistentFlags().StringVar(&nbrFormatLanguage, "nfl", "de-CH", "the number format language to be used.")
}

// get a k8s client and default namespace
func newClient() (client.Client, string, error) {
	cf := genericclioptions.NewConfigFlags(true)

	var err error
	ns := namespace
	if ns == "" {
		ns, _, err = cf.ToRawKubeConfigLoader().Namespace()
		if err != nil {
			ns = ""
		}
	}

	config, err := cf.ToRESTConfig()
	if err != nil {
		return nil, ns, err
	}

	mapper, err := cf.ToRESTMapper()
	if err != nil {
		return nil, ns, err
	}
	cl, err := client.New(config, client.Options{Scheme: scheme, Mapper: mapper})
	return cl, ns, err
}

func numberPrinter() *message.Printer {
	return message.NewPrinter(language.MustParse(nbrFormatLanguage))
}
