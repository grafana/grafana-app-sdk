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
	return &AppManifestManifestKind{}
}

// +k8s:openapi-gen=true
type AppManifestManifestKindVersion struct {
	Name                     string                                `json:"name"`
	Admission                *AppManifestAdmissionCapabilities     `json:"admission,omitempty"`
	Schema                   AppManifestManifestKindVersionSchema  `json:"schema"`
	SelectableFields         []string                              `json:"selectableFields,omitempty"`
	AdditionalPrinterColumns []AppManifestAdditionalPrinterColumns `json:"additionalPrinterColumns,omitempty"`
	CustomRoutes             *AppManifestCustomRouteCapabilities   `json:"customRoutes,omitempty"`
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
	return &AppManifestValidationCapability{}
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
	return &AppManifestMutationCapability{}
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
type AppManifestCustomRouteCapabilities map[string]AppManifestCustomRoute

// +k8s:openapi-gen=true
type AppManifestCustomRoute struct {
	Summary     *string                                   `json:"summary,omitempty"`
	Description *string                                   `json:"description,omitempty"`
	Operations  *AppManifestV1alpha1CustomRouteOperations `json:"operations,omitempty"`
	Parameters  []AppManifestCustomRouteParameter         `json:"parameters,omitempty"`
}

// NewAppManifestCustomRoute creates a new AppManifestCustomRoute object.
func NewAppManifestCustomRoute() *AppManifestCustomRoute {
	return &AppManifestCustomRoute{}
}

// +k8s:openapi-gen=true
type AppManifestCustomRouteOperation struct {
	Tags        []string                                          `json:"tags,omitempty"`
	Summary     *string                                           `json:"summary,omitempty"`
	Description *string                                           `json:"description,omitempty"`
	OperationId *string                                           `json:"operationId,omitempty"`
	Deprecated  *bool                                             `json:"deprecated,omitempty"`
	Consumes    []string                                          `json:"consumes,omitempty"`
	Produces    []string                                          `json:"produces,omitempty"`
	Parameters  []AppManifestCustomRouteParameter                 `json:"parameters,omitempty"`
	Responses   *AppManifestV1alpha1CustomRouteOperationResponses `json:"responses,omitempty"`
}

// NewAppManifestCustomRouteOperation creates a new AppManifestCustomRouteOperation object.
func NewAppManifestCustomRouteOperation() *AppManifestCustomRouteOperation {
	return &AppManifestCustomRouteOperation{}
}

// +k8s:openapi-gen=true
type AppManifestCustomRouteParameter struct {
	Description     *string                            `json:"description,omitempty"`
	Name            *string                            `json:"name,omitempty"`
	In              *AppManifestCustomRouteParameterIn `json:"in,omitempty"`
	Required        *bool                              `json:"required,omitempty"`
	Schema          map[string]interface{}             `json:"schema,omitempty"`
	AllowEmptyValue *bool                              `json:"allowEmptyValue,omitempty"`
}

// NewAppManifestCustomRouteParameter creates a new AppManifestCustomRouteParameter object.
func NewAppManifestCustomRouteParameter() *AppManifestCustomRouteParameter {
	return &AppManifestCustomRouteParameter{}
}

// +k8s:openapi-gen=true
type AppManifestCustomRouteResponse struct {
	Description string                 `json:"description"`
	Schema      map[string]interface{} `json:"schema,omitempty"`
	Examples    map[string]interface{} `json:"examples,omitempty"`
}

// NewAppManifestCustomRouteResponse creates a new AppManifestCustomRouteResponse object.
func NewAppManifestCustomRouteResponse() *AppManifestCustomRouteResponse {
	return &AppManifestCustomRouteResponse{}
}

// +k8s:openapi-gen=true
type AppManifestKindPermission struct {
	Group    string   `json:"group"`
	Resource string   `json:"resource"`
	Actions  []string `json:"actions"`
}

// NewAppManifestKindPermission creates a new AppManifestKindPermission object.
func NewAppManifestKindPermission() *AppManifestKindPermission {
	return &AppManifestKindPermission{}
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
}

// NewAppManifestSpec creates a new AppManifestSpec object.
func NewAppManifestSpec() *AppManifestSpec {
	return &AppManifestSpec{
		DryRunKinds: (func(input bool) *bool { return &input })(false),
	}
}

// +k8s:openapi-gen=true
type AppManifestV1alpha1CustomRouteOperations struct {
	Get     *AppManifestCustomRouteOperation `json:"get,omitempty"`
	Put     *AppManifestCustomRouteOperation `json:"put,omitempty"`
	Post    *AppManifestCustomRouteOperation `json:"post,omitempty"`
	Delete  *AppManifestCustomRouteOperation `json:"delete,omitempty"`
	Options *AppManifestCustomRouteOperation `json:"options,omitempty"`
	Head    *AppManifestCustomRouteOperation `json:"head,omitempty"`
	Patch   *AppManifestCustomRouteOperation `json:"patch,omitempty"`
	Trace   *AppManifestCustomRouteOperation `json:"trace,omitempty"`
}

// NewAppManifestV1alpha1CustomRouteOperations creates a new AppManifestV1alpha1CustomRouteOperations object.
func NewAppManifestV1alpha1CustomRouteOperations() *AppManifestV1alpha1CustomRouteOperations {
	return &AppManifestV1alpha1CustomRouteOperations{}
}

// +k8s:openapi-gen=true
type AppManifestV1alpha1CustomRouteOperationResponses struct {
	Default             *AppManifestCustomRouteResponse `json:"default,omitempty"`
	StatusCodeResponses interface{}                     `json:"statusCodeResponses,omitempty"`
}

// NewAppManifestV1alpha1CustomRouteOperationResponses creates a new AppManifestV1alpha1CustomRouteOperationResponses object.
func NewAppManifestV1alpha1CustomRouteOperationResponses() *AppManifestV1alpha1CustomRouteOperationResponses {
	return &AppManifestV1alpha1CustomRouteOperationResponses{}
}

// +k8s:openapi-gen=true
type AppManifestV1alpha1SpecExtraPermissions struct {
	// accessKinds is a list of KindPermission objects for accessing additional kinds provided by other apps
	AccessKinds []AppManifestKindPermission `json:"accessKinds"`
}

// NewAppManifestV1alpha1SpecExtraPermissions creates a new AppManifestV1alpha1SpecExtraPermissions object.
func NewAppManifestV1alpha1SpecExtraPermissions() *AppManifestV1alpha1SpecExtraPermissions {
	return &AppManifestV1alpha1SpecExtraPermissions{}
}

// +k8s:openapi-gen=true
type AppManifestCustomRouteParameterIn string

const (
	AppManifestCustomRouteParameterInBody     AppManifestCustomRouteParameterIn = "body"
	AppManifestCustomRouteParameterInQuery    AppManifestCustomRouteParameterIn = "query"
	AppManifestCustomRouteParameterInPath     AppManifestCustomRouteParameterIn = "path"
	AppManifestCustomRouteParameterInHeader   AppManifestCustomRouteParameterIn = "header"
	AppManifestCustomRouteParameterInFormData AppManifestCustomRouteParameterIn = "formData"
)
