package k8s

import (
	"context"
	"fmt"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime/schema"

	"github.com/grafana/grafana-app-sdk/app"
	"github.com/grafana/grafana-app-sdk/resource"
)

type webhookCapabilities struct {
	conversion bool
	mutation   bool
	validation bool
}

// ManifestRequiresWebhooks reports whether the given app manifest declares any admission,
// conversion, or custom route capabilities, i.e. whether the app requires a WebhookHandler
// to be configured for it.
func ManifestRequiresWebhooks(manifestData *app.ManifestData) bool {
	for _, version := range manifestData.Versions {
		for _, kind := range version.Kinds {
			if kind.Conversion {
				return true
			}
			if kind.Admission != nil && (kind.Admission.SupportsAnyMutation() || kind.Admission.SupportsAnyValidation()) {
				return true
			}
		}
	}
	return manifestHasCustomRoutes(manifestData)
}

// ConfigureWebhookHandler inspects the app's manifest for admission, conversion, and custom
// route capabilities, and registers the corresponding controllers, converters, and custom
// route handler on handler.
func ConfigureWebhookHandler(handler *WebhookHandler, a app.App, manifestData *app.ManifestData) error {
	vkCapabilities := make(map[string]webhookCapabilities)
	for _, version := range manifestData.Versions {
		for _, kind := range version.Kinds {
			if kind.Admission == nil {
				if kind.Conversion {
					vkCapabilities[fmt.Sprintf("%s/%s", kind.Kind, version.Name)] = webhookCapabilities{
						conversion: kind.Conversion,
					}
				}
				continue
			}
			vkCapabilities[fmt.Sprintf("%s/%s", kind.Kind, version.Name)] = webhookCapabilities{
				conversion: kind.Conversion,
				mutation:   kind.Admission.SupportsAnyMutation(),
				validation: kind.Admission.SupportsAnyValidation(),
			}
		}
	}
	for _, kind := range a.ManagedKinds() {
		c, ok := vkCapabilities[fmt.Sprintf("%s/%s", kind.Kind(), kind.Version())]
		if !ok {
			continue
		}
		if c.validation {
			handler.AddValidatingAdmissionController(&resource.SimpleValidatingAdmissionController{
				ValidateFunc: func(ctx context.Context, request *resource.AdmissionRequest) error {
					return a.Validate(ctx, translateAdmissionRequest(request))
				},
			}, kind)
		}
		if c.mutation {
			handler.AddMutatingAdmissionController(&resource.SimpleMutatingAdmissionController{
				MutateFunc: func(ctx context.Context, request *resource.AdmissionRequest) (*resource.MutatingResponse, error) {
					resp, err := a.Mutate(ctx, translateAdmissionRequest(request))
					return translateMutatingResponse(resp), err
				},
			}, kind)
		}
		if c.conversion {
			handler.AddConverter(toWebhookConverter(a), metav1.GroupKind{
				Group: kind.Group(),
				Kind:  kind.Kind(),
			})
		}
	}
	if manifestHasCustomRoutes(manifestData) {
		crh, err := NewCustomRouteHandler(CustomRouteHandlerConfig{
			Caller:   a,
			Manifest: *manifestData,
		})
		if err != nil {
			return fmt.Errorf("failed to create custom route handler: %w", err)
		}
		handler.SetCustomRouteHandler(crh)
	}
	return nil
}

func translateAdmissionRequest(request *resource.AdmissionRequest) *app.AdmissionRequest {
	if request == nil {
		return nil
	}
	// app.AdmissionRequest is of type resource.AdmissionRequest
	req := app.AdmissionRequest(*request)
	return &req
}

func translateMutatingResponse(response *app.MutatingResponse) *resource.MutatingResponse {
	if response == nil {
		return nil
	}
	// app.MutatingResponse is of type resource.MutatingResponse
	resp := resource.MutatingResponse(*response)
	return &resp
}

func toWebhookConverter(a app.App) Converter {
	return &simpleConverter{
		convertFunc: func(obj RawKind, targetAPIVersion string) ([]byte, error) {
			converted, err := a.Convert(context.Background(), app.ConversionRequest{
				SourceGVK: schema.FromAPIVersionAndKind(obj.APIVersion, obj.Kind),
				TargetGVK: schema.FromAPIVersionAndKind(targetAPIVersion, obj.Kind),
				Raw: app.RawObject{
					Raw:      obj.Raw,
					Encoding: resource.KindEncodingJSON,
				},
			})
			if err != nil {
				return nil, err
			}
			return converted.Raw, nil
		},
	}
}

type simpleConverter struct {
	convertFunc func(obj RawKind, targetAPIVersion string) ([]byte, error)
}

func (s *simpleConverter) Convert(obj RawKind, targetAPIVersion string) ([]byte, error) {
	return s.convertFunc(obj, targetAPIVersion)
}

// manifestHasCustomRoutes reports whether the manifest declares any kind subresource routes
// or any version-level routes.
func manifestHasCustomRoutes(md *app.ManifestData) bool {
	for _, version := range md.Versions {
		if len(version.Routes.Namespaced) > 0 || len(version.Routes.Cluster) > 0 {
			return true
		}
		for _, kind := range version.Kinds {
			if len(kind.Routes) > 0 {
				return true
			}
		}
	}
	return false
}
