package plugin

import (
	"context"
	"encoding/json"
	"strconv"
	"strings"

	"github.com/grafana/authlib/authn"
	"github.com/grafana/authlib/types"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	"github.com/grafana/grafana-plugin-sdk-go/backend"

	"github.com/grafana/grafana-app-sdk/plugin/grpcplugin"
	"github.com/grafana/grafana-app-sdk/resource"
)

// v2AccessTokenHeader is the header in which Grafana sends a plugin protocol
// v2 call the caller's access token, exchanged for the plugin.
const v2AccessTokenHeader = "X-Access-Token"

// Grafana serves an app plugin's settings as a single object, under the plugin
// ID as API group:
//
//	/apis/<plugin ID>/v0alpha1/namespaces/<namespace>/app/instance
//
// Its health and resources subresources reach the plugin as plugin protocol v2
// CheckHealth and CallResource calls.
const (
	settingsVersion  = "v0alpha1"
	settingsKind     = "Settings"
	settingsResource = "app"
	settingsName     = "instance"
)

// v2Auth authenticates plugin protocol v2 calls the way grpcplugin serves v3
// ones (see grpcplugin.ServeOpts): with the same authenticator, and with
// InsecureSkipAuthentication parsing tokens without verifying them.
type v2Auth struct {
	authenticator authn.Authenticator
	// insecure is set by InsecureSkipAuthentication: tokens are parsed without
	// being verified, and calls without one are let through.
	insecure bool
}

func (cfg *runConfig) v2Auth() v2Auth {
	authenticator := cfg.authenticator
	if authenticator == nil && cfg.insecureSkipAuthentication {
		authenticator = authn.NewAccessTokenAuthenticator(authn.NewUnsafeAccessTokenVerifier(authn.VerifierConfig{}))
	}
	return v2Auth{authenticator: authenticator, insecure: cfg.insecureSkipAuthentication}
}

// newV2Middleware authenticates every call that carries request headers
// (QueryData, QueryChunkedData, CheckHealth, CallResource, and the stream
// calls), and gives CallResource handlers the app settings as
// resource.RouteRequestInfo, as v3 custom route handlers get theirs.
// CollectMetrics has no headers to carry a token, and passes through unchanged.
//
// Each call must carry the caller's access token (X-Access-Token), which is
// verified; the handler's context then carries the caller's AuthInfo, so
// clients made with the app's KubeConfig act on behalf of the caller. A call
// without a token, or with one that does not verify, fails, rather than leaving
// the handler to act as the plugin. Only with InsecureSkipAuthentication is a
// call without a token let through, carrying no identity.
func newV2Middleware(auth v2Auth) backend.HandlerMiddleware {
	return backend.HandlerMiddlewareFunc(func(next backend.Handler) backend.Handler {
		return &v2Handler{BaseHandler: backend.NewBaseHandler(next), auth: auth}
	})
}

type v2Handler struct {
	backend.BaseHandler
	auth v2Auth
}

func (h *v2Handler) QueryData(ctx context.Context, req *backend.QueryDataRequest) (*backend.QueryDataResponse, error) {
	if req == nil {
		return h.BaseHandler.QueryData(ctx, req)
	}
	ctx, err := h.auth.authenticate(ctx, req.PluginContext, req.GetHTTPHeader(v2AccessTokenHeader))
	if err != nil {
		return nil, err
	}
	return h.BaseHandler.QueryData(ctx, req)
}

func (h *v2Handler) QueryChunkedData(ctx context.Context, req *backend.QueryChunkedDataRequest, w backend.ChunkedDataWriter) error {
	if req == nil {
		return h.BaseHandler.QueryChunkedData(ctx, req, w)
	}
	ctx, err := h.auth.authenticate(ctx, req.PluginContext, req.GetHTTPHeader(v2AccessTokenHeader))
	if err != nil {
		return err
	}
	return h.BaseHandler.QueryChunkedData(ctx, req, w)
}

func (h *v2Handler) SubscribeStream(ctx context.Context, req *backend.SubscribeStreamRequest) (*backend.SubscribeStreamResponse, error) {
	if req == nil {
		return h.BaseHandler.SubscribeStream(ctx, req)
	}
	ctx, err := h.auth.authenticate(ctx, req.PluginContext, req.GetHTTPHeader(v2AccessTokenHeader))
	if err != nil {
		return nil, err
	}
	return h.BaseHandler.SubscribeStream(ctx, req)
}

func (h *v2Handler) PublishStream(ctx context.Context, req *backend.PublishStreamRequest) (*backend.PublishStreamResponse, error) {
	if req == nil {
		return h.BaseHandler.PublishStream(ctx, req)
	}
	ctx, err := h.auth.authenticate(ctx, req.PluginContext, req.GetHTTPHeader(v2AccessTokenHeader))
	if err != nil {
		return nil, err
	}
	return h.BaseHandler.PublishStream(ctx, req)
}

func (h *v2Handler) RunStream(ctx context.Context, req *backend.RunStreamRequest, sender *backend.StreamSender) error {
	if req == nil {
		return h.BaseHandler.RunStream(ctx, req, sender)
	}
	ctx, err := h.auth.authenticate(ctx, req.PluginContext, req.GetHTTPHeader(v2AccessTokenHeader))
	if err != nil {
		return err
	}
	return h.BaseHandler.RunStream(ctx, req, sender)
}

func (h *v2Handler) CheckHealth(ctx context.Context, req *backend.CheckHealthRequest) (*backend.CheckHealthResult, error) {
	if req == nil {
		return h.BaseHandler.CheckHealth(ctx, req)
	}
	ctx, err := h.auth.authenticate(ctx, req.PluginContext, req.GetHTTPHeader(v2AccessTokenHeader))
	if err != nil {
		return nil, err
	}
	return h.BaseHandler.CheckHealth(ctx, req)
}

func (h *v2Handler) CallResource(ctx context.Context, req *backend.CallResourceRequest, sender backend.CallResourceResponseSender) error {
	if req == nil {
		return h.BaseHandler.CallResource(ctx, req, sender)
	}
	ctx, err := h.auth.authenticate(ctx, req.PluginContext, req.GetHTTPHeader(v2AccessTokenHeader))
	if err != nil {
		return err
	}
	if resource.RouteRequestInfoFrom(ctx) == nil {
		if info := settingsRouteRequestInfo(req.PluginContext); info != nil {
			ctx = resource.WithRouteRequestInfo(ctx, info)
		}
	}
	return h.BaseHandler.CallResource(ctx, req, sender)
}

// authenticate returns ctx with the caller's AuthInfo. A call without a token
// is rejected, unless InsecureSkipAuthentication is set, in which case it
// carries no identity. A verified token must cover the plugin context's
// namespace.
func (a v2Auth) authenticate(ctx context.Context, pluginCtx backend.PluginContext, token string) (context.Context, error) {
	token = strings.TrimSpace(token)
	if len(token) > len("Bearer ") && strings.EqualFold(token[:len("Bearer ")], "Bearer ") {
		token = strings.TrimSpace(token[len("Bearer "):])
	}
	if token == "" {
		if a.insecure {
			return ctx, nil
		}
		return nil, status.Error(codes.Unauthenticated, "an access token ("+v2AccessTokenHeader+") is required")
	}
	if a.authenticator == nil {
		// The token cannot be verified, and must not be ignored: its handler
		// would act as the plugin rather than as the caller.
		return nil, status.Error(codes.Unauthenticated, "plugin has no authenticator configured to verify the access token; set "+EnvVarGrafanaJWKSURL+" or "+EnvVarGrafanaJWKS)
	}
	ctx, err := grpcplugin.AuthenticateAccessToken(ctx, a.authenticator, token)
	if err != nil {
		return nil, err
	}
	if !a.insecure {
		info, _ := types.AuthInfoFrom(ctx)
		if pluginCtx.Namespace == "" {
			if info.GetNamespace() == "" {
				return nil, status.Error(codes.PermissionDenied, "access token does not cover the requested namespace")
			}
		} else if !types.NamespaceMatches(info.GetNamespace(), pluginCtx.Namespace) {
			return nil, status.Error(codes.PermissionDenied, "access token does not cover the requested namespace")
		}
	}
	return ctx, nil
}

// settingsRouteRequestInfo describes the app instance settings a resource
// call is for: the settings object, under the plugin ID as API group, with its
// jsonData as the parent's spec and its decrypted secure values. It is nil for
// a call without app instance settings, such as one for a data source.
func settingsRouteRequestInfo(pluginCtx backend.PluginContext) *resource.RouteRequestInfo {
	settings := pluginCtx.AppInstanceSettings
	if settings == nil || pluginCtx.PluginID == "" {
		return nil
	}
	version := pluginCtx.APIVersion
	if version == "" {
		version = settings.APIVersion
	}
	if version == "" {
		version = settingsVersion
	}
	info := &resource.RouteRequestInfo{
		Namespace: pluginCtx.Namespace,
		Name:      settingsName,
		Group:     pluginCtx.PluginID,
		Version:   version,
		Kind:      settingsKind,
		Plural:    settingsResource,
	}

	// Settings carry no resource version; their update time changes with them.
	if !settings.Updated.IsZero() {
		info.ResourceVersion = strconv.FormatInt(settings.Updated.UnixMilli(), 10)
	}

	parent := map[string]any{
		"apiVersion": pluginCtx.PluginID + "/" + version,
		"kind":       settingsKind,
		"metadata":   map[string]any{"name": settingsName, "namespace": pluginCtx.Namespace},
	}
	if len(settings.JSONData) > 0 && json.Valid(settings.JSONData) {
		parent["spec"] = map[string]any{"jsonData": settings.JSONData}
	}
	if raw, err := json.Marshal(parent); err == nil {
		info.Parent = raw
	}

	if len(settings.DecryptedSecureJSONData) > 0 {
		info.DecryptedSecureValues = make(resource.DecryptedSecureValues, len(settings.DecryptedSecureJSONData))
		for k, v := range settings.DecryptedSecureJSONData {
			info.DecryptedSecureValues[k] = resource.RawSecureValue(v)
		}
	}
	return info
}
