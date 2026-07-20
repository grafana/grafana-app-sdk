// Code generated - EDITING IS FUTILE. DO NOT EDIT.

package v1alpha2

// #CommonBackendConfig is the common shape for every backend target: where to reach it and
// how to trust it. The three concrete configs embed it. GroupVersions are NOT
// carried here — they are derived from the same-name AppManifest for all modes.
// +k8s:openapi-gen=true
type RouteBackendCommonBackendConfig struct {
	Url string                 `json:"url"`
	Tls RouteBackendTLSOptions `json:"tls"`
}

// NewRouteBackendCommonBackendConfig creates a new RouteBackendCommonBackendConfig object.
func NewRouteBackendCommonBackendConfig() *RouteBackendCommonBackendConfig {
	return &RouteBackendCommonBackendConfig{
		Tls: *NewRouteBackendTLSOptions(),
	}
}

// OpenAPIModelName returns the OpenAPI model name for RouteBackendCommonBackendConfig.
func (RouteBackendCommonBackendConfig) OpenAPIModelName() string {
	return "com.github.grafana.grafana-app-sdk.app.appmanifest.v1alpha2.RouteBackendCommonBackendConfig"
}

// +k8s:openapi-gen=true
type RouteBackendTLSOptions struct {
	CaData        *string `json:"caData,omitempty"`
	SkipTLSVerify bool    `json:"skipTLSVerify"`
}

// NewRouteBackendTLSOptions creates a new RouteBackendTLSOptions object.
func NewRouteBackendTLSOptions() *RouteBackendTLSOptions {
	return &RouteBackendTLSOptions{
		SkipTLSVerify: false,
	}
}

// OpenAPIModelName returns the OpenAPI model name for RouteBackendTLSOptions.
func (RouteBackendTLSOptions) OpenAPIModelName() string {
	return "com.github.grafana.grafana-app-sdk.app.appmanifest.v1alpha2.RouteBackendTLSOptions"
}

// +k8s:openapi-gen=true
type RouteBackendOperatorConfig struct {
	Webhooks *RouteBackendOperatorWebhookOptions `json:"webhooks,omitempty"`
	Url      string                              `json:"url"`
	Tls      RouteBackendTLSOptions              `json:"tls"`
}

// NewRouteBackendOperatorConfig creates a new RouteBackendOperatorConfig object.
func NewRouteBackendOperatorConfig() *RouteBackendOperatorConfig {
	return &RouteBackendOperatorConfig{
		Tls: *NewRouteBackendTLSOptions(),
	}
}

// OpenAPIModelName returns the OpenAPI model name for RouteBackendOperatorConfig.
func (RouteBackendOperatorConfig) OpenAPIModelName() string {
	return "com.github.grafana.grafana-app-sdk.app.appmanifest.v1alpha2.RouteBackendOperatorConfig"
}

// +k8s:openapi-gen=true
type RouteBackendOperatorWebhookOptions struct {
	ConversionPath *string `json:"conversionPath,omitempty"`
	ValidationPath *string `json:"validationPath,omitempty"`
	MutationPath   *string `json:"mutationPath,omitempty"`
}

// NewRouteBackendOperatorWebhookOptions creates a new RouteBackendOperatorWebhookOptions object.
func NewRouteBackendOperatorWebhookOptions() *RouteBackendOperatorWebhookOptions {
	return &RouteBackendOperatorWebhookOptions{}
}

// OpenAPIModelName returns the OpenAPI model name for RouteBackendOperatorWebhookOptions.
func (RouteBackendOperatorWebhookOptions) OpenAPIModelName() string {
	return "com.github.grafana.grafana-app-sdk.app.appmanifest.v1alpha2.RouteBackendOperatorWebhookOptions"
}

// +k8s:openapi-gen=true
type RouteBackendPluginConfig struct {
	Url string                 `json:"url"`
	Tls RouteBackendTLSOptions `json:"tls"`
}

// NewRouteBackendPluginConfig creates a new RouteBackendPluginConfig object.
func NewRouteBackendPluginConfig() *RouteBackendPluginConfig {
	return &RouteBackendPluginConfig{
		Tls: *NewRouteBackendTLSOptions(),
	}
}

// OpenAPIModelName returns the OpenAPI model name for RouteBackendPluginConfig.
func (RouteBackendPluginConfig) OpenAPIModelName() string {
	return "com.github.grafana.grafana-app-sdk.app.appmanifest.v1alpha2.RouteBackendPluginConfig"
}

// +k8s:openapi-gen=true
type RouteBackendSpec struct {
	Mode RouteBackendSpecMode `json:"mode"`
	// Basic HTTP Proxy mode
	Forward *RouteBackendCommonBackendConfig `json:"forward,omitempty"`
	// HTTP based operator
	Operator *RouteBackendOperatorConfig `json:"operator,omitempty"`
	// Plugin based handler
	Plugin *RouteBackendPluginConfig `json:"plugin,omitempty"`
}

// NewRouteBackendSpec creates a new RouteBackendSpec object.
func NewRouteBackendSpec() *RouteBackendSpec {
	return &RouteBackendSpec{}
}

// OpenAPIModelName returns the OpenAPI model name for RouteBackendSpec.
func (RouteBackendSpec) OpenAPIModelName() string {
	return "com.github.grafana.grafana-app-sdk.app.appmanifest.v1alpha2.RouteBackendSpec"
}

// +k8s:openapi-gen=true
type RouteBackendSpecMode string

const (
	RouteBackendSpecModeForward  RouteBackendSpecMode = "forward"
	RouteBackendSpecModePlugin   RouteBackendSpecMode = "plugin"
	RouteBackendSpecModeOperator RouteBackendSpecMode = "operator"
)

// OpenAPIModelName returns the OpenAPI model name for RouteBackendSpecMode.
func (RouteBackendSpecMode) OpenAPIModelName() string {
	return "com.github.grafana.grafana-app-sdk.app.appmanifest.v1alpha2.RouteBackendSpecMode"
}
