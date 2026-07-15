package v1alpha3

import (
	"context"

	"github.com/grafana/grafana-app-sdk/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

type AppRouteClient struct {
	client *resource.TypedClient[*AppRoute, *AppRouteList]
}

func NewAppRouteClient(client resource.Client) *AppRouteClient {
	return &AppRouteClient{
		client: resource.NewTypedClient[*AppRoute, *AppRouteList](client, AppRouteKind()),
	}
}

func NewAppRouteClientFromGenerator(generator resource.ClientGenerator) (*AppRouteClient, error) {
	c, err := generator.ClientFor(AppRouteKind())
	if err != nil {
		return nil, err
	}
	return NewAppRouteClient(c), nil
}

func (c *AppRouteClient) Get(ctx context.Context, identifier resource.Identifier) (*AppRoute, error) {
	return c.client.Get(ctx, identifier)
}

func (c *AppRouteClient) List(ctx context.Context, namespace string, opts resource.ListOptions) (*AppRouteList, error) {
	return c.client.List(ctx, namespace, opts)
}

func (c *AppRouteClient) ListAll(ctx context.Context, namespace string, opts resource.ListOptions) (*AppRouteList, error) {
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

func (c *AppRouteClient) Create(ctx context.Context, obj *AppRoute, opts resource.CreateOptions) (*AppRoute, error) {
	// Make sure apiVersion and kind are set
	obj.APIVersion = GroupVersion.Identifier()
	obj.Kind = AppRouteKind().Kind()
	return c.client.Create(ctx, obj, opts)
}

func (c *AppRouteClient) Update(ctx context.Context, obj *AppRoute, opts resource.UpdateOptions) (*AppRoute, error) {
	return c.client.Update(ctx, obj, opts)
}

func (c *AppRouteClient) Patch(ctx context.Context, identifier resource.Identifier, req resource.PatchRequest, opts resource.PatchOptions) (*AppRoute, error) {
	return c.client.Patch(ctx, identifier, req, opts)
}

func (c *AppRouteClient) UpdateStatus(ctx context.Context, identifier resource.Identifier, newStatus AppRouteStatus, opts resource.UpdateOptions) (*AppRoute, error) {
	return c.client.Update(ctx, &AppRoute{
		TypeMeta: metav1.TypeMeta{
			Kind:       AppRouteKind().Kind(),
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

func (c *AppRouteClient) Delete(ctx context.Context, identifier resource.Identifier, opts resource.DeleteOptions) error {
	return c.client.Delete(ctx, identifier, opts)
}
