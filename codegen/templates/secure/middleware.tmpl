package secure

import (
	"context"
	"fmt"

	"github.com/grafana/grafana-plugin-sdk-go/backend"
	"github.com/grafana/grafana-plugin-sdk-go/backend/log"
	"github.com/grafana/grafana-app-sdk/plugin"
	"github.com/grafana/grafana-app-sdk/plugin/router"
)

type secureSettingsCtxKey struct{}

// Middleware is a router middleware that extracts the decrypted secureJsonData, validates them and injects them
// into the request context.
func Middleware(handler router.HandlerFunc) router.HandlerFunc {
	return func(ctx context.Context, req *backend.CallResourceRequest, sender backend.CallResourceResponseSender) {
		secureJSON := Data{}
		if err := secureJSON.ParseRaw(req.PluginContext.AppInstanceSettings.DecryptedSecureJSONData); err != nil {
			send(sender, plugin.InternalError(fmt.Errorf("misconfigured secureJsonData: %w", err)))
			return
		}
		handler(context.WithValue(ctx, secureSettingsCtxKey{}, secureJSON), req, sender)
	}
}

func send(sender backend.CallResourceResponseSender, res *backend.CallResourceResponse) {
	err := sender.Send(res)
	if err != nil {
		log.DefaultLogger.Error("Error sending response", "err", err.Error())
	}
}
