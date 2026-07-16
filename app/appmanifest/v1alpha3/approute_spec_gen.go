// Code generated - EDITING IS FUTILE. DO NOT EDIT.

package v1alpha3

// +k8s:openapi-gen=true
type AppRouteAPIServerConfig struct {
	Url string             `json:"url"`
	Tls AppRouteTLSOptions `json:"tls"`
	// groupVersions this full apiserver serves. Enough to statically assemble
	// the /openapi/v3 root directory; leaf specs are proxied on demand.
	GroupVersions []AppRouteGroupVersion `json:"groupVersions"`
}

// NewAppRouteAPIServerConfig creates a new AppRouteAPIServerConfig object.
func NewAppRouteAPIServerConfig() *AppRouteAPIServerConfig {
	return &AppRouteAPIServerConfig{
		Tls:           *NewAppRouteTLSOptions(),
		GroupVersions: []AppRouteGroupVersion{},
	}
}

// OpenAPIModelName returns the OpenAPI model name for AppRouteAPIServerConfig.
func (AppRouteAPIServerConfig) OpenAPIModelName() string {
	return "com.github.grafana.grafana-app-sdk.app.appmanifest.v1alpha3.AppRouteAPIServerConfig"
}

// +k8s:openapi-gen=true
type AppRouteTLSOptions struct {
	CaData        *string `json:"caData,omitempty"`
	SkipTLSVerify bool    `json:"skipTLSVerify"`
}

// NewAppRouteTLSOptions creates a new AppRouteTLSOptions object.
func NewAppRouteTLSOptions() *AppRouteTLSOptions {
	return &AppRouteTLSOptions{
		SkipTLSVerify: false,
	}
}

// OpenAPIModelName returns the OpenAPI model name for AppRouteTLSOptions.
func (AppRouteTLSOptions) OpenAPIModelName() string {
	return "com.github.grafana.grafana-app-sdk.app.appmanifest.v1alpha3.AppRouteTLSOptions"
}

// +k8s:openapi-gen=true
type AppRouteGroupVersion struct {
	Group   string `json:"group"`
	Version string `json:"version"`
}

// NewAppRouteGroupVersion creates a new AppRouteGroupVersion object.
func NewAppRouteGroupVersion() *AppRouteGroupVersion {
	return &AppRouteGroupVersion{}
}

// OpenAPIModelName returns the OpenAPI model name for AppRouteGroupVersion.
func (AppRouteGroupVersion) OpenAPIModelName() string {
	return "com.github.grafana.grafana-app-sdk.app.appmanifest.v1alpha3.AppRouteGroupVersion"
}

// +k8s:openapi-gen=true
type AppRouteOperatorConfig struct {
	Url      string                          `json:"url"`
	Tls      AppRouteTLSOptions              `json:"tls"`
	Webhooks *AppRouteOperatorWebhookOptions `json:"webhooks,omitempty"`
}

// NewAppRouteOperatorConfig creates a new AppRouteOperatorConfig object.
func NewAppRouteOperatorConfig() *AppRouteOperatorConfig {
	return &AppRouteOperatorConfig{
		Tls: *NewAppRouteTLSOptions(),
	}
}

// OpenAPIModelName returns the OpenAPI model name for AppRouteOperatorConfig.
func (AppRouteOperatorConfig) OpenAPIModelName() string {
	return "com.github.grafana.grafana-app-sdk.app.appmanifest.v1alpha3.AppRouteOperatorConfig"
}

// +k8s:openapi-gen=true
type AppRouteOperatorWebhookOptions struct {
	ConversionPath *string `json:"conversionPath,omitempty"`
	ValidationPath *string `json:"validationPath,omitempty"`
	MutationPath   *string `json:"mutationPath,omitempty"`
}

// NewAppRouteOperatorWebhookOptions creates a new AppRouteOperatorWebhookOptions object.
func NewAppRouteOperatorWebhookOptions() *AppRouteOperatorWebhookOptions {
	return &AppRouteOperatorWebhookOptions{}
}

// OpenAPIModelName returns the OpenAPI model name for AppRouteOperatorWebhookOptions.
func (AppRouteOperatorWebhookOptions) OpenAPIModelName() string {
	return "com.github.grafana.grafana-app-sdk.app.appmanifest.v1alpha3.AppRouteOperatorWebhookOptions"
}

// +k8s:openapi-gen=true
type AppRoutePluginConfig struct {
	Url string             `json:"url"`
	Tls AppRouteTLSOptions `json:"tls"`
}

// NewAppRoutePluginConfig creates a new AppRoutePluginConfig object.
func NewAppRoutePluginConfig() *AppRoutePluginConfig {
	return &AppRoutePluginConfig{
		Tls: *NewAppRouteTLSOptions(),
	}
}

// OpenAPIModelName returns the OpenAPI model name for AppRoutePluginConfig.
func (AppRoutePluginConfig) OpenAPIModelName() string {
	return "com.github.grafana.grafana-app-sdk.app.appmanifest.v1alpha3.AppRoutePluginConfig"
}

// K8s-style tagged union: a `mode` discriminator plus optional member structs.
// A top-level CUE disjunction (oneOf) can't be code-generated into a Go struct,
// and another kind's spec can't be embedded as a typed field (the codegen can't
// resolve a multi-hop selector reference), so:
//   - purity of the union is enforced in the validating admission hook, and
//   - manifest data is referenced by name, not embedded (no duplication/drift).
//
// Modes:
//   - apiserver: only `apiServer` set. A pure proxy target — the remote apiserver
//     owns its kinds/schemas/OpenAPI; we only need where to reach it and which
//     groupVersions it serves (to assemble the /openapi/v3 root).
//   - operator/plugin: `manifestName` + the respective config set. The platform
//     serves those kinds, so the referenced AppManifest is authoritative.
//
// +k8s:openapi-gen=true
type AppRouteSpec struct {
	// mode selects which member below applies.
	Mode AppRouteSpecMode `json:"mode"`
	// apiserver mode
	ApiServer *AppRouteAPIServerConfig `json:"apiServer,omitempty"`
	// operator / plugin modes
	Operator *AppRouteOperatorConfig `json:"operator,omitempty"`
	Plugin   *AppRoutePluginConfig   `json:"plugin,omitempty"`
	// manifestName references the (cluster-scoped) AppManifest this route serves.
	// Required for operator/plugin modes; unused for apiserver mode.
	ManifestName string `json:"manifestName"`
}

// NewAppRouteSpec creates a new AppRouteSpec object.
func NewAppRouteSpec() *AppRouteSpec {
	return &AppRouteSpec{}
}

// OpenAPIModelName returns the OpenAPI model name for AppRouteSpec.
func (AppRouteSpec) OpenAPIModelName() string {
	return "com.github.grafana.grafana-app-sdk.app.appmanifest.v1alpha3.AppRouteSpec"
}

// +k8s:openapi-gen=true
type AppRouteSpecMode string

const (
	AppRouteSpecModeApiserver AppRouteSpecMode = "apiserver"
	AppRouteSpecModeOperator  AppRouteSpecMode = "operator"
	AppRouteSpecModePlugin    AppRouteSpecMode = "plugin"
)

// OpenAPIModelName returns the OpenAPI model name for AppRouteSpecMode.
func (AppRouteSpecMode) OpenAPIModelName() string {
	return "com.github.grafana.grafana-app-sdk.app.appmanifest.v1alpha3.AppRouteSpecMode"
}
