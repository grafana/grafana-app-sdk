package grpcplugin

import (
	"context"
	"errors"
	"fmt"
	"slices"

	"github.com/grafana/authlib/authn"
	authlib "github.com/grafana/authlib/types"
	"github.com/hashicorp/go-plugin"
	"google.golang.org/grpc"
	"google.golang.org/grpc/metadata"

	pluginv3 "github.com/grafana/grafana-app-sdk/plugin/genproto/grafana/plugin/v3"
)

// ClientV3 groups the plugin protocol v3 RPCs. Implementations handle caller
// authentication, so the methods take no gRPC call options.
//
// Experimental: Plugin protocol v3 is a work in progress and may change or be
// removed without notice.
type ClientV3 interface {
	AdmissionReview(ctx context.Context, in *pluginv3.AdmissionReviewRequest) (*pluginv3.AdmissionReviewResponse, error)
	ConvertObjects(ctx context.Context, in *pluginv3.ConvertObjectsRequest) (*pluginv3.ConvertObjectsResponse, error)
	CallRoute(ctx context.Context, in *pluginv3.CallRouteRequest) (grpc.ServerStreamingClient[pluginv3.CallRouteResponse], error)
}

// ClientV3Options configures how ClientV3 authenticates requests to a plugin.
//
// Experimental: Plugin protocol v3 is a work in progress and may change or be
// removed without notice.
type ClientV3Options struct {
	// TokenExchanger exchanges the caller's signed access or ID token for an
	// access token scoped to the request's namespace and API group. Missing
	// caller credentials fail the request; they never fall back to service
	// access, except as allowed by IsServiceIdentity.
	// If nil, requests carry no credentials, and plugins that authenticate
	// requests reject them.
	TokenExchanger authn.TokenExchanger

	// Groups lists the API groups the plugin serves. It is required with
	// TokenExchanger: tokens are only minted for these audiences, so a request
	// routed to the wrong plugin cannot hand it a token for another app.
	Groups []string

	// IsServiceIdentity reports whether ctx carries the host's own in-process
	// service identity, which has no signed token to exchange. Such requests
	// use the host's service token instead of a delegated one. Grafana passes
	// identity.IsServiceIdentity. If nil, every request needs a caller token.
	IsServiceIdentity func(ctx context.Context) bool
}

// ClientV3 groups clients for the grafana.plugin.v3 services.
//
// Experimental: Plugin protocol v3 is a work in progress and may change or be
// removed without notice.
type clientV3 struct {
	admission  pluginv3.AdmissionServiceClient
	conversion pluginv3.ConversionServiceClient
	route      pluginv3.RouteServiceClient

	tokenExchange     authn.TokenExchanger
	groups            []string
	isServiceIdentity func(ctx context.Context) bool
}

var _ ClientV3 = (*clientV3)(nil)

// NewClientV3 dispenses clients for all grafana.plugin.v3 services from a
// negotiated go-plugin client connection. See ClientV3Options for how requests
// are authenticated.
//
// Experimental: Plugin protocol v3 is a work in progress and may change or be
// removed without notice.
func NewClientV3(rpcClient plugin.ClientProtocol, opts ClientV3Options) (ClientV3, error) {
	if opts.TokenExchanger != nil {
		if len(opts.Groups) == 0 {
			return nil, errors.New("plugin token exchange: the plugin's API groups are required")
		}
		if slices.Contains(opts.Groups, "") {
			return nil, errors.New("plugin token exchange: API groups must not be empty")
		}
	}

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

	return &clientV3{
		admission:         admission,
		conversion:        conversion,
		route:             router,
		tokenExchange:     opts.TokenExchanger,
		groups:            slices.Clone(opts.Groups),
		isServiceIdentity: opts.IsServiceIdentity,
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

// addMetadataToContext attaches an access token for group. The token is scoped
// to namespace when the request has a single one, and otherwise to the caller's.
func (c *clientV3) addMetadataToContext(ctx context.Context, group, namespace string) (context.Context, error) {
	if c.tokenExchange == nil {
		return ctx, nil
	}
	if group == "" {
		return nil, errors.New("plugin token exchange: API group is required")
	}
	if !slices.Contains(c.groups, group) {
		return nil, fmt.Errorf("plugin token exchange: API group %q is not served by this plugin", group)
	}
	caller, ok := authlib.AuthInfoFrom(ctx)
	if !ok || caller == nil {
		return nil, errors.New("plugin token exchange: caller auth info is required")
	}
	// An empty subject token requests the host's own service token.
	subjectToken := ""
	if c.isServiceIdentity == nil || !c.isServiceIdentity(ctx) {
		// Prefer the access token to preserve an existing delegation chain.
		// An ID token starts a new on-behalf-of exchange for a user.
		subjectToken = caller.GetAccessToken()
		if subjectToken == "" {
			subjectToken = caller.GetIDToken()
		}
		if subjectToken == "" {
			return nil, errors.New("plugin token exchange: caller access or ID token is required")
		}
	}
	if caller.GetNamespace() == "" {
		return nil, errors.New("plugin token exchange: caller namespace is required")
	}
	if namespace == "" {
		namespace = caller.GetNamespace()
	} else if !authlib.NamespaceMatches(caller.GetNamespace(), namespace) {
		return nil, errors.New("plugin token exchange: caller namespace does not cover the requested namespace")
	}
	rsp, err := c.tokenExchange.Exchange(ctx, authn.TokenExchangeRequest{
		Namespace:    namespace,
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

// AdmissionReview implements [ClientV3].
func (c *clientV3) AdmissionReview(ctx context.Context, in *pluginv3.AdmissionReviewRequest) (*pluginv3.AdmissionReviewResponse, error) {
	namespace := ""
	if c.tokenExchange != nil {
		var err error
		namespace, err = singleNamespace(in.GetObjectBytes(), in.GetOldObjectBytes())
		if err != nil {
			return nil, err
		}
	}
	ctx, err := c.addMetadataToContext(ctx, in.GetKind().GetGroup(), namespace)
	if err != nil {
		return nil, err
	}
	return c.admission.AdmissionReview(ctx, in)
}

// CallRoute implements [ClientV3].
func (c *clientV3) CallRoute(ctx context.Context, in *pluginv3.CallRouteRequest) (grpc.ServerStreamingClient[pluginv3.CallRouteResponse], error) {
	ctx, err := c.addMetadataToContext(ctx, in.GetGroup(), in.GetNamespace())
	if err != nil {
		return nil, err
	}
	return c.route.CallRoute(ctx, in)
}

// ConvertObjects implements [ClientV3].
func (c *clientV3) ConvertObjects(ctx context.Context, in *pluginv3.ConvertObjectsRequest) (*pluginv3.ConvertObjectsResponse, error) {
	group, namespace := "", ""
	if c.tokenExchange != nil {
		var err error
		group, err = conversionGroup(in)
		if err != nil {
			return nil, err
		}
		raws := make([][]byte, 0, len(in.GetObjects()))
		for _, obj := range in.GetObjects() {
			raws = append(raws, obj.GetRaw())
		}
		namespace, err = singleNamespace(raws...)
		if err != nil {
			return nil, err
		}
	}
	ctx, err := c.addMetadataToContext(ctx, group, namespace)
	if err != nil {
		return nil, err
	}
	return c.conversion.ConvertObjects(ctx, in)
}

func conversionGroup(req *pluginv3.ConvertObjectsRequest) (string, error) {
	// The envelope's API describes the conversion protocol, not the resources.
	group := ""
	for i, obj := range req.GetObjects() {
		objectGroup := obj.GetGvk().GetGroup()
		if objectGroup == "" || (i > 0 && objectGroup != group) {
			return "", errors.New("conversion objects must have the same non-empty API group")
		}
		group = objectGroup
	}
	if group == "" {
		return "", errors.New("conversion requires objects with a non-empty API group")
	}
	return group, nil
}

// singleNamespace returns the namespace shared by all non-empty objects, or ""
// if they span several namespaces or are cluster-scoped. The token is then
// scoped to the caller's namespace, which the server checks against each object.
func singleNamespace(raws ...[]byte) (string, error) {
	namespaces, err := objectNamespaces(raws...)
	if err != nil {
		return "", err
	}
	if len(namespaces) == 0 {
		return "", nil
	}
	for _, ns := range namespaces[1:] {
		if ns != namespaces[0] {
			return "", nil
		}
	}
	return namespaces[0], nil
}
