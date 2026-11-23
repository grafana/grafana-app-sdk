package simple

import (
	"context"

	"github.com/grafana/grafana-app-sdk/app"
	"github.com/grafana/grafana-app-sdk/logging"
)

func admissionLoggerFromContext(ctx context.Context, req *app.AdmissionRequest) logging.Logger {
	if req == nil {
		return logging.FromContext(ctx)
	}
	return logging.FromContext(ctx).With("group", req.Group, "version", req.Version, "kind", req.Kind)
}

func conversionLoggerFromContext(ctx context.Context, req app.ConversionRequest) logging.Logger {
	return logging.FromContext(ctx).
		With("sourceGroup", req.SourceGVK.Group,
			"sourceVersion", req.SourceGVK.Version,
			"sourceKind", req.SourceGVK.Kind,
			"targetGroup", req.TargetGVK.Group,
			"targetVersion", req.TargetGVK.Version,
			"targetKind", req.TargetGVK.Kind,
		)
}

func handleCustomRouteWithLogging(ctx context.Context, handler AppCustomRouteHandler, writer app.CustomRouteResponseWriter, req *app.CustomRouteRequest) error {
	logger := logging.FromContext(ctx)
	if req != nil {
		logger = logger.With(
			"method", req.Method,
			"path", req.Path,
			"group", req.ResourceIdentifier.Group,
			"version", req.ResourceIdentifier.Version,
			"kind", req.ResourceIdentifier.Kind,
		)
	}
	ctx = logging.Context(ctx, logger)
	err := handler(ctx, writer, req)
	if err != nil {
		logger.With("error", err).Error("custom route handler failed")
		return err
	}
	logger.Info("custom route handler succeeded")
	return nil
}
