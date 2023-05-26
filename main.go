package main

import (
	"github.com/bakito/pod-metrics/cmd/cli"
	// Import all Kubernetes client auth plugins (e.g. Azure, GCP, OIDC, etc.)
	_ "k8s.io/client-go/plugin/pkg/client/auth"
)

func main() {
	cli.Execute()
}
