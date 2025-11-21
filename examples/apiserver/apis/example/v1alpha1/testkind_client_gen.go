package v1alpha1

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"

	"github.com/grafana/grafana-app-sdk/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

type TestKindClient struct {
	client *resource.TypedClient[*TestKind, *TestKindList]
}

func NewTestKindClient(client resource.Client) *TestKindClient {
	return &TestKindClient{
		client: resource.NewTypedClient[*TestKind, *TestKindList](client, TestKindKind()),
	}
}

func NewTestKindClientFromGenerator(generator resource.ClientGenerator) (*TestKindClient, error) {
	c, err := generator.ClientFor(TestKindKind())
	if err != nil {
		return nil, err
	}
	return NewTestKindClient(c), nil
}

func (c *TestKindClient) Get(ctx context.Context, identifier resource.Identifier) (*TestKind, error) {
	return c.client.Get(ctx, identifier)
}

func (c *TestKindClient) List(ctx context.Context, namespace string, opts resource.ListOptions) (*TestKindList, error) {
	return c.client.List(ctx, namespace, opts)
}

func (c *TestKindClient) ListAll(ctx context.Context, namespace string, opts resource.ListOptions) (*TestKindList, error) {
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

func (c *TestKindClient) Create(ctx context.Context, obj *TestKind, opts resource.CreateOptions) (*TestKind, error) {
	// Make sure apiVersion and kind are set
	obj.APIVersion = GroupVersion.Identifier()
	obj.Kind = TestKindKind().Kind()
	return c.client.Create(ctx, obj, opts)
}

func (c *TestKindClient) Update(ctx context.Context, obj *TestKind, opts resource.UpdateOptions) (*TestKind, error) {
	return c.client.Update(ctx, obj, opts)
}

func (c *TestKindClient) Patch(ctx context.Context, identifier resource.Identifier, req resource.PatchRequest, opts resource.PatchOptions) (*TestKind, error) {
	return c.client.Patch(ctx, identifier, req, opts)
}

func (c *TestKindClient) UpdateMysubresource(ctx context.Context, identifier resource.Identifier, newMysubresource TestKindMysubresource, opts resource.UpdateOptions) (*TestKind, error) {
	return c.client.Update(ctx, &TestKind{
		TypeMeta: metav1.TypeMeta{
			Kind:       TestKindKind().Kind(),
			APIVersion: GroupVersion.Identifier(),
		},
		ObjectMeta: metav1.ObjectMeta{
			ResourceVersion: opts.ResourceVersion,
			Namespace:       identifier.Namespace,
			Name:            identifier.Name,
		},
		Mysubresource: newMysubresource,
	}, resource.UpdateOptions{
		Subresource:     "mysubresource",
		ResourceVersion: opts.ResourceVersion,
	})
}
func (c *TestKindClient) UpdateStatus(ctx context.Context, identifier resource.Identifier, newStatus TestKindStatus, opts resource.UpdateOptions) (*TestKind, error) {
	return c.client.Update(ctx, &TestKind{
		TypeMeta: metav1.TypeMeta{
			Kind:       TestKindKind().Kind(),
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

func (c *TestKindClient) Delete(ctx context.Context, identifier resource.Identifier, opts resource.DeleteOptions) error {
	return c.client.Delete(ctx, identifier, opts)
}

type GetMessageRequest struct {
	Headers http.Header
}

func (c *TestKindClient) GetMessage(ctx context.Context, identifier resource.Identifier, request GetMessageRequest) (*GetMessage, error) {
	resp, err := c.client.SubresourceRequest(ctx, identifier, resource.CustomRouteRequestOptions{
		Path:    "/bar",
		Verb:    "GET",
		Headers: request.Headers,
	})
	if err != nil {
		return nil, err
	}
	cast := GetMessage{}
	err = json.Unmarshal(resp, &cast)
	if err != nil {
		return nil, fmt.Errorf("unable to unmarshal response bytes into GetMessage: %w", err)
	}
	return &cast, nil
}

type GetFooRequest struct {
	Params  GetFooRequestParams
	Body    GetFooRequestBody
	Headers http.Header
}

func (c *TestKindClient) GetFoo(ctx context.Context, identifier resource.Identifier, request GetFooRequest) (*GetFoo, error) {
	params := url.Values{}
	params.Set("foo", fmt.Sprintf("%v", request.Params.Foo))
	body, err := json.Marshal(request.Body)
	if err != nil {
		return nil, fmt.Errorf("unable to marshal body to JSON: %w", err)
	}
	resp, err := c.client.SubresourceRequest(ctx, identifier, resource.CustomRouteRequestOptions{
		Path:    "/foo",
		Verb:    "GET",
		Query:   params,
		Body:    io.NopCloser(bytes.NewReader(body)),
		Headers: request.Headers,
	})
	if err != nil {
		return nil, err
	}
	cast := GetFoo{}
	err = json.Unmarshal(resp, &cast)
	if err != nil {
		return nil, fmt.Errorf("unable to unmarshal response bytes into GetFoo: %w", err)
	}
	return &cast, nil
}

type GetRecursiveResponseRequest struct {
	Headers http.Header
}

func (c *TestKindClient) GetRecursiveResponse(ctx context.Context, identifier resource.Identifier, request GetRecursiveResponseRequest) (*GetRecursiveResponse, error) {
	resp, err := c.client.SubresourceRequest(ctx, identifier, resource.CustomRouteRequestOptions{
		Path:    "/recurse",
		Verb:    "GET",
		Headers: request.Headers,
	})
	if err != nil {
		return nil, err
	}
	cast := GetRecursiveResponse{}
	err = json.Unmarshal(resp, &cast)
	if err != nil {
		return nil, fmt.Errorf("unable to unmarshal response bytes into GetRecursiveResponse: %w", err)
	}
	return &cast, nil
}
