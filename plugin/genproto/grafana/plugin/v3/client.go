package pluginv3

import (
	context "context"

	grpc "google.golang.org/grpc"
)

// Client groups the plugin protocol v3 RPCs. Implementations handle caller authentication.
type Client interface {
	AdmissionReview(ctx context.Context, in *AdmissionReviewRequest) (*AdmissionReviewResponse, error)
	ConvertObjects(ctx context.Context, in *ConvertObjectsRequest) (*ConvertObjectsResponse, error)
	CallRoute(ctx context.Context, in *CallRouteRequest) (grpc.ServerStreamingClient[CallRouteResponse], error)
}
