package v2alpha1

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"

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

type GetExampleRequest struct {
	Headers http.Header
}

func (c *GroupVersionClient) GetExample(ctx context.Context, namespace string, request GetExampleRequest) (*GetExampleResponse, error) {
	resp, err := c.client.CustomRouteRequest(ctx, namespace, "", "", resource.CustomRouteRequestOptions{
		Path:    "/example",
		Verb:    "GET",
		Headers: request.Headers,
	})
	if err != nil {
		return nil, err
	}
	cast := GetExampleResponse{}
	err = json.Unmarshal(resp, &cast)
	if err != nil {
		return nil, fmt.Errorf("unable to unmarshal response bytes into GetExampleResponse: %w", err)
	}
	return &cast, nil
}
