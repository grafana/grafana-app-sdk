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
	insecure      bool
}

func (cfg *runConfig) v2Auth() v2Auth {
	authenticator := cfg.authenticator
	if authenticator == nil && cfg.insecureSkipAuthentication {
		authenticator = authn.NewAccessTokenAuthenticator(authn.NewUnsafeAccessTokenVerifier(authn.VerifierConfig{}))
	}
	return v2Auth{authenticator: authenticator, insecure: cfg.insecureSkipAuthentication && authenticator != nil}
}

// newV2Middleware authenticates the CheckHealth and CallResource calls Grafana
// makes for an app plugin's settings subresources, and gives CallResource
// handlers the settings as resource.RouteRequestInfo, as v3 custom route
// handlers get theirs. Other calls pass through unchanged.
//
// When Grafana sends the caller's access token (X-Access-Token), it is
// verified, and the handler's context carries the caller's AuthInfo, so
// clients made with the app's KubeConfig act on behalf of the caller. A token
// that does not verify fails the call, rather than leaving the handler to act
// as the plugin. A call without a token, as Grafana makes when it does not
// exchange one, carries no identity, and its requests act as the plugin.
func newV2Middleware(auth v2Auth) backend.HandlerMiddleware {
	return backend.HandlerMiddlewareFunc(func(next backend.Handler) backend.Handler {
		return &v2Handler{BaseHandler: backend.NewBaseHandler(next), auth: auth}
	})
}

type v2Handler struct {
	backend.BaseHandler
	auth v2Auth
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

// authenticate returns ctx with the caller's AuthInfo when token is set. A
// verified token must cover the plugin context's namespace.
func (a v2Auth) authenticate(ctx context.Context, pluginCtx backend.PluginContext, token string) (context.Context, error) {
	token = strings.TrimPrefix(token, "Bearer ")
	if token == "" {
		return ctx, nil
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
		FullIdentifier: resource.FullIdentifier{
			Namespace: pluginCtx.Namespace,
			Name:      settingsName,
			Group:     pluginCtx.PluginID,
			Version:   version,
			Kind:      settingsKind,
			Plural:    settingsResource,
		},
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
