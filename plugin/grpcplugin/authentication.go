package grpcplugin

import (
	"context"
	"encoding/json"
	"fmt"
	"slices"

	"github.com/grafana/authlib/authn"
	"github.com/grafana/authlib/types"
	"github.com/grafana/grafana-plugin-sdk-go/backend/auth"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	pluginv3 "github.com/grafana/grafana-app-sdk/plugin/genproto/grafana/plugin/v3"
)

// authenticated reports whether the request's AuthInfo was verified, which the
// plugin SDK's server did before the request reached the service (see
// auth.RequestAuthenticator.UnaryServerInterceptor): the request's audience and
// namespaces are only checked then. A request that must be verified, but
// reached the service without an identity, is rejected.
func (a serverAuth) authenticated(ctx context.Context) (bool, error) {
	if a.verifies {
		if info, ok := types.AuthInfoFrom(ctx); !ok || info == nil {
			return false, status.Error(codes.Unauthenticated, "request was not authenticated")
		}
		return true, nil
	}
	// Serving without authentication requires ServeOpts.InsecureSkipAuthentication.
	if !a.insecure {
		return false, status.Error(codes.FailedPrecondition, "plugin has no authenticator configured")
	}
	return false, nil
}

// AuthenticateAccessToken verifies an access token, and returns ctx carrying
// the caller's AuthInfo, which keeps the token.
//
// Deprecated: the plugin SDK authenticates plugin protocol v2 and v3 requests
// itself; use auth.AuthenticateAccessToken from the grafana-plugin-sdk-go
// backend/auth package to verify a token some other way.
func AuthenticateAccessToken(ctx context.Context, authenticator authn.Authenticator, token string) (context.Context, error) {
	return auth.AuthenticateAccessToken(ctx, authenticator, token)
}

// serverAuth configures how the service wrappers check authenticated requests.
type serverAuth struct {
	// verifies is set when the plugin SDK verifies tokens (ServeOpts.Authenticator).
	verifies bool
	// pluginID, if set, is an audience covering every API group the plugin serves.
	pluginID string
	// insecure serves requests the plugin SDK did not verify (see
	// ServeOpts.InsecureSkipAuthentication).
	insecure bool
}

type authenticatedAdmissionServer struct {
	pluginv3.AdmissionServiceServer
	auth serverAuth
}

func (s *authenticatedAdmissionServer) AdmissionReview(ctx context.Context, req *pluginv3.AdmissionReviewRequest) (*pluginv3.AdmissionReviewResponse, error) {
	verified, err := s.auth.authenticated(ctx)
	if err != nil {
		return nil, err
	}
	if verified {
		if err := s.auth.checkAudience(ctx, req.GetKind().GetGroup()); err != nil {
			return nil, err
		}
		if err := checkObjectNamespaces(ctx, req.GetObjectBytes(), req.GetOldObjectBytes()); err != nil {
			return nil, err
		}
	}
	return s.AdmissionServiceServer.AdmissionReview(ctx, req)
}

type authenticatedConversionServer struct {
	pluginv3.ConversionServiceServer
	auth serverAuth
}

func (s *authenticatedConversionServer) ConvertObjects(ctx context.Context, req *pluginv3.ConvertObjectsRequest) (*pluginv3.ConvertObjectsResponse, error) {
	verified, err := s.auth.authenticated(ctx)
	if err != nil {
		return nil, err
	}
	if verified {
		target := req.GetTarget()
		if err := s.auth.checkAudience(ctx, target.GetGroup()); err != nil {
			return nil, err
		}

		if err := checkNamespace(ctx, req.GetNamespace()); err != nil {
			return nil, err
		}
	}
	return s.ConversionServiceServer.ConvertObjects(ctx, req)
}

type authenticatedRouteServer struct {
	pluginv3.RouteServiceServer
	auth serverAuth
}

func (s *authenticatedRouteServer) CallRoute(req *pluginv3.CallRouteRequest, stream grpc.ServerStreamingServer[pluginv3.CallRouteResponse]) error {
	ctx := stream.Context()
	verified, err := s.auth.authenticated(ctx)
	if err != nil {
		return err
	}
	if verified {
		if err := s.auth.checkAudience(ctx, req.GetGroup()); err != nil {
			return err
		}
		if err := checkRouteNamespace(ctx, req.GetNamespace()); err != nil {
			return err
		}
	}
	return s.RouteServiceServer.CallRoute(req, stream)
}

// The verifier checks the service's configured audiences. Also bind the token
// to this particular request, as a plugin may serve more than one API group:
// the token must name the request's group, or the plugin, which covers all of them.
func (a serverAuth) checkAudience(ctx context.Context, group string) error {
	if group == "" {
		return status.Error(codes.InvalidArgument, "API group is required")
	}
	info, _ := types.AuthInfoFrom(ctx)
	audience := info.GetAudience()
	if !slices.Contains(audience, group) && (a.pluginID == "" || !slices.Contains(audience, a.pluginID)) {
		return status.Error(codes.PermissionDenied, "access token does not cover the requested API group")
	}
	return nil
}

// checkNamespace requires the token to cover namespace. Cluster-scoped
// requests, with an empty namespace, need a wildcard token.
func checkNamespace(ctx context.Context, namespace string) error {
	info, _ := types.AuthInfoFrom(ctx)
	if !types.NamespaceMatches(info.GetNamespace(), namespace) {
		return status.Error(codes.PermissionDenied, "access token does not cover the requested namespace")
	}
	return nil
}

// checkRouteNamespace also lets a tenant's token call a cluster-scoped route.
// The client scopes such calls to the caller's namespace, and the handler acts
// with that identity, so the token grants no access beyond its own tenant.
// Cluster-scoped objects in admission and conversion still need a wildcard
// token, as those requests carry the objects themselves.
func checkRouteNamespace(ctx context.Context, namespace string) error {
	if namespace == "" {
		info, _ := types.AuthInfoFrom(ctx)
		if info.GetNamespace() == "" {
			return status.Error(codes.PermissionDenied, "access token does not cover the requested namespace")
		}
		return nil
	}
	return checkNamespace(ctx, namespace)
}

// checkObjectNamespaces requires at least one object and a token that covers
// the namespace of every object, as these are what the handler acts on.
func checkObjectNamespaces(ctx context.Context, raws ...[]byte) error {
	namespaces, err := objectNamespaces(raws...)
	if err != nil {
		return status.Error(codes.InvalidArgument, err.Error())
	}
	if len(namespaces) == 0 {
		return status.Error(codes.InvalidArgument, "request carries no objects")
	}
	for _, ns := range namespaces {
		if err := checkNamespace(ctx, ns); err != nil {
			return err
		}
	}
	return nil
}

// objectNamespaces returns metadata.namespace of each non-empty JSON object.
func objectNamespaces(raws ...[]byte) ([]string, error) {
	namespaces := make([]string, 0, len(raws))
	for _, raw := range raws {
		if len(raw) == 0 {
			continue
		}
		var obj struct {
			Metadata struct {
				Namespace string `json:"namespace"`
			} `json:"metadata"`
		}
		if err := json.Unmarshal(raw, &obj); err != nil {
			return nil, fmt.Errorf("decode object metadata: %w", err)
		}
		namespaces = append(namespaces, obj.Metadata.Namespace)
	}
	return namespaces, nil
}
