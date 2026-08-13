// Code generated - EDITING IS FUTILE. DO NOT EDIT.

package v1alpha3

// +k8s:openapi-gen=true
type AppManifestManifestVersion struct {
	// Name is the version name string, such as "v1" or "v1alpha1"
	Name string `json:"name"`
	// Served dictates whether this version is served by the API server.
	// A version cannot be removed from a manifest until it is no longer served.
	Served *bool `json:"served,omitempty"`
	// Kinds is a list of all the kinds served in this version.
	// Generally, kinds should exist in each version unless they have been deprecated (and no longer exist in a newer version)
	// or newly added (and didn't exist for older versions).
	Kinds []AppManifestManifestVersionKind `json:"kinds"`
	// Routes is a section defining version-level custom resource routes.
	// These routes are registered at the same level as kinds, and thus should not conflict with existing kinds.
	Routes *AppManifestManifestVersionRoutes `json:"routes,omitempty"`
}

// NewAppManifestManifestVersion creates a new AppManifestManifestVersion object.
func NewAppManifestManifestVersion() *AppManifestManifestVersion {
	return &AppManifestManifestVersion{
		Served: (func(input bool) *bool { return &input })(true),
		Kinds:  []AppManifestManifestVersionKind{},
	}
}

// OpenAPIModelName returns the OpenAPI model name for AppManifestManifestVersion.
func (AppManifestManifestVersion) OpenAPIModelName() string {
	return "com.github.grafana.grafana-app-sdk.app.appmanifest.v1alpha3.AppManifestManifestVersion"
}

// +k8s:openapi-gen=true
type AppManifestManifestVersionKind struct {
	// Kind is the name of the kind. This should begin with a capital letter and be CamelCased
	Kind string `json:"kind"`
	// Plural is the plural version of `kind`. This is optional and defaults to the kind + "s" if not present.
	Plural *string `json:"plural,omitempty"`
	// Scope dictates the scope of the kind. This field must be the same for all versions of the kind.
	// Different values will result in an error or undefined behavior.
	Scope AppManifestManifestVersionKindScope `json:"scope"`
	// userReadable controls whether end users may get/list cluster-scoped resources of this kind.
	// Only meaningful when scope is "Cluster"; for namespaced kinds the field is ignored.
	UserReadable *bool                             `json:"userReadable,omitempty"`
	Admission    *AppManifestAdmissionCapabilities `json:"admission,omitempty"`
	// Schemas is the components.schemas section of an OpenAPI document describing this Kind.
	// It must contain a key named the same as the `kind` field of the Kind.
	// Other fields may be present to be referenced by $ref tags in a schema,
	// and references should lead with '#/components/schemas/' just as they would in a standard OpenAPI document.
	// If route responses in the `routes` section reference a definition in '#/components/schemas',
	// that schema definition must exist in this section.
	Schemas                  map[string]interface{}                `json:"schemas"`
	SelectableFields         []string                              `json:"selectableFields,omitempty"`
	AdditionalPrinterColumns []AppManifestAdditionalPrinterColumns `json:"additionalPrinterColumns,omitempty"`
	SearchFields             []AppManifestSearchField              `json:"searchFields,omitempty"`
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

// NewAppManifestManifestVersionKind creates a new AppManifestManifestVersionKind object.
func NewAppManifestManifestVersionKind() *AppManifestManifestVersionKind {
	return &AppManifestManifestVersionKind{
		Scope:        AppManifestManifestVersionKindScopeNamespaced,
		UserReadable: (func(input bool) *bool { return &input })(false),
		Schemas:      map[string]interface{}{},
		Conversion:   (func(input bool) *bool { return &input })(false),
	}
}

// OpenAPIModelName returns the OpenAPI model name for AppManifestManifestVersionKind.
func (AppManifestManifestVersionKind) OpenAPIModelName() string {
	return "com.github.grafana.grafana-app-sdk.app.appmanifest.v1alpha3.AppManifestManifestVersionKind"
}

// +k8s:openapi-gen=true
type AppManifestAdmissionCapabilities struct {
	Validation *AppManifestValidationCapability `json:"validation,omitempty"`
	Mutation   *AppManifestMutationCapability   `json:"mutation,omitempty"`
}

// NewAppManifestAdmissionCapabilities creates a new AppManifestAdmissionCapabilities object.
func NewAppManifestAdmissionCapabilities() *AppManifestAdmissionCapabilities {
	return &AppManifestAdmissionCapabilities{}
}

// OpenAPIModelName returns the OpenAPI model name for AppManifestAdmissionCapabilities.
func (AppManifestAdmissionCapabilities) OpenAPIModelName() string {
	return "com.github.grafana.grafana-app-sdk.app.appmanifest.v1alpha3.AppManifestAdmissionCapabilities"
}

// +k8s:openapi-gen=true
type AppManifestValidationCapability struct {
	Operations []AppManifestAdmissionOperation `json:"operations"`
}

// NewAppManifestValidationCapability creates a new AppManifestValidationCapability object.
func NewAppManifestValidationCapability() *AppManifestValidationCapability {
	return &AppManifestValidationCapability{
		Operations: []AppManifestAdmissionOperation{},
	}
}

// OpenAPIModelName returns the OpenAPI model name for AppManifestValidationCapability.
func (AppManifestValidationCapability) OpenAPIModelName() string {
	return "com.github.grafana.grafana-app-sdk.app.appmanifest.v1alpha3.AppManifestValidationCapability"
}

// +k8s:openapi-gen=true
type AppManifestAdmissionOperation string

const (
	AppManifestAdmissionOperationCreate  AppManifestAdmissionOperation = "CREATE"
	AppManifestAdmissionOperationUpdate  AppManifestAdmissionOperation = "UPDATE"
	AppManifestAdmissionOperationDelete  AppManifestAdmissionOperation = "DELETE"
	AppManifestAdmissionOperationConnect AppManifestAdmissionOperation = "CONNECT"
	AppManifestAdmissionOperationAll     AppManifestAdmissionOperation = "*"
)

// OpenAPIModelName returns the OpenAPI model name for AppManifestAdmissionOperation.
func (AppManifestAdmissionOperation) OpenAPIModelName() string {
	return "com.github.grafana.grafana-app-sdk.app.appmanifest.v1alpha3.AppManifestAdmissionOperation"
}

// +k8s:openapi-gen=true
type AppManifestMutationCapability struct {
	Operations []AppManifestAdmissionOperation `json:"operations"`
}

// NewAppManifestMutationCapability creates a new AppManifestMutationCapability object.
func NewAppManifestMutationCapability() *AppManifestMutationCapability {
	return &AppManifestMutationCapability{
		Operations: []AppManifestAdmissionOperation{},
	}
}

// OpenAPIModelName returns the OpenAPI model name for AppManifestMutationCapability.
func (AppManifestMutationCapability) OpenAPIModelName() string {
	return "com.github.grafana.grafana-app-sdk.app.appmanifest.v1alpha3.AppManifestMutationCapability"
}

// +k8s:openapi-gen=true
type AppManifestAdditionalPrinterColumns struct {
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

// NewAppManifestAdditionalPrinterColumns creates a new AppManifestAdditionalPrinterColumns object.
func NewAppManifestAdditionalPrinterColumns() *AppManifestAdditionalPrinterColumns {
	return &AppManifestAdditionalPrinterColumns{}
}

// OpenAPIModelName returns the OpenAPI model name for AppManifestAdditionalPrinterColumns.
func (AppManifestAdditionalPrinterColumns) OpenAPIModelName() string {
	return "com.github.grafana.grafana-app-sdk.app.appmanifest.v1alpha3.AppManifestAdditionalPrinterColumns"
}

// #SearchField describes a field exposed for search indexing and querying.
// The type and capabilities values, and which capabilities are valid on which
// type, are defined by the searchfields package
// (github.com/grafana/grafana-app-sdk/searchfields), the shared source of
// truth for both the SDK codegen validator and the runtime search backend.
// +k8s:openapi-gen=true
type AppManifestSearchField struct {
	// name is the field name as it appears in search documents and queries.
	Name string `json:"name"`
	// path is the JSON path within the resource that supplies this field's value
	// (for example "spec.email"). When omitted, the field is populated by a custom
	// document builder rather than read directly from the resource.
	Path *string `json:"path,omitempty"`
	// type is the value type of the field.
	Type AppManifestSearchFieldType `json:"type"`
	// array indicates that the field holds a list of values of the given type.
	Array *bool `json:"array,omitempty"`
	// capabilities lists what the field can be used for at query time, such as
	// filtering, full-text search, sorting, or faceting.
	Capabilities []AppManifestSearchFieldCapabilities `json:"capabilities"`
	// emitZeroIfAbsent indexes the type's zero value when path resolves to nothing,
	// so sort and range queries see every document. Without it, a document missing
	// the path omits the field entirely.
	EmitZeroIfAbsent *bool `json:"emitZeroIfAbsent,omitempty"`
	// description is a human readable description of the field.
	Description *string `json:"description,omitempty"`
}

// NewAppManifestSearchField creates a new AppManifestSearchField object.
func NewAppManifestSearchField() *AppManifestSearchField {
	return &AppManifestSearchField{
		Array:            (func(input bool) *bool { return &input })(false),
		Capabilities:     []AppManifestSearchFieldCapabilities{},
		EmitZeroIfAbsent: (func(input bool) *bool { return &input })(false),
	}
}

// OpenAPIModelName returns the OpenAPI model name for AppManifestSearchField.
func (AppManifestSearchField) OpenAPIModelName() string {
	return "com.github.grafana.grafana-app-sdk.app.appmanifest.v1alpha3.AppManifestSearchField"
}

// +k8s:openapi-gen=true
type AppManifestManifestVersionRoutes struct {
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

// NewAppManifestManifestVersionRoutes creates a new AppManifestManifestVersionRoutes object.
func NewAppManifestManifestVersionRoutes() *AppManifestManifestVersionRoutes {
	return &AppManifestManifestVersionRoutes{}
}

// OpenAPIModelName returns the OpenAPI model name for AppManifestManifestVersionRoutes.
func (AppManifestManifestVersionRoutes) OpenAPIModelName() string {
	return "com.github.grafana.grafana-app-sdk.app.appmanifest.v1alpha3.AppManifestManifestVersionRoutes"
}

// +k8s:openapi-gen=true
type AppManifestKindPermission struct {
	Group    string   `json:"group"`
	Resource string   `json:"resource"`
	Actions  []string `json:"actions"`
}

// NewAppManifestKindPermission creates a new AppManifestKindPermission object.
func NewAppManifestKindPermission() *AppManifestKindPermission {
	return &AppManifestKindPermission{
		Actions: []string{},
	}
}

// OpenAPIModelName returns the OpenAPI model name for AppManifestKindPermission.
func (AppManifestKindPermission) OpenAPIModelName() string {
	return "com.github.grafana.grafana-app-sdk.app.appmanifest.v1alpha3.AppManifestKindPermission"
}

// +k8s:openapi-gen=true
type AppManifestRole struct {
	// Title will be used as the role title in grafana
	Title string `json:"title"`
	// Description is used as the role description in grafana, displayed in the UI and API responses
	Description string                `json:"description"`
	Kinds       []AppManifestRoleKind `json:"kinds,omitempty"`
	// Routes is a list of route names to match.
	// To match the same route in multiple versions, it should share the same name.
	Routes []string `json:"routes,omitempty"`
}

// NewAppManifestRole creates a new AppManifestRole object.
func NewAppManifestRole() *AppManifestRole {
	return &AppManifestRole{}
}

// OpenAPIModelName returns the OpenAPI model name for AppManifestRole.
func (AppManifestRole) OpenAPIModelName() string {
	return "com.github.grafana.grafana-app-sdk.app.appmanifest.v1alpha3.AppManifestRole"
}

// +k8s:openapi-gen=true
type AppManifestRoleKind interface{}

// +k8s:openapi-gen=true
type AppManifestRoleKindWithPermissionSet struct {
	Kind          string                                            `json:"kind"`
	PermissionSet AppManifestRoleKindWithPermissionSetPermissionSet `json:"permissionSet"`
}

// NewAppManifestRoleKindWithPermissionSet creates a new AppManifestRoleKindWithPermissionSet object.
func NewAppManifestRoleKindWithPermissionSet() *AppManifestRoleKindWithPermissionSet {
	return &AppManifestRoleKindWithPermissionSet{
		PermissionSet: AppManifestRoleKindWithPermissionSetPermissionSetViewer,
	}
}

// OpenAPIModelName returns the OpenAPI model name for AppManifestRoleKindWithPermissionSet.
func (AppManifestRoleKindWithPermissionSet) OpenAPIModelName() string {
	return "com.github.grafana.grafana-app-sdk.app.appmanifest.v1alpha3.AppManifestRoleKindWithPermissionSet"
}

// +k8s:openapi-gen=true
type AppManifestRoleKindWithVerbs struct {
	Kind  string   `json:"kind"`
	Verbs []string `json:"verbs"`
}

// NewAppManifestRoleKindWithVerbs creates a new AppManifestRoleKindWithVerbs object.
func NewAppManifestRoleKindWithVerbs() *AppManifestRoleKindWithVerbs {
	return &AppManifestRoleKindWithVerbs{
		Verbs: []string{},
	}
}

// OpenAPIModelName returns the OpenAPI model name for AppManifestRoleKindWithVerbs.
func (AppManifestRoleKindWithVerbs) OpenAPIModelName() string {
	return "com.github.grafana.grafana-app-sdk.app.appmanifest.v1alpha3.AppManifestRoleKindWithVerbs"
}

// +k8s:openapi-gen=true
type AppManifestSpec struct {
	// AppName is the unique ID of the app
	AppName string `json:"appName"`
	// AppDisplayName is the display name of the app, which can contain any printable characters
	AppDisplayName string `json:"appDisplayName"`
	Group          string `json:"group"`
	// Versions is the list of versions for this manifest, in order.
	Versions []AppManifestManifestVersion `json:"versions"`
	// PreferredVersion is the preferred version for API use. If empty, it will use the latest from versions.
	// For CRDs, this also dictates which version is used for storage.
	PreferredVersion *string `json:"preferredVersion,omitempty"`
	// ExtraPermissions contains additional permissions needed for an app's backend component to operate.
	// Apps implicitly have all permissions for kinds they managed (defined in `kinds`).
	ExtraPermissions *AppManifestV1alpha3SpecExtraPermissions `json:"extraPermissions,omitempty"`
	// DryRunKinds dictates whether this revision should create/update CRD's from the provided kinds,
	// Or simply validate and report errors in status.resources.crds.
	// If dryRunKinds is true, CRD change validation will be skipped on ingress and reported in status instead.
	// Even if no validation errors exist, CRDs will not be created or updated for a revision with dryRunKinds=true.
	DryRunKinds *bool `json:"dryRunKinds,omitempty"`
	// Roles contains information for new user roles associated with this app.
	// It is a map of the role name (e.g. "dashboard:reader") to the set of permissions on resources managed by this app.
	Roles map[string]AppManifestRole `json:"roles,omitempty"`
	// RoleBindings binds the roles specified in Roles to groups.
	// Basic groups are "anonymous", "viewer", "editor", and "admin".
	// Additional groups are specified under "additional"
	RoleBindings *AppManifestV1alpha3SpecRoleBindings `json:"roleBindings,omitempty"`
}

// NewAppManifestSpec creates a new AppManifestSpec object.
func NewAppManifestSpec() *AppManifestSpec {
	return &AppManifestSpec{
		Versions:    []AppManifestManifestVersion{},
		DryRunKinds: (func(input bool) *bool { return &input })(false),
	}
}

// OpenAPIModelName returns the OpenAPI model name for AppManifestSpec.
func (AppManifestSpec) OpenAPIModelName() string {
	return "com.github.grafana.grafana-app-sdk.app.appmanifest.v1alpha3.AppManifestSpec"
}

// +k8s:openapi-gen=true
type AppManifestV1alpha3SpecExtraPermissions struct {
	// accessKinds is a list of KindPermission objects for accessing additional kinds provided by other apps
	AccessKinds []AppManifestKindPermission `json:"accessKinds"`
}

// NewAppManifestV1alpha3SpecExtraPermissions creates a new AppManifestV1alpha3SpecExtraPermissions object.
func NewAppManifestV1alpha3SpecExtraPermissions() *AppManifestV1alpha3SpecExtraPermissions {
	return &AppManifestV1alpha3SpecExtraPermissions{
		AccessKinds: []AppManifestKindPermission{},
	}
}

// OpenAPIModelName returns the OpenAPI model name for AppManifestV1alpha3SpecExtraPermissions.
func (AppManifestV1alpha3SpecExtraPermissions) OpenAPIModelName() string {
	return "com.github.grafana.grafana-app-sdk.app.appmanifest.v1alpha3.AppManifestV1alpha3SpecExtraPermissions"
}

// +k8s:openapi-gen=true
type AppManifestV1alpha3SpecRoleBindings struct {
	Viewer     []string            `json:"viewer,omitempty"`
	Editor     []string            `json:"editor,omitempty"`
	Admin      []string            `json:"admin,omitempty"`
	Additional map[string][]string `json:"additional,omitempty"`
}

// NewAppManifestV1alpha3SpecRoleBindings creates a new AppManifestV1alpha3SpecRoleBindings object.
func NewAppManifestV1alpha3SpecRoleBindings() *AppManifestV1alpha3SpecRoleBindings {
	return &AppManifestV1alpha3SpecRoleBindings{}
}

// OpenAPIModelName returns the OpenAPI model name for AppManifestV1alpha3SpecRoleBindings.
func (AppManifestV1alpha3SpecRoleBindings) OpenAPIModelName() string {
	return "com.github.grafana.grafana-app-sdk.app.appmanifest.v1alpha3.AppManifestV1alpha3SpecRoleBindings"
}

// +k8s:openapi-gen=true
type AppManifestManifestVersionKindScope string

const (
	AppManifestManifestVersionKindScopeNamespaced AppManifestManifestVersionKindScope = "Namespaced"
	AppManifestManifestVersionKindScopeCluster    AppManifestManifestVersionKindScope = "Cluster"
)

// OpenAPIModelName returns the OpenAPI model name for AppManifestManifestVersionKindScope.
func (AppManifestManifestVersionKindScope) OpenAPIModelName() string {
	return "com.github.grafana.grafana-app-sdk.app.appmanifest.v1alpha3.AppManifestManifestVersionKindScope"
}

// +k8s:openapi-gen=true
type AppManifestSearchFieldType string

const (
	AppManifestSearchFieldTypeString  AppManifestSearchFieldType = "string"
	AppManifestSearchFieldTypeInt64   AppManifestSearchFieldType = "int64"
	AppManifestSearchFieldTypeDouble  AppManifestSearchFieldType = "double"
	AppManifestSearchFieldTypeBoolean AppManifestSearchFieldType = "boolean"
	AppManifestSearchFieldTypeDate    AppManifestSearchFieldType = "date"
)

// OpenAPIModelName returns the OpenAPI model name for AppManifestSearchFieldType.
func (AppManifestSearchFieldType) OpenAPIModelName() string {
	return "com.github.grafana.grafana-app-sdk.app.appmanifest.v1alpha3.AppManifestSearchFieldType"
}

// +k8s:openapi-gen=true
type AppManifestSearchFieldCapabilities string

const (
	AppManifestSearchFieldCapabilitiesFilter   AppManifestSearchFieldCapabilities = "filter"
	AppManifestSearchFieldCapabilitiesText     AppManifestSearchFieldCapabilities = "text"
	AppManifestSearchFieldCapabilitiesPartial  AppManifestSearchFieldCapabilities = "partial"
	AppManifestSearchFieldCapabilitiesSort     AppManifestSearchFieldCapabilities = "sort"
	AppManifestSearchFieldCapabilitiesFacet    AppManifestSearchFieldCapabilities = "facet"
	AppManifestSearchFieldCapabilitiesRetrieve AppManifestSearchFieldCapabilities = "retrieve"
	AppManifestSearchFieldCapabilitiesUnranked AppManifestSearchFieldCapabilities = "unranked"
)

// OpenAPIModelName returns the OpenAPI model name for AppManifestSearchFieldCapabilities.
func (AppManifestSearchFieldCapabilities) OpenAPIModelName() string {
	return "com.github.grafana.grafana-app-sdk.app.appmanifest.v1alpha3.AppManifestSearchFieldCapabilities"
}

// +k8s:openapi-gen=true
type AppManifestRoleKindWithPermissionSetPermissionSet string

const (
	AppManifestRoleKindWithPermissionSetPermissionSetViewer AppManifestRoleKindWithPermissionSetPermissionSet = "viewer"
	AppManifestRoleKindWithPermissionSetPermissionSetEditor AppManifestRoleKindWithPermissionSetPermissionSet = "editor"
	AppManifestRoleKindWithPermissionSetPermissionSetAdmin  AppManifestRoleKindWithPermissionSetPermissionSet = "admin"
)

// OpenAPIModelName returns the OpenAPI model name for AppManifestRoleKindWithPermissionSetPermissionSet.
func (AppManifestRoleKindWithPermissionSetPermissionSet) OpenAPIModelName() string {
	return "com.github.grafana.grafana-app-sdk.app.appmanifest.v1alpha3.AppManifestRoleKindWithPermissionSetPermissionSet"
}
