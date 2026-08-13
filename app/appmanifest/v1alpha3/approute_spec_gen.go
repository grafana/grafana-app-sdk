// Code generated - EDITING IS FUTILE. DO NOT EDIT.

package v1alpha3

// +k8s:openapi-gen=true
type AppRouteappManifestv1alpha3SchemaManifestVersion struct {
	// Name is the version name string, such as "v1" or "v1alpha1"
	Name string `json:"name"`
	// Served dictates whether this version is served by the API server.
	// A version cannot be removed from a manifest until it is no longer served.
	Served *bool `json:"served,omitempty"`
	// Kinds is a list of all the kinds served in this version.
	// Generally, kinds should exist in each version unless they have been deprecated (and no longer exist in a newer version)
	// or newly added (and didn't exist for older versions).
	Kinds []AppRouteappManifestv1alpha3SchemaManifestVersionKind `json:"kinds"`
	// Routes is a section defining version-level custom resource routes.
	// These routes are registered at the same level as kinds, and thus should not conflict with existing kinds.
	Routes *AppRouteappManifestv1alpha3SchemaManifestVersionRoutes `json:"routes,omitempty"`
}

// NewAppRouteappManifestv1alpha3SchemaManifestVersion creates a new AppRouteappManifestv1alpha3SchemaManifestVersion object.
func NewAppRouteappManifestv1alpha3SchemaManifestVersion() *AppRouteappManifestv1alpha3SchemaManifestVersion {
	return &AppRouteappManifestv1alpha3SchemaManifestVersion{
		Served: (func(input bool) *bool { return &input })(true),
		Kinds:  []AppRouteappManifestv1alpha3SchemaManifestVersionKind{},
	}
}

// OpenAPIModelName returns the OpenAPI model name for AppRouteappManifestv1alpha3SchemaManifestVersion.
func (AppRouteappManifestv1alpha3SchemaManifestVersion) OpenAPIModelName() string {
	return "com.github.grafana.grafana-app-sdk.app.appmanifest.v1alpha3.AppRouteappManifestv1alpha3SchemaManifestVersion"
}

// +k8s:openapi-gen=true
type AppRouteappManifestv1alpha3SchemaManifestVersionKind struct {
	// Kind is the name of the kind. This should begin with a capital letter and be CamelCased
	Kind string `json:"kind"`
	// Plural is the plural version of `kind`. This is optional and defaults to the kind + "s" if not present.
	Plural *string `json:"plural,omitempty"`
	// Scope dictates the scope of the kind. This field must be the same for all versions of the kind.
	// Different values will result in an error or undefined behavior.
	Scope AppRouteAppManifestv1alpha3SchemaManifestVersionKindScope `json:"scope"`
	// userReadable controls whether end users may get/list cluster-scoped resources of this kind.
	// Only meaningful when scope is "Cluster"; for namespaced kinds the field is ignored.
	UserReadable *bool                                                   `json:"userReadable,omitempty"`
	Admission    *AppRouteappManifestv1alpha3SchemaAdmissionCapabilities `json:"admission,omitempty"`
	// Schemas is the components.schemas section of an OpenAPI document describing this Kind.
	// It must contain a key named the same as the `kind` field of the Kind.
	// Other fields may be present to be referenced by $ref tags in a schema,
	// and references should lead with '#/components/schemas/' just as they would in a standard OpenAPI document.
	// If route responses in the `routes` section reference a definition in '#/components/schemas',
	// that schema definition must exist in this section.
	Schemas                  map[string]interface{}                                      `json:"schemas"`
	SelectableFields         []string                                                    `json:"selectableFields,omitempty"`
	AdditionalPrinterColumns []AppRouteappManifestv1alpha3SchemaAdditionalPrinterColumns `json:"additionalPrinterColumns,omitempty"`
	SearchFields             []AppRouteappManifestv1alpha3SchemaSearchField              `json:"searchFields,omitempty"`
	// Conversion indicates whether this kind supports custom conversion behavior exposed by the Convert method in the App.
	// It may not prevent automatic conversion behavior between versions of the kind when set to false
	// (for example, CRDs will always support simple conversion, and this flag enables webhook conversion).
	// This field should be the same for all versions of the kind. Different values will result in an error or undefined behavior.
	Conversion *bool `json:"conversion,omitempty"`
	// Routes is a map of subresource route path to spec3.PathProps description of the route.
	// Currently the spec3.PathProps is not explicitly typed, but typing will be enfoced in the future.
	// Invalid payloads will not be parsed correctly and may cause undefined behavior.
	Routes map[string]interface{} `json:"routes,omitempty"`
}

// NewAppRouteappManifestv1alpha3SchemaManifestVersionKind creates a new AppRouteappManifestv1alpha3SchemaManifestVersionKind object.
func NewAppRouteappManifestv1alpha3SchemaManifestVersionKind() *AppRouteappManifestv1alpha3SchemaManifestVersionKind {
	return &AppRouteappManifestv1alpha3SchemaManifestVersionKind{
		Scope:        AppRouteAppManifestv1alpha3SchemaManifestVersionKindScopeNamespaced,
		UserReadable: (func(input bool) *bool { return &input })(false),
		Schemas:      map[string]interface{}{},
		Conversion:   (func(input bool) *bool { return &input })(false),
	}
}

// OpenAPIModelName returns the OpenAPI model name for AppRouteappManifestv1alpha3SchemaManifestVersionKind.
func (AppRouteappManifestv1alpha3SchemaManifestVersionKind) OpenAPIModelName() string {
	return "com.github.grafana.grafana-app-sdk.app.appmanifest.v1alpha3.AppRouteappManifestv1alpha3SchemaManifestVersionKind"
}

// +k8s:openapi-gen=true
type AppRouteappManifestv1alpha3SchemaAdmissionCapabilities struct {
	Validation *AppRouteappManifestv1alpha3SchemaValidationCapability `json:"validation,omitempty"`
	Mutation   *AppRouteappManifestv1alpha3SchemaMutationCapability   `json:"mutation,omitempty"`
}

// NewAppRouteappManifestv1alpha3SchemaAdmissionCapabilities creates a new AppRouteappManifestv1alpha3SchemaAdmissionCapabilities object.
func NewAppRouteappManifestv1alpha3SchemaAdmissionCapabilities() *AppRouteappManifestv1alpha3SchemaAdmissionCapabilities {
	return &AppRouteappManifestv1alpha3SchemaAdmissionCapabilities{}
}

// OpenAPIModelName returns the OpenAPI model name for AppRouteappManifestv1alpha3SchemaAdmissionCapabilities.
func (AppRouteappManifestv1alpha3SchemaAdmissionCapabilities) OpenAPIModelName() string {
	return "com.github.grafana.grafana-app-sdk.app.appmanifest.v1alpha3.AppRouteappManifestv1alpha3SchemaAdmissionCapabilities"
}

// +k8s:openapi-gen=true
type AppRouteappManifestv1alpha3SchemaValidationCapability struct {
	Operations []AppRouteappManifestv1alpha3SchemaAdmissionOperation `json:"operations"`
}

// NewAppRouteappManifestv1alpha3SchemaValidationCapability creates a new AppRouteappManifestv1alpha3SchemaValidationCapability object.
func NewAppRouteappManifestv1alpha3SchemaValidationCapability() *AppRouteappManifestv1alpha3SchemaValidationCapability {
	return &AppRouteappManifestv1alpha3SchemaValidationCapability{
		Operations: []AppRouteappManifestv1alpha3SchemaAdmissionOperation{},
	}
}

// OpenAPIModelName returns the OpenAPI model name for AppRouteappManifestv1alpha3SchemaValidationCapability.
func (AppRouteappManifestv1alpha3SchemaValidationCapability) OpenAPIModelName() string {
	return "com.github.grafana.grafana-app-sdk.app.appmanifest.v1alpha3.AppRouteappManifestv1alpha3SchemaValidationCapability"
}

// +k8s:openapi-gen=true
type AppRouteappManifestv1alpha3SchemaAdmissionOperation string

const (
	AppRouteAppManifestv1alpha3SchemaAdmissionOperationCreate  AppRouteappManifestv1alpha3SchemaAdmissionOperation = "CREATE"
	AppRouteAppManifestv1alpha3SchemaAdmissionOperationUpdate  AppRouteappManifestv1alpha3SchemaAdmissionOperation = "UPDATE"
	AppRouteAppManifestv1alpha3SchemaAdmissionOperationDelete  AppRouteappManifestv1alpha3SchemaAdmissionOperation = "DELETE"
	AppRouteAppManifestv1alpha3SchemaAdmissionOperationConnect AppRouteappManifestv1alpha3SchemaAdmissionOperation = "CONNECT"
	AppRouteAppManifestv1alpha3SchemaAdmissionOperationAll     AppRouteappManifestv1alpha3SchemaAdmissionOperation = "*"
)

// OpenAPIModelName returns the OpenAPI model name for AppRouteappManifestv1alpha3SchemaAdmissionOperation.
func (AppRouteappManifestv1alpha3SchemaAdmissionOperation) OpenAPIModelName() string {
	return "com.github.grafana.grafana-app-sdk.app.appmanifest.v1alpha3.AppRouteappManifestv1alpha3SchemaAdmissionOperation"
}

// +k8s:openapi-gen=true
type AppRouteappManifestv1alpha3SchemaMutationCapability struct {
	Operations []AppRouteappManifestv1alpha3SchemaAdmissionOperation `json:"operations"`
}

// NewAppRouteappManifestv1alpha3SchemaMutationCapability creates a new AppRouteappManifestv1alpha3SchemaMutationCapability object.
func NewAppRouteappManifestv1alpha3SchemaMutationCapability() *AppRouteappManifestv1alpha3SchemaMutationCapability {
	return &AppRouteappManifestv1alpha3SchemaMutationCapability{
		Operations: []AppRouteappManifestv1alpha3SchemaAdmissionOperation{},
	}
}

// OpenAPIModelName returns the OpenAPI model name for AppRouteappManifestv1alpha3SchemaMutationCapability.
func (AppRouteappManifestv1alpha3SchemaMutationCapability) OpenAPIModelName() string {
	return "com.github.grafana.grafana-app-sdk.app.appmanifest.v1alpha3.AppRouteappManifestv1alpha3SchemaMutationCapability"
}

// +k8s:openapi-gen=true
type AppRouteappManifestv1alpha3SchemaAdditionalPrinterColumns struct {
	// name is a human readable name for the column.
	Name string `json:"name"`
	// type is an OpenAPI type definition for this column.
	// See https://github.com/OAI/OpenAPI-Specification/blob/master/versions/2.0.md#data-types for details.
	Type string `json:"type"`
	// format is an optional OpenAPI type definition for this column. The 'name' format is applied
	// to the primary identifier column to assist in clients identifying column is the resource name.
	// See https://github.com/OAI/OpenAPI-Specification/blob/master/versions/2.0.md#data-types for details.
	Format *string `json:"format,omitempty"`
	// description is a human readable description of this column.
	Description *string `json:"description,omitempty"`
	// priority is an integer defining the relative importance of this column compared to others. Lower
	// numbers are considered higher priority. Columns that may be omitted in limited space scenarios
	// should be given a priority greater than 0.
	Priority *int32 `json:"priority,omitempty"`
	// jsonPath is a simple JSON path (i.e. with array notation) which is evaluated against
	// each custom resource to produce the value for this column.
	JsonPath string `json:"jsonPath"`
}

// NewAppRouteappManifestv1alpha3SchemaAdditionalPrinterColumns creates a new AppRouteappManifestv1alpha3SchemaAdditionalPrinterColumns object.
func NewAppRouteappManifestv1alpha3SchemaAdditionalPrinterColumns() *AppRouteappManifestv1alpha3SchemaAdditionalPrinterColumns {
	return &AppRouteappManifestv1alpha3SchemaAdditionalPrinterColumns{}
}

// OpenAPIModelName returns the OpenAPI model name for AppRouteappManifestv1alpha3SchemaAdditionalPrinterColumns.
func (AppRouteappManifestv1alpha3SchemaAdditionalPrinterColumns) OpenAPIModelName() string {
	return "com.github.grafana.grafana-app-sdk.app.appmanifest.v1alpha3.AppRouteappManifestv1alpha3SchemaAdditionalPrinterColumns"
}

// #SearchField describes a field exposed for search indexing and querying.
// The type and capabilities values, and which capabilities are valid on which
// type, are defined by the searchfields package
// (github.com/grafana/grafana-app-sdk/searchfields), the shared source of
// truth for both the SDK codegen validator and the runtime search backend.
// +k8s:openapi-gen=true
type AppRouteappManifestv1alpha3SchemaSearchField struct {
	// name is the field name as it appears in search documents and queries.
	Name string `json:"name"`
	// path is the JSON path within the resource that supplies this field's value
	// (for example "spec.email"). When omitted, the field is populated by a custom
	// document builder rather than read directly from the resource.
	Path *string `json:"path,omitempty"`
	// type is the value type of the field.
	Type AppRouteAppManifestv1alpha3SchemaSearchFieldType `json:"type"`
	// array indicates that the field holds a list of values of the given type.
	Array *bool `json:"array,omitempty"`
	// capabilities lists what the field can be used for at query time, such as
	// filtering, full-text search, sorting, or faceting.
	Capabilities []AppRouteAppManifestv1alpha3SchemaSearchFieldCapabilities `json:"capabilities"`
	// emitZeroIfAbsent indexes the type's zero value when path resolves to nothing,
	// so sort and range queries see every document. Without it, a document missing
	// the path omits the field entirely.
	EmitZeroIfAbsent *bool `json:"emitZeroIfAbsent,omitempty"`
	// description is a human readable description of the field.
	Description *string `json:"description,omitempty"`
}

// NewAppRouteappManifestv1alpha3SchemaSearchField creates a new AppRouteappManifestv1alpha3SchemaSearchField object.
func NewAppRouteappManifestv1alpha3SchemaSearchField() *AppRouteappManifestv1alpha3SchemaSearchField {
	return &AppRouteappManifestv1alpha3SchemaSearchField{
		Array:            (func(input bool) *bool { return &input })(false),
		Capabilities:     []AppRouteAppManifestv1alpha3SchemaSearchFieldCapabilities{},
		EmitZeroIfAbsent: (func(input bool) *bool { return &input })(false),
	}
}

// OpenAPIModelName returns the OpenAPI model name for AppRouteappManifestv1alpha3SchemaSearchField.
func (AppRouteappManifestv1alpha3SchemaSearchField) OpenAPIModelName() string {
	return "com.github.grafana.grafana-app-sdk.app.appmanifest.v1alpha3.AppRouteappManifestv1alpha3SchemaSearchField"
}

// +k8s:openapi-gen=true
type AppRouteappManifestv1alpha3SchemaManifestVersionRoutes struct {
	// Namespaced is a map of namespace-scoped route paths to spec3.PathProps description of the route.
	// Currently the spec3.PathProps is not explicitly typed, but typing will be enfoced in the future.
	// Invalid payloads will not be parsed correctly and may cause undefined behavior.
	Namespaced map[string]interface{} `json:"namespaced,omitempty"`
	// Cluster is a map of cluster-scoped route paths to spec3.PathProps description of the route.
	// Currently the spec3.PathProps is not explicitly typed, but typing will be enfoced in the future.
	// Invalid payloads will not be parsed correctly and may cause undefined behavior.
	Cluster map[string]interface{} `json:"cluster,omitempty"`
	// Schemas contains additional schemas referenced by requests/responses for namespaced and cluster routes.
	// If route responses in the `namespaced` or `cluster` section reference a definition in '#/components/schemas',
	// that schema definition must exist in this section.
	Schemas map[string]interface{} `json:"schemas,omitempty"`
}

// NewAppRouteappManifestv1alpha3SchemaManifestVersionRoutes creates a new AppRouteappManifestv1alpha3SchemaManifestVersionRoutes object.
func NewAppRouteappManifestv1alpha3SchemaManifestVersionRoutes() *AppRouteappManifestv1alpha3SchemaManifestVersionRoutes {
	return &AppRouteappManifestv1alpha3SchemaManifestVersionRoutes{}
}

// OpenAPIModelName returns the OpenAPI model name for AppRouteappManifestv1alpha3SchemaManifestVersionRoutes.
func (AppRouteappManifestv1alpha3SchemaManifestVersionRoutes) OpenAPIModelName() string {
	return "com.github.grafana.grafana-app-sdk.app.appmanifest.v1alpha3.AppRouteappManifestv1alpha3SchemaManifestVersionRoutes"
}

// +k8s:openapi-gen=true
type AppRouteappManifestv1alpha3SchemaKindPermission struct {
	Group    string   `json:"group"`
	Resource string   `json:"resource"`
	Actions  []string `json:"actions"`
}

// NewAppRouteappManifestv1alpha3SchemaKindPermission creates a new AppRouteappManifestv1alpha3SchemaKindPermission object.
func NewAppRouteappManifestv1alpha3SchemaKindPermission() *AppRouteappManifestv1alpha3SchemaKindPermission {
	return &AppRouteappManifestv1alpha3SchemaKindPermission{
		Actions: []string{},
	}
}

// OpenAPIModelName returns the OpenAPI model name for AppRouteappManifestv1alpha3SchemaKindPermission.
func (AppRouteappManifestv1alpha3SchemaKindPermission) OpenAPIModelName() string {
	return "com.github.grafana.grafana-app-sdk.app.appmanifest.v1alpha3.AppRouteappManifestv1alpha3SchemaKindPermission"
}

// +k8s:openapi-gen=true
type AppRouteappManifestv1alpha3SchemaRole struct {
	// Title will be used as the role title in grafana
	Title string `json:"title"`
	// Description is used as the role description in grafana, displayed in the UI and API responses
	Description string                                      `json:"description"`
	Kinds       []AppRouteappManifestv1alpha3SchemaRoleKind `json:"kinds,omitempty"`
	// Routes is a list of route names to match.
	// To match the same route in multiple versions, it should share the same name.
	Routes []string `json:"routes,omitempty"`
}

// NewAppRouteappManifestv1alpha3SchemaRole creates a new AppRouteappManifestv1alpha3SchemaRole object.
func NewAppRouteappManifestv1alpha3SchemaRole() *AppRouteappManifestv1alpha3SchemaRole {
	return &AppRouteappManifestv1alpha3SchemaRole{}
}

// OpenAPIModelName returns the OpenAPI model name for AppRouteappManifestv1alpha3SchemaRole.
func (AppRouteappManifestv1alpha3SchemaRole) OpenAPIModelName() string {
	return "com.github.grafana.grafana-app-sdk.app.appmanifest.v1alpha3.AppRouteappManifestv1alpha3SchemaRole"
}

// +k8s:openapi-gen=true
type AppRouteappManifestv1alpha3SchemaRoleKind interface{}

// +k8s:openapi-gen=true
type AppRouteappManifestv1alpha3SchemaRoleKindWithPermissionSet struct {
	Kind          string                                                                  `json:"kind"`
	PermissionSet AppRouteAppManifestv1alpha3SchemaRoleKindWithPermissionSetPermissionSet `json:"permissionSet"`
}

// NewAppRouteappManifestv1alpha3SchemaRoleKindWithPermissionSet creates a new AppRouteappManifestv1alpha3SchemaRoleKindWithPermissionSet object.
func NewAppRouteappManifestv1alpha3SchemaRoleKindWithPermissionSet() *AppRouteappManifestv1alpha3SchemaRoleKindWithPermissionSet {
	return &AppRouteappManifestv1alpha3SchemaRoleKindWithPermissionSet{
		PermissionSet: AppRouteAppManifestv1alpha3SchemaRoleKindWithPermissionSetPermissionSetViewer,
	}
}

// OpenAPIModelName returns the OpenAPI model name for AppRouteappManifestv1alpha3SchemaRoleKindWithPermissionSet.
func (AppRouteappManifestv1alpha3SchemaRoleKindWithPermissionSet) OpenAPIModelName() string {
	return "com.github.grafana.grafana-app-sdk.app.appmanifest.v1alpha3.AppRouteappManifestv1alpha3SchemaRoleKindWithPermissionSet"
}

// +k8s:openapi-gen=true
type AppRouteappManifestv1alpha3SchemaRoleKindWithVerbs struct {
	Kind  string   `json:"kind"`
	Verbs []string `json:"verbs"`
}

// NewAppRouteappManifestv1alpha3SchemaRoleKindWithVerbs creates a new AppRouteappManifestv1alpha3SchemaRoleKindWithVerbs object.
func NewAppRouteappManifestv1alpha3SchemaRoleKindWithVerbs() *AppRouteappManifestv1alpha3SchemaRoleKindWithVerbs {
	return &AppRouteappManifestv1alpha3SchemaRoleKindWithVerbs{
		Verbs: []string{},
	}
}

// OpenAPIModelName returns the OpenAPI model name for AppRouteappManifestv1alpha3SchemaRoleKindWithVerbs.
func (AppRouteappManifestv1alpha3SchemaRoleKindWithVerbs) OpenAPIModelName() string {
	return "com.github.grafana.grafana-app-sdk.app.appmanifest.v1alpha3.AppRouteappManifestv1alpha3SchemaRoleKindWithVerbs"
}

// +k8s:openapi-gen=true
type AppRouteruntimeConfigv1alpha1SchemaAPIServer struct {
	Url string                                        `json:"url"`
	Tls AppRouteruntimeConfigv1alpha1SchemaTLSOptions `json:"tls"`
}

// NewAppRouteruntimeConfigv1alpha1SchemaAPIServer creates a new AppRouteruntimeConfigv1alpha1SchemaAPIServer object.
func NewAppRouteruntimeConfigv1alpha1SchemaAPIServer() *AppRouteruntimeConfigv1alpha1SchemaAPIServer {
	return &AppRouteruntimeConfigv1alpha1SchemaAPIServer{
		Tls: *NewAppRouteruntimeConfigv1alpha1SchemaTLSOptions(),
	}
}

// OpenAPIModelName returns the OpenAPI model name for AppRouteruntimeConfigv1alpha1SchemaAPIServer.
func (AppRouteruntimeConfigv1alpha1SchemaAPIServer) OpenAPIModelName() string {
	return "com.github.grafana.grafana-app-sdk.app.appmanifest.v1alpha3.AppRouteruntimeConfigv1alpha1SchemaAPIServer"
}

// +k8s:openapi-gen=true
type AppRouteruntimeConfigv1alpha1SchemaTLSOptions struct {
	CaData        *string `json:"caData,omitempty"`
	SkipTLSVerify bool    `json:"skipTLSVerify"`
}

// NewAppRouteruntimeConfigv1alpha1SchemaTLSOptions creates a new AppRouteruntimeConfigv1alpha1SchemaTLSOptions object.
func NewAppRouteruntimeConfigv1alpha1SchemaTLSOptions() *AppRouteruntimeConfigv1alpha1SchemaTLSOptions {
	return &AppRouteruntimeConfigv1alpha1SchemaTLSOptions{
		SkipTLSVerify: false,
	}
}

// OpenAPIModelName returns the OpenAPI model name for AppRouteruntimeConfigv1alpha1SchemaTLSOptions.
func (AppRouteruntimeConfigv1alpha1SchemaTLSOptions) OpenAPIModelName() string {
	return "com.github.grafana.grafana-app-sdk.app.appmanifest.v1alpha3.AppRouteruntimeConfigv1alpha1SchemaTLSOptions"
}

// +k8s:openapi-gen=true
type AppRouteruntimeConfigv1alpha1SchemaOperatorConfig struct {
	Url      string                                                     `json:"url"`
	Tls      AppRouteruntimeConfigv1alpha1SchemaTLSOptions              `json:"tls"`
	Webhooks *AppRouteruntimeConfigv1alpha1SchemaOperatorWebhookOptions `json:"webhooks,omitempty"`
}

// NewAppRouteruntimeConfigv1alpha1SchemaOperatorConfig creates a new AppRouteruntimeConfigv1alpha1SchemaOperatorConfig object.
func NewAppRouteruntimeConfigv1alpha1SchemaOperatorConfig() *AppRouteruntimeConfigv1alpha1SchemaOperatorConfig {
	return &AppRouteruntimeConfigv1alpha1SchemaOperatorConfig{
		Tls: *NewAppRouteruntimeConfigv1alpha1SchemaTLSOptions(),
	}
}

// OpenAPIModelName returns the OpenAPI model name for AppRouteruntimeConfigv1alpha1SchemaOperatorConfig.
func (AppRouteruntimeConfigv1alpha1SchemaOperatorConfig) OpenAPIModelName() string {
	return "com.github.grafana.grafana-app-sdk.app.appmanifest.v1alpha3.AppRouteruntimeConfigv1alpha1SchemaOperatorConfig"
}

// +k8s:openapi-gen=true
type AppRouteruntimeConfigv1alpha1SchemaOperatorWebhookOptions struct {
	ConversionPath *string `json:"conversionPath,omitempty"`
	ValidationPath *string `json:"validationPath,omitempty"`
	MutationPath   *string `json:"mutationPath,omitempty"`
}

// NewAppRouteruntimeConfigv1alpha1SchemaOperatorWebhookOptions creates a new AppRouteruntimeConfigv1alpha1SchemaOperatorWebhookOptions object.
func NewAppRouteruntimeConfigv1alpha1SchemaOperatorWebhookOptions() *AppRouteruntimeConfigv1alpha1SchemaOperatorWebhookOptions {
	return &AppRouteruntimeConfigv1alpha1SchemaOperatorWebhookOptions{}
}

// OpenAPIModelName returns the OpenAPI model name for AppRouteruntimeConfigv1alpha1SchemaOperatorWebhookOptions.
func (AppRouteruntimeConfigv1alpha1SchemaOperatorWebhookOptions) OpenAPIModelName() string {
	return "com.github.grafana.grafana-app-sdk.app.appmanifest.v1alpha3.AppRouteruntimeConfigv1alpha1SchemaOperatorWebhookOptions"
}

// +k8s:openapi-gen=true
type AppRouteruntimeConfigv1alpha1SchemaPluginConfig struct {
	Url string                                        `json:"url"`
	Tls AppRouteruntimeConfigv1alpha1SchemaTLSOptions `json:"tls"`
}

// NewAppRouteruntimeConfigv1alpha1SchemaPluginConfig creates a new AppRouteruntimeConfigv1alpha1SchemaPluginConfig object.
func NewAppRouteruntimeConfigv1alpha1SchemaPluginConfig() *AppRouteruntimeConfigv1alpha1SchemaPluginConfig {
	return &AppRouteruntimeConfigv1alpha1SchemaPluginConfig{
		Tls: *NewAppRouteruntimeConfigv1alpha1SchemaTLSOptions(),
	}
}

// OpenAPIModelName returns the OpenAPI model name for AppRouteruntimeConfigv1alpha1SchemaPluginConfig.
func (AppRouteruntimeConfigv1alpha1SchemaPluginConfig) OpenAPIModelName() string {
	return "com.github.grafana.grafana-app-sdk.app.appmanifest.v1alpha3.AppRouteruntimeConfigv1alpha1SchemaPluginConfig"
}

// AppRoute's spec is the flat union of the AppManifest and RuntimeConfig
// specs. DRY: reference both specs directly, never redeclare fields.
// This only unifies cleanly because the two field sets are disjoint
// (v1alpha3 dropped `operator`, which used to collide with RuntimeConfig's).
// +k8s:openapi-gen=true
type AppRouteSpec struct {
	// AppName is the unique ID of the app
	AppName string `json:"appName"`
	// AppDisplayName is the display name of the app, which can contain any printable characters
	AppDisplayName string `json:"appDisplayName"`
	Group          string `json:"group"`
	// Versions is the list of versions for this manifest, in order.
	Versions []AppRouteappManifestv1alpha3SchemaManifestVersion `json:"versions"`
	// PreferredVersion is the preferred version for API use. If empty, it will use the latest from versions.
	// For CRDs, this also dictates which version is used for storage.
	PreferredVersion *string `json:"preferredVersion,omitempty"`
	// ExtraPermissions contains additional permissions needed for an app's backend component to operate.
	// Apps implicitly have all permissions for kinds they managed (defined in `kinds`).
	ExtraPermissions *AppRouteV1alpha3SpecExtraPermissions `json:"extraPermissions,omitempty"`
	// DryRunKinds dictates whether this revision should create/update CRD's from the provided kinds,
	// Or simply validate and report errors in status.resources.crds.
	// If dryRunKinds is true, CRD change validation will be skipped on ingress and reported in status instead.
	// Even if no validation errors exist, CRDs will not be created or updated for a revision with dryRunKinds=true.
	DryRunKinds *bool `json:"dryRunKinds,omitempty"`
	// Roles contains information for new user roles associated with this app.
	// It is a map of the role name (e.g. "dashboard:reader") to the set of permissions on resources managed by this app.
	Roles     map[string]AppRouteappManifestv1alpha3SchemaRole   `json:"roles,omitempty"`
	Mode      AppRouteSpecMode                                   `json:"mode"`
	ApiServer *AppRouteruntimeConfigv1alpha1SchemaAPIServer      `json:"apiServer,omitempty"`
	Operator  *AppRouteruntimeConfigv1alpha1SchemaOperatorConfig `json:"operator,omitempty"`
	// RoleBindings binds the roles specified in Roles to groups.
	// Basic groups are "anonymous", "viewer", "editor", and "admin".
	// Additional groups are specified under "additional"
	RoleBindings *AppRouteV1alpha3SpecRoleBindings                `json:"roleBindings,omitempty"`
	Plugin       *AppRouteruntimeConfigv1alpha1SchemaPluginConfig `json:"plugin,omitempty"`
}

// NewAppRouteSpec creates a new AppRouteSpec object.
func NewAppRouteSpec() *AppRouteSpec {
	return &AppRouteSpec{
		Versions:    []AppRouteappManifestv1alpha3SchemaManifestVersion{},
		DryRunKinds: (func(input bool) *bool { return &input })(false),
	}
}

// OpenAPIModelName returns the OpenAPI model name for AppRouteSpec.
func (AppRouteSpec) OpenAPIModelName() string {
	return "com.github.grafana.grafana-app-sdk.app.appmanifest.v1alpha3.AppRouteSpec"
}

// +k8s:openapi-gen=true
type AppRouteV1alpha3SpecExtraPermissions struct {
	// accessKinds is a list of KindPermission objects for accessing additional kinds provided by other apps
	AccessKinds []AppRouteappManifestv1alpha3SchemaKindPermission `json:"accessKinds"`
}

// NewAppRouteV1alpha3SpecExtraPermissions creates a new AppRouteV1alpha3SpecExtraPermissions object.
func NewAppRouteV1alpha3SpecExtraPermissions() *AppRouteV1alpha3SpecExtraPermissions {
	return &AppRouteV1alpha3SpecExtraPermissions{
		AccessKinds: []AppRouteappManifestv1alpha3SchemaKindPermission{},
	}
}

// OpenAPIModelName returns the OpenAPI model name for AppRouteV1alpha3SpecExtraPermissions.
func (AppRouteV1alpha3SpecExtraPermissions) OpenAPIModelName() string {
	return "com.github.grafana.grafana-app-sdk.app.appmanifest.v1alpha3.AppRouteV1alpha3SpecExtraPermissions"
}

// +k8s:openapi-gen=true
type AppRouteV1alpha3SpecRoleBindings struct {
	Viewer     []string            `json:"viewer,omitempty"`
	Editor     []string            `json:"editor,omitempty"`
	Admin      []string            `json:"admin,omitempty"`
	Additional map[string][]string `json:"additional,omitempty"`
}

// NewAppRouteV1alpha3SpecRoleBindings creates a new AppRouteV1alpha3SpecRoleBindings object.
func NewAppRouteV1alpha3SpecRoleBindings() *AppRouteV1alpha3SpecRoleBindings {
	return &AppRouteV1alpha3SpecRoleBindings{}
}

// OpenAPIModelName returns the OpenAPI model name for AppRouteV1alpha3SpecRoleBindings.
func (AppRouteV1alpha3SpecRoleBindings) OpenAPIModelName() string {
	return "com.github.grafana.grafana-app-sdk.app.appmanifest.v1alpha3.AppRouteV1alpha3SpecRoleBindings"
}

// +k8s:openapi-gen=true
type AppRouteAppManifestv1alpha3SchemaManifestVersionKindScope string

const (
	AppRouteAppManifestv1alpha3SchemaManifestVersionKindScopeNamespaced AppRouteAppManifestv1alpha3SchemaManifestVersionKindScope = "Namespaced"
	AppRouteAppManifestv1alpha3SchemaManifestVersionKindScopeCluster    AppRouteAppManifestv1alpha3SchemaManifestVersionKindScope = "Cluster"
)

// OpenAPIModelName returns the OpenAPI model name for AppRouteAppManifestv1alpha3SchemaManifestVersionKindScope.
func (AppRouteAppManifestv1alpha3SchemaManifestVersionKindScope) OpenAPIModelName() string {
	return "com.github.grafana.grafana-app-sdk.app.appmanifest.v1alpha3.AppRouteAppManifestv1alpha3SchemaManifestVersionKindScope"
}

// +k8s:openapi-gen=true
type AppRouteAppManifestv1alpha3SchemaSearchFieldType string

const (
	AppRouteAppManifestv1alpha3SchemaSearchFieldTypeString  AppRouteAppManifestv1alpha3SchemaSearchFieldType = "string"
	AppRouteAppManifestv1alpha3SchemaSearchFieldTypeInt64   AppRouteAppManifestv1alpha3SchemaSearchFieldType = "int64"
	AppRouteAppManifestv1alpha3SchemaSearchFieldTypeDouble  AppRouteAppManifestv1alpha3SchemaSearchFieldType = "double"
	AppRouteAppManifestv1alpha3SchemaSearchFieldTypeBoolean AppRouteAppManifestv1alpha3SchemaSearchFieldType = "boolean"
	AppRouteAppManifestv1alpha3SchemaSearchFieldTypeDate    AppRouteAppManifestv1alpha3SchemaSearchFieldType = "date"
)

// OpenAPIModelName returns the OpenAPI model name for AppRouteAppManifestv1alpha3SchemaSearchFieldType.
func (AppRouteAppManifestv1alpha3SchemaSearchFieldType) OpenAPIModelName() string {
	return "com.github.grafana.grafana-app-sdk.app.appmanifest.v1alpha3.AppRouteAppManifestv1alpha3SchemaSearchFieldType"
}

// +k8s:openapi-gen=true
type AppRouteAppManifestv1alpha3SchemaSearchFieldCapabilities string

const (
	AppRouteAppManifestv1alpha3SchemaSearchFieldCapabilitiesFilter   AppRouteAppManifestv1alpha3SchemaSearchFieldCapabilities = "filter"
	AppRouteAppManifestv1alpha3SchemaSearchFieldCapabilitiesText     AppRouteAppManifestv1alpha3SchemaSearchFieldCapabilities = "text"
	AppRouteAppManifestv1alpha3SchemaSearchFieldCapabilitiesPartial  AppRouteAppManifestv1alpha3SchemaSearchFieldCapabilities = "partial"
	AppRouteAppManifestv1alpha3SchemaSearchFieldCapabilitiesSort     AppRouteAppManifestv1alpha3SchemaSearchFieldCapabilities = "sort"
	AppRouteAppManifestv1alpha3SchemaSearchFieldCapabilitiesFacet    AppRouteAppManifestv1alpha3SchemaSearchFieldCapabilities = "facet"
	AppRouteAppManifestv1alpha3SchemaSearchFieldCapabilitiesRetrieve AppRouteAppManifestv1alpha3SchemaSearchFieldCapabilities = "retrieve"
	AppRouteAppManifestv1alpha3SchemaSearchFieldCapabilitiesUnranked AppRouteAppManifestv1alpha3SchemaSearchFieldCapabilities = "unranked"
)

// OpenAPIModelName returns the OpenAPI model name for AppRouteAppManifestv1alpha3SchemaSearchFieldCapabilities.
func (AppRouteAppManifestv1alpha3SchemaSearchFieldCapabilities) OpenAPIModelName() string {
	return "com.github.grafana.grafana-app-sdk.app.appmanifest.v1alpha3.AppRouteAppManifestv1alpha3SchemaSearchFieldCapabilities"
}

// +k8s:openapi-gen=true
type AppRouteAppManifestv1alpha3SchemaRoleKindWithPermissionSetPermissionSet string

const (
	AppRouteAppManifestv1alpha3SchemaRoleKindWithPermissionSetPermissionSetViewer AppRouteAppManifestv1alpha3SchemaRoleKindWithPermissionSetPermissionSet = "viewer"
	AppRouteAppManifestv1alpha3SchemaRoleKindWithPermissionSetPermissionSetEditor AppRouteAppManifestv1alpha3SchemaRoleKindWithPermissionSetPermissionSet = "editor"
	AppRouteAppManifestv1alpha3SchemaRoleKindWithPermissionSetPermissionSetAdmin  AppRouteAppManifestv1alpha3SchemaRoleKindWithPermissionSetPermissionSet = "admin"
)

// OpenAPIModelName returns the OpenAPI model name for AppRouteAppManifestv1alpha3SchemaRoleKindWithPermissionSetPermissionSet.
func (AppRouteAppManifestv1alpha3SchemaRoleKindWithPermissionSetPermissionSet) OpenAPIModelName() string {
	return "com.github.grafana.grafana-app-sdk.app.appmanifest.v1alpha3.AppRouteAppManifestv1alpha3SchemaRoleKindWithPermissionSetPermissionSet"
}

// +k8s:openapi-gen=true
type AppRouteSpecMode string

const (
	AppRouteSpecModeApiserver AppRouteSpecMode = "apiserver"
	AppRouteSpecModePlugin    AppRouteSpecMode = "plugin"
	AppRouteSpecModeOperator  AppRouteSpecMode = "operator"
)

// OpenAPIModelName returns the OpenAPI model name for AppRouteSpecMode.
func (AppRouteSpecMode) OpenAPIModelName() string {
	return "com.github.grafana.grafana-app-sdk.app.appmanifest.v1alpha3.AppRouteSpecMode"
}
