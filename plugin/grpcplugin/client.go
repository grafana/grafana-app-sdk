package grpcplugin

import (
	"context"
	"errors"
	"fmt"

	"github.com/grafana/authlib/authn"
	authlib "github.com/grafana/authlib/types"
	"github.com/hashicorp/go-plugin"
	"google.golang.org/grpc"
	"google.golang.org/grpc/metadata"

	pluginv3 "github.com/grafana/grafana-app-sdk/plugin/genproto/grafana/plugin/v3"
)

// ClientV3 groups clients for the grafana.plugin.v3 services.
//
// Experimental: Plugin protocol v3 is a work in progress and may change or be
// removed without notice.
type ClientV3 struct {
	admission  pluginv3.AdmissionServiceClient
	conversion pluginv3.ConversionServiceClient
	route      pluginv3.RouteServiceClient

	tokenExchange authn.TokenExchanger
}

var _ pluginv3.Client = (*ClientV3)(nil)

// NewClientV3 dispenses clients for all grafana.plugin.v3 services from a
// negotiated go-plugin client connection. If tokenExchange is non-nil, each
// request exchanges the caller's signed access or ID token for an access token
// scoped to the caller's namespace and the requested resource API group. Missing
// caller credentials fail the request; they never fall back to service access.
// A nil tokenExchange leaves outgoing metadata unchanged.
//
// Experimental: Plugin protocol v3 is a work in progress and may change or be
// removed without notice.
func NewClientV3(rpcClient plugin.ClientProtocol, tokenExchange authn.TokenExchanger) (*ClientV3, error) {
	admission, err := dispense[pluginv3.AdmissionServiceClient](rpcClient, pluginKeyAdmission)
	if err != nil {
		return nil, err
	}

	conversion, err := dispense[pluginv3.ConversionServiceClient](rpcClient, pluginKeyConversion)
	if err != nil {
		return nil, err
	}

	router, err := dispense[pluginv3.RouteServiceClient](rpcClient, pluginKeyRouter)
	if err != nil {
		return nil, err
	}

	return &ClientV3{
		admission:     admission,
		conversion:    conversion,
		route:         router,
		tokenExchange: tokenExchange,
	}, nil
}

func dispense[T any](rpcClient plugin.ClientProtocol, key string) (T, error) {
	var zero T
	raw, err := rpcClient.Dispense(key)
	if err != nil {
		return zero, fmt.Errorf("dispense plugin %q: %w", key, err)
	}

	client, ok := raw.(T)
	if !ok {
		return zero, fmt.Errorf("dispense plugin %q: unexpected client type %T", key, raw)
	}
	return client, nil
}

func (c *ClientV3) addMetadataToContext(ctx context.Context, group string) (context.Context, error) {
	if c.tokenExchange == nil {
		return ctx, nil
	}
	if group == "" {
		return nil, errors.New("plugin token exchange: API group is required")
	}
	caller, ok := authlib.AuthInfoFrom(ctx)
	if !ok || caller == nil {
		return nil, errors.New("plugin token exchange: caller auth info is required")
	}
	// Prefer the access token to preserve an existing delegation chain.
	// An ID token starts a new on-behalf-of exchange for a user.
	subjectToken := caller.GetAccessToken()
	if subjectToken == "" {
		subjectToken = caller.GetIDToken()
	}
	if subjectToken == "" {
		return nil, errors.New("plugin token exchange: caller access or ID token is required")
	}
	if caller.GetNamespace() == "" {
		return nil, errors.New("plugin token exchange: caller namespace is required")
	}
	rsp, err := c.tokenExchange.Exchange(ctx, authn.TokenExchangeRequest{
		Namespace:    caller.GetNamespace(),
		Audiences:    []string{group},
		SubjectToken: subjectToken,
	})
	if err != nil {
		return nil, fmt.Errorf("plugin token exchange: %w", err)
	}
	if rsp == nil || rsp.Token == "" {
		return nil, errors.New("plugin token exchange: empty access token")
	}

	md, _ := metadata.FromOutgoingContext(ctx)
	md = md.Copy()
	md.Set("x-access-token", rsp.Token)
	// Identity is embedded in the exchanged token. A stale ID token must not
	// override it when the server authenticates the request.
	md.Delete("x-id-token")
	return metadata.NewOutgoingContext(ctx, md), nil
}

// AdmissionReview implements [pluginv3.Client].
func (c *ClientV3) AdmissionReview(ctx context.Context, in *pluginv3.AdmissionReviewRequest) (*pluginv3.AdmissionReviewResponse, error) {
	ctx, err := c.addMetadataToContext(ctx, in.GetKind().GetGroup())
	if err != nil {
		return nil, err
	}
	return c.admission.AdmissionReview(ctx, in)
}

// CallRoute implements [pluginv3.Client].
func (c *ClientV3) CallRoute(ctx context.Context, in *pluginv3.CallRouteRequest) (grpc.ServerStreamingClient[pluginv3.CallRouteResponse], error) {
	ctx, err := c.addMetadataToContext(ctx, in.GetGroup())
	if err != nil {
		return nil, err
	}
	return c.route.CallRoute(ctx, in)
}

// ConvertObjects implements [pluginv3.Client].
func (c *ClientV3) ConvertObjects(ctx context.Context, in *pluginv3.ConvertObjectsRequest) (*pluginv3.ConvertObjectsResponse, error) {
	// The request's API describes the conversion envelope, not the resources.
	// All converted objects must belong to the same API group.
	group := ""
	if c.tokenExchange != nil {
		for i, obj := range in.GetObjects() {
			objectGroup := obj.GetGvk().GetGroup()
			if objectGroup == "" || (i > 0 && objectGroup != group) {
				return nil, errors.New("plugin token exchange: conversion objects must have the same non-empty API group")
			}
			group = objectGroup
		}
	}
	ctx, err := c.addMetadataToContext(ctx, group)
	if err != nil {
		return nil, err
	}
	return c.conversion.ConvertObjects(ctx, in)
}
