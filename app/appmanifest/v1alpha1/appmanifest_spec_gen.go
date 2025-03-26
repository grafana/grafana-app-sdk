// Code generated - EDITING IS FUTILE. DO NOT EDIT.

package v1alpha1

// +k8s:openapi-gen=true
type AppManifestManifestKind struct {
	Kind       string                           `json:"kind"`
	Scope      string                           `json:"scope"`
	Conversion bool                             `json:"conversion"`
	Versions   []AppManifestManifestKindVersion `json:"versions"`
}

// NewAppManifestManifestKind creates a new AppManifestManifestKind object.
func NewAppManifestManifestKind() *AppManifestManifestKind {
	return &AppManifestManifestKind{
		Versions: []AppManifestManifestKindVersion{},
	}
}

// +k8s:openapi-gen=true
type AppManifestManifestKindVersion struct {
	Name                     string                                `json:"name"`
	Admission                *AppManifestAdmissionCapabilities     `json:"admission,omitempty"`
	Schema                   AppManifestManifestKindVersionSchema  `json:"schema"`
	SelectableFields         []string                              `json:"selectableFields,omitempty"`
	AdditionalPrinterColumns []AppManifestAdditionalPrinterColumns `json:"additionalPrinterColumns,omitempty"`
}

// NewAppManifestManifestKindVersion creates a new AppManifestManifestKindVersion object.
func NewAppManifestManifestKindVersion() *AppManifestManifestKindVersion {
	return &AppManifestManifestKindVersion{}
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
type AppManifestManifestKindVersionSchema map[string]interface{}

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
type AppManifestSpec struct {
	AppName string                    `json:"appName"`
	Group   string                    `json:"group"`
	Kinds   []AppManifestManifestKind `json:"kinds"`
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
}

// NewAppManifestSpec creates a new AppManifestSpec object.
func NewAppManifestSpec() *AppManifestSpec {
	return &AppManifestSpec{
		Kinds:       []AppManifestManifestKind{},
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
