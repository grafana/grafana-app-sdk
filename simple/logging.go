package simple

import (
	"context"

	"github.com/grafana/grafana-app-sdk/app"
	"github.com/grafana/grafana-app-sdk/logging"
)

func admissionLoggerFromContext(ctx context.Context, req *app.AdmissionRequest) logging.Logger {
	return logging.FromContext(ctx).With("group", req.Group).With("version", req.Version).With("kind", req.Kind)
}

func conversionLoggerFromContext(ctx context.Context, req app.ConversionRequest) logging.Logger {
	return logging.FromContext(ctx).With("sourceGroup", req.SourceGVK.Group).With("sourceVersion", req.SourceGVK.Version).With("sourceKind", req.SourceGVK.Kind).
		With("targetGroup", req.TargetGVK.Group).With("targetVersion", req.TargetGVK.Version).With("targetKind", req.TargetGVK.Kind)
}
