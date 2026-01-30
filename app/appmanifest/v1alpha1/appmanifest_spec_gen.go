// Code generated - EDITING IS FUTILE. DO NOT EDIT.

package v1alpha1

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
}

// NewAppManifestManifestVersion creates a new AppManifestManifestVersion object.
func NewAppManifestManifestVersion() *AppManifestManifestVersion {
	return &AppManifestManifestVersion{
		Served: (func(input bool) *bool { return &input })(true),
		Kinds:  []AppManifestManifestVersionKind{},
	}
}

// +k8s:openapi-gen=true
type AppManifestManifestVersionKind struct {
	// Kind is the name of the kind. This should begin with a capital letter and be CamelCased
	Kind string `json:"kind"`
	// Plural is the plural version of `kind`. This is optional and defaults to the kind + "s" if not present.
	Plural *string `json:"plural,omitempty"`
	// Scope dictates the scope of the kind. This field must be the same for all versions of the kind.
	// Different values will result in an error or undefined behavior.
	Scope                    AppManifestManifestVersionKindScope   `json:"scope"`
	Admission                *AppManifestAdmissionCapabilities     `json:"admission,omitempty"`
	Schema                   AppManifestManifestVersionKindSchema  `json:"schema"`
	SelectableFields         []string                              `json:"selectableFields,omitempty"`
	AdditionalPrinterColumns []AppManifestAdditionalPrinterColumns `json:"additionalPrinterColumns,omitempty"`
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
		Scope:      AppManifestManifestVersionKindScopeNamespaced,
		Conversion: (func(input bool) *bool { return &input })(false),
	}
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

// +k8s:openapi-gen=true
type AppManifestAdmissionOperation string

const (
	AppManifestAdmissionOperationCreate  AppManifestAdmissionOperation = "CREATE"
	AppManifestAdmissionOperationUpdate  AppManifestAdmissionOperation = "UPDATE"
	AppManifestAdmissionOperationDelete  AppManifestAdmissionOperation = "DELETE"
	AppManifestAdmissionOperationConnect AppManifestAdmissionOperation = "CONNECT"
	AppManifestAdmissionOperationAll     AppManifestAdmissionOperation = "*"
)

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

// +k8s:openapi-gen=true
type AppManifestManifestVersionKindSchema map[string]interface{}

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

// +k8s:openapi-gen=true
type AppManifestOperatorInfo struct {
	// URL is the URL of the operator's HTTPS endpoint, including port if non-standard (443).
	// It should be a URL which the API server can access.
	Url *string `json:"url,omitempty"`
	// Webhooks contains information about the various webhook paths.
	Webhooks *AppManifestOperatorWebhookProperties `json:"webhooks,omitempty"`
}

// NewAppManifestOperatorInfo creates a new AppManifestOperatorInfo object.
func NewAppManifestOperatorInfo() *AppManifestOperatorInfo {
	return &AppManifestOperatorInfo{}
}

// +k8s:openapi-gen=true
type AppManifestOperatorWebhookProperties struct {
	ConversionPath *string `json:"conversionPath,omitempty"`
	ValidationPath *string `json:"validationPath,omitempty"`
	MutationPath   *string `json:"mutationPath,omitempty"`
}

// NewAppManifestOperatorWebhookProperties creates a new AppManifestOperatorWebhookProperties object.
func NewAppManifestOperatorWebhookProperties() *AppManifestOperatorWebhookProperties {
	return &AppManifestOperatorWebhookProperties{
		ConversionPath: (func(input string) *string { return &input })("/convert"),
		ValidationPath: (func(input string) *string { return &input })("/validate"),
		MutationPath:   (func(input string) *string { return &input })("/mutate"),
	}
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

// +k8s:openapi-gen=true
type AppManifestSpec struct {
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
	ExtraPermissions *AppManifestV1alpha1SpecExtraPermissions `json:"extraPermissions,omitempty"`
	// DryRunKinds dictates whether this revision should create/update CRD's from the provided kinds,
	// Or simply validate and report errors in status.resources.crds.
	// If dryRunKinds is true, CRD change validation will be skipped on ingress and reported in status instead.
	// Even if no validation errors exist, CRDs will not be created or updated for a revision with dryRunKinds=true.
	DryRunKinds *bool `json:"dryRunKinds,omitempty"`
	// Operator has information about the operator being run for the app, if there is one.
	// When present, it can indicate to the API server the URL and paths for webhooks, if applicable.
	// This is only required if you run your app as an operator and any of your kinds support webhooks for validation,
	// mutation, or conversion.
	Operator *AppManifestOperatorInfo `json:"operator,omitempty"`
	// Roles contains information for new user roles associated with this app.
	// It is a map of the role name (e.g. "dashboard:reader") to the set of permissions on resources managed by this app.
	Roles map[string]AppManifestRole `json:"roles,omitempty"`
	// RoleBindings binds the roles specified in Roles to groups.
	// Basic groups are "anonymous", "viewer", "editor", and "admin".
	// Additional groups are specified under "additional"
	RoleBindings *AppManifestV1alpha1SpecRoleBindings `json:"roleBindings,omitempty"`
}

// NewAppManifestSpec creates a new AppManifestSpec object.
func NewAppManifestSpec() *AppManifestSpec {
	return &AppManifestSpec{
		Versions:    []AppManifestManifestVersion{},
		DryRunKinds: (func(input bool) *bool { return &input })(false),
	}
}

// +k8s:openapi-gen=true
type AppManifestV1alpha1SpecExtraPermissions struct {
	// accessKinds is a list of KindPermission objects for accessing additional kinds provided by other apps
	AccessKinds []AppManifestKindPermission `json:"accessKinds"`
}

// NewAppManifestV1alpha1SpecExtraPermissions creates a new AppManifestV1alpha1SpecExtraPermissions object.
func NewAppManifestV1alpha1SpecExtraPermissions() *AppManifestV1alpha1SpecExtraPermissions {
	return &AppManifestV1alpha1SpecExtraPermissions{
		AccessKinds: []AppManifestKindPermission{},
	}
}

// +k8s:openapi-gen=true
type AppManifestV1alpha1SpecRoleBindings struct {
	Anonymous  []string            `json:"anonymous,omitempty"`
	Viewer     []string            `json:"viewer,omitempty"`
	Editor     []string            `json:"editor,omitempty"`
	Admin      []string            `json:"admin,omitempty"`
	Additional map[string][]string `json:"additional,omitempty"`
}

// NewAppManifestV1alpha1SpecRoleBindings creates a new AppManifestV1alpha1SpecRoleBindings object.
func NewAppManifestV1alpha1SpecRoleBindings() *AppManifestV1alpha1SpecRoleBindings {
	return &AppManifestV1alpha1SpecRoleBindings{}
}

// +k8s:openapi-gen=true
type AppManifestManifestVersionKindScope string

const (
	AppManifestManifestVersionKindScopeNamespaced AppManifestManifestVersionKindScope = "Namespaced"
	AppManifestManifestVersionKindScopeCluster    AppManifestManifestVersionKindScope = "Cluster"
)

// +k8s:openapi-gen=true
type AppManifestRoleKindWithPermissionSetPermissionSet string

const (
	AppManifestRoleKindWithPermissionSetPermissionSetViewer AppManifestRoleKindWithPermissionSetPermissionSet = "viewer"
	AppManifestRoleKindWithPermissionSetPermissionSetEditor AppManifestRoleKindWithPermissionSetPermissionSet = "editor"
	AppManifestRoleKindWithPermissionSetPermissionSetAdmin  AppManifestRoleKindWithPermissionSetPermissionSet = "admin"
)
func (AppManifestManifestVersion) OpenAPIModelName() string {
	return "appmanifest.v1alpha1.AppManifestManifestVersion"
}
func (AppManifestManifestVersionKind) OpenAPIModelName() string {
	return "appmanifest.v1alpha1.AppManifestManifestVersionKind"
}
func (AppManifestAdmissionCapabilities) OpenAPIModelName() string {
	return "appmanifest.v1alpha1.AppManifestAdmissionCapabilities"
}
func (AppManifestValidationCapability) OpenAPIModelName() string {
	return "appmanifest.v1alpha1.AppManifestValidationCapability"
}
func (AppManifestMutationCapability) OpenAPIModelName() string {
	return "appmanifest.v1alpha1.AppManifestMutationCapability"
}
func (AppManifestAdditionalPrinterColumns) OpenAPIModelName() string {
	return "appmanifest.v1alpha1.AppManifestAdditionalPrinterColumns"
}
func (AppManifestKindPermission) OpenAPIModelName() string {
	return "appmanifest.v1alpha1.AppManifestKindPermission"
}
func (AppManifestOperatorInfo) OpenAPIModelName() string {
	return "appmanifest.v1alpha1.AppManifestOperatorInfo"
}
func (AppManifestOperatorWebhookProperties) OpenAPIModelName() string {
	return "appmanifest.v1alpha1.AppManifestOperatorWebhookProperties"
}
func (AppManifestRole) OpenAPIModelName() string {
	return "appmanifest.v1alpha1.AppManifestRole"
}
func (AppManifestRoleKindWithPermissionSet) OpenAPIModelName() string {
	return "appmanifest.v1alpha1.AppManifestRoleKindWithPermissionSet"
}
func (AppManifestRoleKindWithVerbs) OpenAPIModelName() string {
	return "appmanifest.v1alpha1.AppManifestRoleKindWithVerbs"
}
func (AppManifestSpec) OpenAPIModelName() string {
	return "appmanifest.v1alpha1.AppManifestSpec"
}
func (AppManifestV1alpha1SpecExtraPermissions) OpenAPIModelName() string {
	return "appmanifest.v1alpha1.AppManifestV1alpha1SpecExtraPermissions"
}
func (AppManifestV1alpha1SpecRoleBindings) OpenAPIModelName() string {
	return "appmanifest.v1alpha1.AppManifestV1alpha1SpecRoleBindings"
}
