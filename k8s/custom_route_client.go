package k8s

import (
	"context"

	"github.com/grafana/grafana-app-sdk/resource"
)

var _ resource.CustomRouteClient = &CustomRouteClient{}

type CustomRouteClient struct {
	groupVersionClient *groupVersionClient
	defaultNamespace   string
}

func (c *CustomRouteClient) NamespacedRequest(ctx context.Context, namespace string, opts resource.CustomRouteRequestOptions) ([]byte, error) {
	if namespace == "" {
		namespace = c.defaultNamespace
	}
	return c.groupVersionClient.customRouteRequest(ctx, namespace, "", "", opts)
}

func (c *CustomRouteClient) ClusteredRequest(ctx context.Context, opts resource.CustomRouteRequestOptions) ([]byte, error) {
	return c.groupVersionClient.customRouteRequest(ctx, "", "", "", opts)
}
