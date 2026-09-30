package grpcplugin

import (
	"context"
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

	tokenExchange *authn.TokenExchangeClient
}

var (
	_ = pluginv3.Client(&ClientV3{})
)

// NewClientV3 dispenses clients for all grafana.plugin.v3 services from a
// negotiated go-plugin client connection.
//
// Experimental: Plugin protocol v3 is a work in progress and may change or be
// removed without notice.
func NewClientV3(rpcClient plugin.ClientProtocol, tokenExchange *authn.TokenExchangeClient) (*ClientV3, error) {
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
	user, ok := authlib.AuthInfoFrom(ctx)
	if ok && c.tokenExchange != nil {
		rsp, err := c.tokenExchange.Exchange(ctx, authn.TokenExchangeRequest{
			Namespace: user.GetNamespace(),
			Audiences: []string{group}, // and the pluginID?
		})
		if err != nil {
			return nil, err
		}

		md, _ := metadata.FromOutgoingContext(ctx)
		md = md.Copy()
		md.Set("x-access-token", rsp.Token)
		return metadata.NewOutgoingContext(ctx, md), nil
	}
	return ctx, nil
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
	ctx, err := c.addMetadataToContext(ctx, "") // ???? TODO, the request should include group
	if err != nil {
		return nil, err
	}
	return c.conversion.ConvertObjects(ctx, in)
}
