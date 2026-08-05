package simple

import (
	"context"

	apierrors "k8s.io/apimachinery/pkg/api/errors"

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
		if cast, ok := err.(apierrors.APIStatus); ok {
			if cast.Status().Code < 500 {
				logger.Info("custom route handler returned non-5xx status error", "code", cast.Status().Code, "message", cast.Status().Message)
				return err
			}
			logger.Error("custom route handler returned 5xx status error", "code", cast.Status().Code, "message", cast.Status().Message)
			return err
		}
		logger.With("error", err).Error("custom route handler failed")
		return err
	}
	logger.Info("custom route handler succeeded")
	return nil
}
