package plugin

import (
	"context"
	"encoding/json"
	"strconv"

	"github.com/grafana/grafana-plugin-sdk-go/backend"

	"github.com/grafana/grafana-app-sdk/resource"
)

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

// newSettingsMiddleware gives CallResource handlers the app instance settings
// as resource.RouteRequestInfo, as v3 custom route handlers get theirs. The
// plugin SDK has authenticated the call by then (see backend.Manage), so the
// handler's context carries the caller's AuthInfo when the call had a token.
func newSettingsMiddleware() backend.HandlerMiddleware {
	return backend.HandlerMiddlewareFunc(func(next backend.Handler) backend.Handler {
		return &settingsHandler{BaseHandler: backend.NewBaseHandler(next)}
	})
}

type settingsHandler struct {
	backend.BaseHandler
}

func (h *settingsHandler) CallResource(ctx context.Context, req *backend.CallResourceRequest, sender backend.CallResourceResponseSender) error {
	if req != nil && resource.RouteRequestInfoFrom(ctx) == nil {
		if info := settingsRouteRequestInfo(req.PluginContext); info != nil {
			ctx = resource.WithRouteRequestInfo(ctx, info)
		}
	}
	return h.BaseHandler.CallResource(ctx, req, sender)
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
