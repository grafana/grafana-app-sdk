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

type GroupVersionClient struct {
	client resource.GroupVersionClient
}

func NewGroupVersionClient(client resource.GroupVersionClient) *GroupVersionClient {
	return &GroupVersionClient{
		client: client,
	}
}

func NewGroupVersionClientFromGenerator(generator resource.ClientGenerator) (*GroupVersionClient, error) {
	client, err := generator.ClientForGV(GroupVersion)
	if err != nil {
		return nil, err
	}
	return NewGroupVersionClient(client), nil
}

type GetFoobarRequest struct {
	Params  GetFoobarRequestParams
	Body    GetFoobarRequestBody
	Headers http.Header
}

func (c *GroupVersionClient) GetFoobar(ctx context.Context, namespace string, request GetFoobarRequest) (*GetFoobarResponse, error) {
	params := url.Values{}
	params.Set("foo", fmt.Sprintf("%v", request.Params.Foo))
	body, err := json.Marshal(request.Body)
	if err != nil {
		return nil, fmt.Errorf("unable to marshal body to JSON: %w", err)
	}
	resp, err := c.client.CustomRouteRequest(ctx, namespace, "", "", resource.CustomRouteRequestOptions{
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

func (c *GroupVersionClient) GetClusterFoobar(ctx context.Context, request GetClusterFoobarRequest) (*GetClusterFoobarResponse, error) {
	resp, err := c.client.CustomRouteRequest(ctx, "", "", "", resource.CustomRouteRequestOptions{
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
