package v1alpha3

import (
	"context"

	"github.com/grafana/grafana-app-sdk/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

type RuntimeConfigClient struct {
	client *resource.TypedClient[*RuntimeConfig, *RuntimeConfigList]
}

func NewRuntimeConfigClient(client resource.Client) *RuntimeConfigClient {
	return &RuntimeConfigClient{
		client: resource.NewTypedClient[*RuntimeConfig, *RuntimeConfigList](client, RuntimeConfigKind()),
	}
}

func NewRuntimeConfigClientFromGenerator(generator resource.ClientGenerator) (*RuntimeConfigClient, error) {
	c, err := generator.ClientFor(RuntimeConfigKind())
	if err != nil {
		return nil, err
	}
	return NewRuntimeConfigClient(c), nil
}

func (c *RuntimeConfigClient) Get(ctx context.Context, identifier resource.Identifier) (*RuntimeConfig, error) {
	return c.client.Get(ctx, identifier)
}

func (c *RuntimeConfigClient) List(ctx context.Context, namespace string, opts resource.ListOptions) (*RuntimeConfigList, error) {
	return c.client.List(ctx, namespace, opts)
}

func (c *RuntimeConfigClient) ListAll(ctx context.Context, namespace string, opts resource.ListOptions) (*RuntimeConfigList, error) {
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

func (c *RuntimeConfigClient) Create(ctx context.Context, obj *RuntimeConfig, opts resource.CreateOptions) (*RuntimeConfig, error) {
	// Make sure apiVersion and kind are set
	obj.APIVersion = GroupVersion.Identifier()
	obj.Kind = RuntimeConfigKind().Kind()
	return c.client.Create(ctx, obj, opts)
}

func (c *RuntimeConfigClient) Update(ctx context.Context, obj *RuntimeConfig, opts resource.UpdateOptions) (*RuntimeConfig, error) {
	return c.client.Update(ctx, obj, opts)
}

func (c *RuntimeConfigClient) Patch(ctx context.Context, identifier resource.Identifier, req resource.PatchRequest, opts resource.PatchOptions) (*RuntimeConfig, error) {
	return c.client.Patch(ctx, identifier, req, opts)
}

func (c *RuntimeConfigClient) UpdateStatus(ctx context.Context, identifier resource.Identifier, newStatus RuntimeConfigStatus, opts resource.UpdateOptions) (*RuntimeConfig, error) {
	return c.client.Update(ctx, &RuntimeConfig{
		TypeMeta: metav1.TypeMeta{
			Kind:       RuntimeConfigKind().Kind(),
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

func (c *RuntimeConfigClient) Delete(ctx context.Context, identifier resource.Identifier, opts resource.DeleteOptions) error {
	return c.client.Delete(ctx, identifier, opts)
}
