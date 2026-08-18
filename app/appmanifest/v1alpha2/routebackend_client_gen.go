package v1alpha2

import (
	"context"

	"github.com/grafana/grafana-app-sdk/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

type RouteBackendClient struct {
	client *resource.TypedClient[*RouteBackend, *RouteBackendList]
}

func NewRouteBackendClient(client resource.Client) *RouteBackendClient {
	return &RouteBackendClient{
		client: resource.NewTypedClient[*RouteBackend, *RouteBackendList](client, RouteBackendKind()),
	}
}

func NewRouteBackendClientFromGenerator(generator resource.ClientGenerator) (*RouteBackendClient, error) {
	c, err := generator.ClientFor(RouteBackendKind())
	if err != nil {
		return nil, err
	}
	return NewRouteBackendClient(c), nil
}

func (c *RouteBackendClient) Get(ctx context.Context, identifier resource.Identifier) (*RouteBackend, error) {
	return c.client.Get(ctx, identifier)
}

func (c *RouteBackendClient) List(ctx context.Context, namespace string, opts resource.ListOptions) (*RouteBackendList, error) {
	return c.client.List(ctx, namespace, opts)
}

func (c *RouteBackendClient) ListAll(ctx context.Context, namespace string, opts resource.ListOptions) (*RouteBackendList, error) {
	resp, err := c.client.List(ctx, namespace, resource.ListOptions{
		ResourceVersion: opts.ResourceVersion,
		Limit:           opts.Limit,
		LabelFilters:    opts.LabelFilters,
		FieldSelectors:  opts.FieldSelectors,
	})
	if err != nil {
		return nil, err
	}
	for resp.GetContinue() != "" {
		page, err := c.client.List(ctx, namespace, resource.ListOptions{
			Continue:        resp.GetContinue(),
			ResourceVersion: opts.ResourceVersion,
			Limit:           opts.Limit,
			LabelFilters:    opts.LabelFilters,
			FieldSelectors:  opts.FieldSelectors,
		})
		if err != nil {
			return nil, err
		}
		resp.SetContinue(page.GetContinue())
		resp.SetResourceVersion(page.GetResourceVersion())
		resp.SetItems(append(resp.GetItems(), page.GetItems()...))
	}
	return resp, nil
}

func (c *RouteBackendClient) Create(ctx context.Context, obj *RouteBackend, opts resource.CreateOptions) (*RouteBackend, error) {
	// Make sure apiVersion and kind are set
	obj.APIVersion = GroupVersion.Identifier()
	obj.Kind = RouteBackendKind().Kind()
	return c.client.Create(ctx, obj, opts)
}

func (c *RouteBackendClient) Update(ctx context.Context, obj *RouteBackend, opts resource.UpdateOptions) (*RouteBackend, error) {
	return c.client.Update(ctx, obj, opts)
}

func (c *RouteBackendClient) Patch(ctx context.Context, identifier resource.Identifier, req resource.PatchRequest, opts resource.PatchOptions) (*RouteBackend, error) {
	return c.client.Patch(ctx, identifier, req, opts)
}

func (c *RouteBackendClient) UpdateStatus(ctx context.Context, identifier resource.Identifier, newStatus RouteBackendStatus, opts resource.UpdateOptions) (*RouteBackend, error) {
	return c.client.Update(ctx, &RouteBackend{
		TypeMeta: metav1.TypeMeta{
			Kind:       RouteBackendKind().Kind(),
			APIVersion: GroupVersion.Identifier(),
		},
		ObjectMeta: metav1.ObjectMeta{
			ResourceVersion: opts.ResourceVersion,
			Namespace:       identifier.Namespace,
			Name:            identifier.Name,
		},
		Status: newStatus,
	}, resource.UpdateOptions{
		Subresource:     "status",
		ResourceVersion: opts.ResourceVersion,
	})
}

func (c *RouteBackendClient) Delete(ctx context.Context, identifier resource.Identifier, opts resource.DeleteOptions) error {
	return c.client.Delete(ctx, identifier, opts)
}
