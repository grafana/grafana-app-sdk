package plugin

import (
	"k8s.io/client-go/rest"

	"github.com/grafana/grafana-app-sdk/k8s"
	"github.com/grafana/grafana-app-sdk/resource"
)

// BuildClientGenerator returns a ClientGenerator that the plugin can use to access resources.
func BuildClientGenerator(kubeConfig rest.Config) resource.ClientGenerator {
	return k8s.NewClientRegistry(kubeConfig, k8s.ClientConfig{})
}
