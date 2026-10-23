package v1alpha1

import (
	"context"

	"github.com/grafana/grafana-app-sdk/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

type AppManifestClient struct {
	client *resource.TypedClient[*AppManifest, *AppManifestList]
}

func NewAppManifestClient(client resource.Client) *AppManifestClient {
	return &AppManifestClient{
		client: resource.NewTypedClient[*AppManifest, *AppManifestList](client, AppManifestKind()),
	}
}

func NewAppManifestClientFromGenerator(generator resource.ClientGenerator) (*AppManifestClient, error) {
	c, err := generator.ClientFor(AppManifestKind())
	if err != nil {
		return nil, err
	}
	return NewAppManifestClient(c), nil
}

func (c *AppManifestClient) Get(ctx context.Context, identifier resource.Identifier) (*AppManifest, error) {
	return c.client.Get(ctx, identifier)
}

func (c *AppManifestClient) List(ctx context.Context, namespace string, opts resource.ListOptions) (*AppManifestList, error) {
	return c.client.List(ctx, namespace, opts)
}

func (c *AppManifestClient) ListAll(ctx context.Context, namespace string, opts resource.ListOptions) (*AppManifestList, error) {
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

func (c *AppManifestClient) Create(ctx context.Context, obj *AppManifest, opts resource.CreateOptions) (*AppManifest, error) {
	// Make sure apiVersion and kind are set
	obj.APIVersion = GroupVersion.Identifier()
	obj.Kind = AppManifestKind().Kind()
	return c.client.Create(ctx, obj, opts)
}

func (c *AppManifestClient) Update(ctx context.Context, obj *AppManifest, opts resource.UpdateOptions) (*AppManifest, error) {
	return c.client.Update(ctx, obj, opts)
}

func (c *AppManifestClient) Patch(ctx context.Context, identifier resource.Identifier, req resource.PatchRequest, opts resource.PatchOptions) (*AppManifest, error) {
	return c.client.Patch(ctx, identifier, req, opts)
}

func (c *AppManifestClient) UpdateStatus(ctx context.Context, identifier resource.Identifier, newStatus AppManifestStatus, opts resource.UpdateOptions) (*AppManifest, error) {
	return c.client.Update(ctx, &AppManifest{
		TypeMeta: metav1.TypeMeta{
			Kind:       AppManifestKind().Kind(),
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

func (c *AppManifestClient) Delete(ctx context.Context, identifier resource.Identifier, opts resource.DeleteOptions) error {
	return c.client.Delete(ctx, identifier, opts)
}
