package secure

import (
	"context"

	"github.com/grafana/grafana-plugin-sdk-go/backend/log"
)

// GetSecureSettings retrieves SecureSettings from the provided context.Context.
func GetData(ctx context.Context) Data {
	// If this function is used, we can assume two things:
	// 1. The middleware be used
	// 2. If the middleware had failed, the request trace would have been terminated with an HTTP 500 error
	// That said, if this code is reached, SecureSettings exists in the context, under the secureSettingsCtxKey{} key
	val := ctx.Value(secureSettingsCtxKey{})
	if val == nil { // Nil check to avoid a crash
		log.DefaultLogger.Warn("No secure settings in context")
		return Data{}
	}
	settings, ok := val.(Data)
	if !ok {
		log.DefaultLogger.Warn("Secure settings in context is not of type SecureSettings")
		return Data{}
	}
	return settings
}
