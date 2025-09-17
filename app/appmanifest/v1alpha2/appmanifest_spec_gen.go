// Code generated - EDITING IS FUTILE. DO NOT EDIT.

package v1alpha2

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

// +k8s:openapi-gen=true
type AppManifestManifestVersionKind struct {
	// Kind is the name of the kind. This should begin with a capital letter and be CamelCased
	Kind string `json:"kind"`
	// Plural is the plural version of `kind`. This is optional and defaults to the kind + "s" if not present.
	Plural *string `json:"plural,omitempty"`
	// Scope dictates the scope of the kind. This field must be the same for all versions of the kind.
	// Different values will result in an error or undefined behavior.
	Scope     AppManifestManifestVersionKindScope `json:"scope"`
	Admission *AppManifestAdmissionCapabilities   `json:"admission,omitempty"`
	// Schemas is the components.schemas section of an OpenAPI document describing this Kind.
	// It must contain a key named the same as the `kind` field of the Kind.
	// Other fields may be present to be referenced by $ref tags in a schema,
	// and references should lead with '#/components/schemas/' just as they would in a standard OpenAPI document.
	// If route responses in the `routes` section reference a definition in '#/components/schemas',
	// that schema definition must exist in this section.
	Schemas                  map[string]interface{}                `json:"schemas"`
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
		Schemas:    map[string]interface{}{},
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
type AppManifestSpec struct {
	AppName string `json:"appName"`
	Group   string `json:"group"`
	// Versions is the list of versions for this manifest, in order.
	Versions []AppManifestManifestVersion `json:"versions"`
	// PreferredVersion is the preferred version for API use. If empty, it will use the latest from versions.
	// For CRDs, this also dictates which version is used for storage.
	PreferredVersion *string `json:"preferredVersion,omitempty"`
	// ExtraPermissions contains additional permissions needed for an app's backend component to operate.
	// Apps implicitly have all permissions for kinds they managed (defined in `kinds`).
	ExtraPermissions *AppManifestV1alpha2SpecExtraPermissions `json:"extraPermissions,omitempty"`
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
}

// NewAppManifestSpec creates a new AppManifestSpec object.
func NewAppManifestSpec() *AppManifestSpec {
	return &AppManifestSpec{
		Versions:    []AppManifestManifestVersion{},
		DryRunKinds: (func(input bool) *bool { return &input })(false),
	}
}

// +k8s:openapi-gen=true
type AppManifestV1alpha2SpecExtraPermissions struct {
	// accessKinds is a list of KindPermission objects for accessing additional kinds provided by other apps
	AccessKinds []AppManifestKindPermission `json:"accessKinds"`
}

// NewAppManifestV1alpha2SpecExtraPermissions creates a new AppManifestV1alpha2SpecExtraPermissions object.
func NewAppManifestV1alpha2SpecExtraPermissions() *AppManifestV1alpha2SpecExtraPermissions {
	return &AppManifestV1alpha2SpecExtraPermissions{
		AccessKinds: []AppManifestKindPermission{},
	}
}

// +k8s:openapi-gen=true
type AppManifestManifestVersionKindScope string

const (
	AppManifestManifestVersionKindScopeNamespaced AppManifestManifestVersionKindScope = "Namespaced"
	AppManifestManifestVersionKindScopeCluster    AppManifestManifestVersionKindScope = "Cluster"
)
