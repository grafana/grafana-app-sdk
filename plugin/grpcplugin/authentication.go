package grpcplugin

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"slices"

	"github.com/go-jose/go-jose/v4"
	"github.com/go-jose/go-jose/v4/jwt"
	"github.com/grafana/authlib/authn"
	"github.com/grafana/authlib/types"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/metadata"
	"google.golang.org/grpc/status"

	pluginv3 "github.com/grafana/grafana-app-sdk/plugin/genproto/grafana/plugin/v3"
)

func authenticate(ctx context.Context, authenticator authn.Authenticator) (context.Context, error) {
	// Serving without authentication requires ServeOpts.InsecureSkipAuthentication.
	if authenticator == nil {
		return nil, status.Error(codes.FailedPrecondition, "plugin has no authenticator configured")
	}
	md, _ := metadata.FromIncomingContext(ctx)
	// Delegation has one source of identity: the exchanged access token.
	// Reject ambiguous credentials before invoking even a custom authenticator.
	tokens := md.Get("x-access-token")
	if len(tokens) != 1 || tokens[0] == "" || len(md.Get("x-id-token")) != 0 {
		return nil, status.Error(codes.Unauthenticated, "a single access token and no separate ID token are required")
	}
	info, err := authenticator.Authenticate(ctx, authn.NewGRPCTokenProvider(md))
	if err != nil {
		if isInvalidTokenErr(err) {
			return nil, status.Error(codes.Unauthenticated, "invalid access token")
		}
		return nil, status.Error(codes.Internal, "authentication failed")
	}
	if info == nil {
		return nil, status.Error(codes.Unauthenticated, "authenticator returned no identity")
	}
	// authlib's AuthInfo does not retain the raw access token. Keep the token
	// only after successful authentication so downstream clients can exchange
	// it without losing the caller's identity or delegation chain.
	return types.WithAuthInfo(ctx, &authenticatedAuthInfo{AuthInfo: info, accessToken: tokens[0]}), nil
}

// invalidTokenErrs are errors about the token itself that authlib passes on
// from go-jose without marking them as unauthenticated.
var invalidTokenErrs = []error{
	jose.ErrCryptoFailure, // Signature mismatch, including a key of the wrong type.
	jwt.ErrNotValidYet,
	jwt.ErrIssuedInTheFuture,
	jwt.ErrExpired,
	jwt.ErrInvalidAudience,
	jwt.ErrInvalidIssuer,
	jwt.ErrInvalidSubject,
	jwt.ErrInvalidID,
	jwt.ErrInvalidClaims,
	jwt.ErrInvalidContentType,
	jwt.ErrUnmarshalAudience,
	jwt.ErrUnmarshalNumericDate,
}

// isInvalidTokenErr reports whether err is the caller's fault, rather than a
// failure of the plugin such as fetching signing keys.
func isInvalidTokenErr(err error) bool {
	if authn.IsUnauthenticatedErr(err) {
		return true
	}
	for _, target := range invalidTokenErrs {
		if errors.Is(err, target) {
			return true
		}
	}
	return false
}

// serverAuth configures how the service wrappers authenticate requests.
type serverAuth struct {
	authenticator authn.Authenticator
	// pluginID, if set, is an audience covering every API group the plugin serves.
	pluginID string
}

type authenticatedAdmissionServer struct {
	pluginv3.AdmissionServiceServer
	auth serverAuth
}

func (s *authenticatedAdmissionServer) AdmissionReview(ctx context.Context, req *pluginv3.AdmissionReviewRequest) (*pluginv3.AdmissionReviewResponse, error) {
	ctx, err := authenticate(ctx, s.auth.authenticator)
	if err != nil {
		return nil, err
	}
	if err := s.auth.checkAudience(ctx, req.GetKind().GetGroup()); err != nil {
		return nil, err
	}
	if err := checkObjectNamespaces(ctx, req.GetObjectBytes(), req.GetOldObjectBytes()); err != nil {
		return nil, err
	}
	return s.AdmissionServiceServer.AdmissionReview(ctx, req)
}

type authenticatedConversionServer struct {
	pluginv3.ConversionServiceServer
	auth serverAuth
}

func (s *authenticatedConversionServer) ConvertObjects(ctx context.Context, req *pluginv3.ConvertObjectsRequest) (*pluginv3.ConvertObjectsResponse, error) {
	ctx, err := authenticate(ctx, s.auth.authenticator)
	if err != nil {
		return nil, err
	}
	group, err := conversionGroup(req)
	if err != nil {
		return nil, status.Error(codes.InvalidArgument, err.Error())
	}
	if err := s.auth.checkAudience(ctx, group); err != nil {
		return nil, err
	}
	raws := make([][]byte, 0, len(req.GetObjects()))
	for _, obj := range req.GetObjects() {
		raws = append(raws, obj.GetRaw())
	}
	if err := checkObjectNamespaces(ctx, raws...); err != nil {
		return nil, err
	}
	return s.ConversionServiceServer.ConvertObjects(ctx, req)
}

type authenticatedRouteServer struct {
	pluginv3.RouteServiceServer
	auth serverAuth
}

func (s *authenticatedRouteServer) CallRoute(req *pluginv3.CallRouteRequest, stream grpc.ServerStreamingServer[pluginv3.CallRouteResponse]) error {
	ctx, err := authenticate(stream.Context(), s.auth.authenticator)
	if err != nil {
		return err
	}
	if err := s.auth.checkAudience(ctx, req.GetGroup()); err != nil {
		return err
	}
	if err := checkNamespace(ctx, req.GetNamespace()); err != nil {
		return err
	}
	return s.RouteServiceServer.CallRoute(req, &authenticatedRouteStream{stream, ctx})
}

type authenticatedRouteStream struct {
	grpc.ServerStreamingServer[pluginv3.CallRouteResponse]
	ctx context.Context
}

func (s *authenticatedRouteStream) Context() context.Context {
	return s.ctx
}

// authenticatedAuthInfo preserves the verified token for onward delegation.
// Its formatting methods omit the token, so logging the identity is safe.
type authenticatedAuthInfo struct {
	types.AuthInfo
	accessToken string
}

func (a *authenticatedAuthInfo) GetAccessToken() string {
	return a.accessToken
}

func (a *authenticatedAuthInfo) String() string {
	return fmt.Sprintf("AuthInfo{subject: %q, namespace: %q}", a.GetSubject(), a.GetNamespace())
}

func (a *authenticatedAuthInfo) GoString() string {
	return a.String()
}

func (a *authenticatedAuthInfo) LogValue() slog.Value {
	return slog.GroupValue(slog.String("subject", a.GetSubject()), slog.String("namespace", a.GetNamespace()))
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
