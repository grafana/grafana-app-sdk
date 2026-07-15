// Code generated - EDITING IS FUTILE. DO NOT EDIT.

package v1alpha3

// +k8s:openapi-gen=true
type RuntimeConfigAPIServerConfig struct {
	Url string                  `json:"url"`
	Tls RuntimeConfigTLSOptions `json:"tls"`
	// groupVersions this full apiserver serves. Enough to statically assemble
	// the /openapi/v3 root directory; leaf specs are proxied on demand.
	GroupVersions []RuntimeConfigGroupVersion `json:"groupVersions"`
}

// NewRuntimeConfigAPIServerConfig creates a new RuntimeConfigAPIServerConfig object.
func NewRuntimeConfigAPIServerConfig() *RuntimeConfigAPIServerConfig {
	return &RuntimeConfigAPIServerConfig{
		Tls:           *NewRuntimeConfigTLSOptions(),
		GroupVersions: []RuntimeConfigGroupVersion{},
	}
}

// OpenAPIModelName returns the OpenAPI model name for RuntimeConfigAPIServerConfig.
func (RuntimeConfigAPIServerConfig) OpenAPIModelName() string {
	return "com.github.grafana.grafana-app-sdk.app.appmanifest.v1alpha3.RuntimeConfigAPIServerConfig"
}

// +k8s:openapi-gen=true
type RuntimeConfigTLSOptions struct {
	CaData        *string `json:"caData,omitempty"`
	SkipTLSVerify bool    `json:"skipTLSVerify"`
}

// NewRuntimeConfigTLSOptions creates a new RuntimeConfigTLSOptions object.
func NewRuntimeConfigTLSOptions() *RuntimeConfigTLSOptions {
	return &RuntimeConfigTLSOptions{
		SkipTLSVerify: false,
	}
}

// OpenAPIModelName returns the OpenAPI model name for RuntimeConfigTLSOptions.
func (RuntimeConfigTLSOptions) OpenAPIModelName() string {
	return "com.github.grafana.grafana-app-sdk.app.appmanifest.v1alpha3.RuntimeConfigTLSOptions"
}

// +k8s:openapi-gen=true
type RuntimeConfigGroupVersion struct {
	Group   string `json:"group"`
	Version string `json:"version"`
}

// NewRuntimeConfigGroupVersion creates a new RuntimeConfigGroupVersion object.
func NewRuntimeConfigGroupVersion() *RuntimeConfigGroupVersion {
	return &RuntimeConfigGroupVersion{}
}

// OpenAPIModelName returns the OpenAPI model name for RuntimeConfigGroupVersion.
func (RuntimeConfigGroupVersion) OpenAPIModelName() string {
	return "com.github.grafana.grafana-app-sdk.app.appmanifest.v1alpha3.RuntimeConfigGroupVersion"
}

// +k8s:openapi-gen=true
type RuntimeConfigOperatorConfig struct {
	Url      string                               `json:"url"`
	Tls      RuntimeConfigTLSOptions              `json:"tls"`
	Webhooks *RuntimeConfigOperatorWebhookOptions `json:"webhooks,omitempty"`
}

// NewRuntimeConfigOperatorConfig creates a new RuntimeConfigOperatorConfig object.
func NewRuntimeConfigOperatorConfig() *RuntimeConfigOperatorConfig {
	return &RuntimeConfigOperatorConfig{
		Tls: *NewRuntimeConfigTLSOptions(),
	}
}

// OpenAPIModelName returns the OpenAPI model name for RuntimeConfigOperatorConfig.
func (RuntimeConfigOperatorConfig) OpenAPIModelName() string {
	return "com.github.grafana.grafana-app-sdk.app.appmanifest.v1alpha3.RuntimeConfigOperatorConfig"
}

// +k8s:openapi-gen=true
type RuntimeConfigOperatorWebhookOptions struct {
	ConversionPath *string `json:"conversionPath,omitempty"`
	ValidationPath *string `json:"validationPath,omitempty"`
	MutationPath   *string `json:"mutationPath,omitempty"`
}

// NewRuntimeConfigOperatorWebhookOptions creates a new RuntimeConfigOperatorWebhookOptions object.
func NewRuntimeConfigOperatorWebhookOptions() *RuntimeConfigOperatorWebhookOptions {
	return &RuntimeConfigOperatorWebhookOptions{}
}

// OpenAPIModelName returns the OpenAPI model name for RuntimeConfigOperatorWebhookOptions.
func (RuntimeConfigOperatorWebhookOptions) OpenAPIModelName() string {
	return "com.github.grafana.grafana-app-sdk.app.appmanifest.v1alpha3.RuntimeConfigOperatorWebhookOptions"
}

// +k8s:openapi-gen=true
type RuntimeConfigPluginConfig struct {
	Url string                  `json:"url"`
	Tls RuntimeConfigTLSOptions `json:"tls"`
}

// NewRuntimeConfigPluginConfig creates a new RuntimeConfigPluginConfig object.
func NewRuntimeConfigPluginConfig() *RuntimeConfigPluginConfig {
	return &RuntimeConfigPluginConfig{
		Tls: *NewRuntimeConfigTLSOptions(),
	}
}

// OpenAPIModelName returns the OpenAPI model name for RuntimeConfigPluginConfig.
func (RuntimeConfigPluginConfig) OpenAPIModelName() string {
	return "com.github.grafana.grafana-app-sdk.app.appmanifest.v1alpha3.RuntimeConfigPluginConfig"
}

// +k8s:openapi-gen=true
type RuntimeConfigSpec struct {
	Mode      RuntimeConfigSpecMode         `json:"mode"`
	ApiServer *RuntimeConfigAPIServerConfig `json:"apiServer,omitempty"`
	Operator  *RuntimeConfigOperatorConfig  `json:"operator,omitempty"`
	Plugin    *RuntimeConfigPluginConfig    `json:"plugin,omitempty"`
}

// NewRuntimeConfigSpec creates a new RuntimeConfigSpec object.
func NewRuntimeConfigSpec() *RuntimeConfigSpec {
	return &RuntimeConfigSpec{}
}

// OpenAPIModelName returns the OpenAPI model name for RuntimeConfigSpec.
func (RuntimeConfigSpec) OpenAPIModelName() string {
	return "com.github.grafana.grafana-app-sdk.app.appmanifest.v1alpha3.RuntimeConfigSpec"
}

// +k8s:openapi-gen=true
type RuntimeConfigSpecMode string

const (
	RuntimeConfigSpecModeApiserver RuntimeConfigSpecMode = "apiserver"
	RuntimeConfigSpecModePlugin    RuntimeConfigSpecMode = "plugin"
	RuntimeConfigSpecModeOperator  RuntimeConfigSpecMode = "operator"
)

// OpenAPIModelName returns the OpenAPI model name for RuntimeConfigSpecMode.
func (RuntimeConfigSpecMode) OpenAPIModelName() string {
	return "com.github.grafana.grafana-app-sdk.app.appmanifest.v1alpha3.RuntimeConfigSpecMode"
}
