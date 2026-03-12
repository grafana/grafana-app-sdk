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
)

type CustomRouteClient struct {
	resource.CustomRouteClient
}

func NewCustomRouteClient(client resource.CustomRouteClient) *CustomRouteClient {
	return &CustomRouteClient{client}
}

func NewCustomRouteClientFromGenerator(generator resource.ClientGenerator, defaultNamespace string) (*CustomRouteClient, error) {
	client, err := generator.GetClient(GroupVersion, defaultNamespace)
	if err != nil {
		return nil, err
	}
	return NewCustomRouteClient(client), nil
}

type GetFoobarRequest struct {
	Params  GetFoobarRequestParams
	Body    GetFoobarRequestBody
	Headers http.Header
}

func (c *CustomRouteClient) GetFoobar(ctx context.Context, namespace string, request GetFoobarRequest) (*GetFoobarResponse, error) {
	params := url.Values{}
	params.Set("foo", fmt.Sprintf("%v", request.Params.Foo))
	body, err := json.Marshal(request.Body)
	if err != nil {
		return nil, fmt.Errorf("unable to marshal body to JSON: %w", err)
	}
	resp, err := c.NamespacedRequest(ctx, namespace, resource.CustomRouteRequestOptions{
		Path:    "/foobar",
		Verb:    "GET",
		Query:   params,
		Body:    io.NopCloser(bytes.NewReader(body)),
		Headers: request.Headers,
	})
	if err != nil {
		return nil, err
	}
	cast := GetFoobarResponse{}
	err = json.Unmarshal(resp, &cast)
	if err != nil {
		return nil, fmt.Errorf("unable to unmarshal response bytes into GetFoobarResponse: %w", err)
	}
	return &cast, nil
}

type GetClusterFoobarRequest struct {
	Headers http.Header
}

func (c *CustomRouteClient) GetClusterFoobar(ctx context.Context, request GetClusterFoobarRequest) (*GetClusterFoobarResponse, error) {
	resp, err := c.ClusteredRequest(ctx, resource.CustomRouteRequestOptions{
		Path:    "/foobar",
		Verb:    "GET",
		Headers: request.Headers,
	})
	if err != nil {
		return nil, err
	}
	cast := GetClusterFoobarResponse{}
	err = json.Unmarshal(resp, &cast)
	if err != nil {
		return nil, fmt.Errorf("unable to unmarshal response bytes into GetClusterFoobarResponse: %w", err)
	}
	return &cast, nil
}
