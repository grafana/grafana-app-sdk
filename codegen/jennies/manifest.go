//nolint:dupl
package jennies

import (
	"bytes"
	"fmt"
	"go/format"
	"slices"
	"sort"
	"strings"

	"cuelang.org/go/cue"
	"github.com/grafana/codejen"
	"golang.org/x/tools/imports"
	"k8s.io/kube-openapi/pkg/spec3"

	"github.com/grafana/grafana-app-sdk/app"
	"github.com/grafana/grafana-app-sdk/codegen"
	"github.com/grafana/grafana-app-sdk/codegen/templates"

	cueformat "cuelang.org/go/cue/format"
	"k8s.io/kube-openapi/pkg/validation/spec"
)

type ManifestOutputEncoder func(any) ([]byte, error)

// ManifestGenerator generates a JSON/YAML App Manifest.
type ManifestGenerator struct {
	Encoder        ManifestOutputEncoder
	FileExtension  string
	IncludeSchemas bool
}

func (*ManifestGenerator) JennyName() string {
	return "ManifestGenerator"
}

// Generate creates one or more codec go files for the provided Kind
// nolint:dupl
func (m *ManifestGenerator) Generate(appManifest codegen.AppManifest) (codejen.Files, error) {
	manifestData, err := buildManifestData(appManifest, m.IncludeSchemas)
	if err != nil {
		return nil, err
	}

	if manifestData.Group == "" {
		if len(manifestData.Versions) > 0 {
			// API Resource kinds that have no group are not allowed, error at this point
			return nil, fmt.Errorf("all APIResource kinds must have a non-empty group")
		}
		// No kinds, make an assumption for the group name
		manifestData.Group = fmt.Sprintf("%s.ext.grafana.com", manifestData.AppName)
	}

	// Make into kubernetes format
	output := make(map[string]any)
	output["apiVersion"] = "apps.grafana.com/v1alpha1"
	output["kind"] = "AppManifest"
	output["metadata"] = map[string]string{
		"name": manifestData.AppName,
	}
	output["spec"] = manifestData

	files := make(codejen.Files, 0)
	out, err := m.Encoder(output)
	if err != nil {
		return nil, err
	}
	files = append(files, codejen.File{
		RelativePath: fmt.Sprintf("%s-manifest.%s", manifestData.AppName, m.FileExtension),
		Data:         out,
		From:         []codejen.NamedJenny{m},
	})

	return files, nil
}

type ManifestGoGenerator struct {
	Package        string
	ProjectRepo    string
	CodegenPath    string
	GroupByKind    bool
	IncludeSchemas bool
}

func (*ManifestGoGenerator) JennyName() string {
	return "ManifestGoGenerator"
}

func (g *ManifestGoGenerator) Generate(appManifest codegen.AppManifest) (codejen.Files, error) {
	manifestData, err := buildManifestData(appManifest, g.IncludeSchemas)
	if err != nil {
		return nil, err
	}

	if manifestData.Group == "" {
		if len(manifestData.Versions) > 0 {
			// API Resource kinds that have no group are not allowed, error at this point
			return nil, fmt.Errorf("all APIResource kinds must have a non-empty group")
		}
		// No kinds, make an assumption for the group name
		manifestData.Group = fmt.Sprintf("%s.ext.grafana.com", manifestData.AppName)
	}

	buf := bytes.Buffer{}
	err = templates.WriteManifestGoFile(templates.ManifestGoFileMetadata{
		Package:              g.Package,
		Repo:                 g.ProjectRepo,
		CodegenPath:          g.CodegenPath,
		KindsAreGrouped:      !g.GroupByKind,
		ManifestData:         *manifestData,
		CodegenManifestGroup: appManifest.Properties().Group,
	}, &buf)
	if err != nil {
		return nil, err
	}

	formatted, err := format.Source(buf.Bytes())
	if err != nil {
		return nil, err
	}

	formatted, err = imports.Process("", formatted, &imports.Options{
		Comments: true,
	})
	if err != nil {
		return nil, err
	}

	files := make(codejen.Files, 0)
	files = append(files, codejen.File{
		Data:         formatted,
		RelativePath: fmt.Sprintf("%s_manifest.go", appManifest.Properties().Group),
		From:         []codejen.NamedJenny{g},
	})

	return files, nil
}

//nolint:revive,gocognit
func buildManifestData(m codegen.AppManifest, includeSchemas bool) (*app.ManifestData, error) {
	manifest := app.ManifestData{
		AppName:  m.Properties().AppName,
		Group:    m.Properties().FullGroup,
		Versions: make([]app.ManifestVersion, 0),
	}

	manifest.AppName = m.Name()
	manifest.Group = m.Properties().FullGroup

	hasAnyValidation := false
	hasAnyMutation := false
	hasAnyConversion := false

	for _, version := range m.Versions() {
		ver := app.ManifestVersion{
			Name:   version.Name(),
			Served: version.Properties().Served,
			Kinds:  make([]app.ManifestVersionKind, len(version.Kinds())),
		}
		for i, kind := range version.Kinds() {
			if kind.Conversion {
				hasAnyConversion = true
			}

			mvkind, err := processKindVersion(kind, version.Name(), includeSchemas)
			if err != nil {
				return nil, err
			}
			if len(kind.Validation.Operations) > 0 {
				hasAnyValidation = true
			}
			if len(kind.Mutation.Operations) > 0 {
				hasAnyMutation = true
			}

			ver.Kinds[i] = mvkind
		}
		// routes
		if len(version.Routes().Namespaced) > 0 {
			ver.Routes.Namespaced = make(map[string]spec3.PathProps)
			for sourcePath, sourceMethodsMap := range version.Routes().Namespaced {
				targetPathProps, err := buildPathPropsFromMethods(sourcePath, sourceMethodsMap)
				if err != nil {
					return nil, fmt.Errorf("custom routes error for namespaced path '%s' on version %s: %w", sourcePath, version.Name(), err)
				}
				ver.Routes.Namespaced[sourcePath] = targetPathProps
			}
		}
		if len(version.Routes().Cluster) > 0 {
			ver.Routes.Cluster = make(map[string]spec3.PathProps)
			for sourcePath, sourceMethodsMap := range version.Routes().Cluster {
				targetPathProps, err := buildPathPropsFromMethods(sourcePath, sourceMethodsMap)
				if err != nil {
					return nil, fmt.Errorf("custom routes error for cluster path '%s' on version %s: %w", sourcePath, version.Name(), err)
				}
				ver.Routes.Cluster[sourcePath] = targetPathProps
			}
		}
		manifest.Versions = append(manifest.Versions, ver)
	}

	if len(m.Properties().ExtraPermissions.AccessKinds) > 0 {
		perms := make([]app.KindPermission, len(m.Properties().ExtraPermissions.AccessKinds))
		for i, p := range m.Properties().ExtraPermissions.AccessKinds {
			perms[i] = app.KindPermission{
				Group:    p.Group,
				Resource: p.Resource,
				Actions:  toKindPermissionActions(p.Actions),
			}
		}
		manifest.ExtraPermissions = &app.Permissions{
			AccessKinds: perms,
		}
	}

	if m.Properties().OperatorURL != nil {
		webhooks := app.ManifestOperatorWebhookProperties{}
		if hasAnyConversion {
			webhooks.ConversionPath = "/convert"
		}
		if hasAnyValidation {
			webhooks.ValidationPath = "/validate"
		}
		if hasAnyMutation {
			webhooks.MutationPath = "/mutate"
		}
		manifest.Operator = &app.ManifestOperatorInfo{
			URL:      *m.Properties().OperatorURL,
			Webhooks: &webhooks,
		}
	}

	return &manifest, nil
}

//nolint:revive
func processKindVersion(vk codegen.VersionedKind, version string, includeSchema bool) (app.ManifestVersionKind, error) {
	mver := app.ManifestVersionKind{
		Kind:       vk.Kind,
		Plural:     vk.PluralName,
		Scope:      vk.Scope,
		Conversion: vk.Conversion,
	}
	if len(vk.Mutation.Operations) > 0 {
		operations, err := sanitizeAdmissionOperations(vk.Mutation.Operations)
		if err != nil {
			return app.ManifestVersionKind{}, fmt.Errorf("mutation operations error: %w", err)
		}
		mver.Admission = &app.AdmissionCapabilities{
			Mutation: &app.MutationCapability{
				Operations: operations,
			},
		}
	}
	if len(vk.Validation.Operations) > 0 {
		if mver.Admission == nil {
			mver.Admission = &app.AdmissionCapabilities{}
		}
		operations, err := sanitizeAdmissionOperations(vk.Validation.Operations)
		if err != nil {
			return app.ManifestVersionKind{}, fmt.Errorf("validation operations error: %w", err)
		}
		mver.Admission.Validation = &app.ValidationCapability{
			Operations: operations,
		}
	}
	if len(vk.Routes) > 0 {
		mver.Routes = make(map[string]spec3.PathProps)
		for sourcePath, sourceMethodsMap := range vk.Routes {
			targetPathProps, err := buildPathPropsFromMethods(sourcePath, sourceMethodsMap)
			if err != nil {
				return app.ManifestVersionKind{}, fmt.Errorf("custom routes error for path '%s': %w", sourcePath, err)
			}
			mver.Routes[sourcePath] = targetPathProps
		}
	}
	// Only include CRD schemas if told to (there is a bug with recursive schemas and CRDs)
	if includeSchema {
		crd, err := KindVersionToCRDSpecVersion(codegen.KindVersion{
			Version:                  version,
			Schema:                   vk.Schema,
			Codegen:                  vk.Codegen,
			Served:                   vk.Served,
			SelectableFields:         vk.SelectableFields,
			Validation:               vk.Validation,
			Mutation:                 vk.Mutation,
			AdditionalPrinterColumns: vk.AdditionalPrinterColumns,
			Routes:                   vk.Routes,
		}, vk.Kind, true)
		if err != nil {
			return app.ManifestVersionKind{}, err
		}
		mver.Schema, err = app.VersionSchemaFromMap(crd.Schema)
		if err != nil {
			return app.ManifestVersionKind{}, fmt.Errorf("version schema error: %w", err)
		}
	}
	mver.SelectableFields = vk.SelectableFields
	return mver, nil
}

var validAdmissionOperations = map[codegen.KindAdmissionCapabilityOperation]app.AdmissionOperation{
	codegen.AdmissionCapabilityOperationAny:     app.AdmissionOperationAny,
	codegen.AdmissionCapabilityOperationConnect: app.AdmissionOperationConnect,
	codegen.AdmissionCapabilityOperationCreate:  app.AdmissionOperationCreate,
	codegen.AdmissionCapabilityOperationDelete:  app.AdmissionOperationDelete,
	codegen.AdmissionCapabilityOperationUpdate:  app.AdmissionOperationUpdate,
}

func sanitizeAdmissionOperations(operations []codegen.KindAdmissionCapabilityOperation) ([]app.AdmissionOperation, error) {
	sanitized := make([]app.AdmissionOperation, 0)
	for _, op := range operations {
		translated, ok := validAdmissionOperations[codegen.KindAdmissionCapabilityOperation(strings.ToUpper(string(op)))]
		if !ok {
			return nil, fmt.Errorf("invalid operation %q", op)
		}
		if translated == app.AdmissionOperationAny && len(operations) > 1 {
			return nil, fmt.Errorf("cannot use any ('*') operation alongside named operations")
		}
		sanitized = append(sanitized, translated)
	}
	return sanitized, nil
}

func toKindPermissionActions(actions []string) []app.KindPermissionAction {
	a := make([]app.KindPermissionAction, len(actions))
	for i, action := range actions {
		a[i] = app.KindPermissionAction(strings.ToLower(action))
	}
	return a
}

func buildPathPropsFromMethods(sourcePath string, sourceMethodsMap map[string]codegen.CustomRoute) (spec3.PathProps, error) {
	targetPathProps := spec3.PathProps{}
	for sourceMethod, sourceRoute := range sourceMethodsMap {
		upperMethod := strings.ToUpper(sourceMethod)
		if !slices.Contains([]string{"GET", "POST", "PUT", "DELETE", "PATCH"}, upperMethod) {
			return spec3.PathProps{}, fmt.Errorf("unhandled HTTP method '%s' defined for custom route path '%s'", sourceMethod, sourcePath)
		}

		targetParameters, err := cueSchemaToParameters(sourceRoute.Request.Query)
		if err != nil {
			return spec3.PathProps{}, fmt.Errorf("error converting query schema for %s %s: %w", sourceMethod, sourcePath, err)
		}
		targetRequestBody, err := cueSchemaToRequestBody(sourceRoute.Request.Body)
		if err != nil {
			return spec3.PathProps{}, fmt.Errorf("error converting body schema for %s %s: %w", sourceMethod, sourcePath, err)
		}
		targetResponses, err := cueSchemaToResponses(sourceRoute.Response.Schema)
		if err != nil {
			return spec3.PathProps{}, fmt.Errorf("error converting response schema for %s %s: %w", sourceMethod, sourcePath, err)
		}

		operationID := defaultRouteName(sourceMethod, sourcePath)
		if sourceRoute.Name != "" {
			operationID = sourceRoute.Name
		}

		targetOperation := &spec3.Operation{
			OperationProps: spec3.OperationProps{
				Summary:     "",
				Description: "",
				Parameters:  targetParameters,
				RequestBody: targetRequestBody,
				Responses:   targetResponses,
				OperationId: operationID,
			},
		}

		switch upperMethod {
		case "GET":
			targetPathProps.Get = targetOperation
		case "POST":
			targetPathProps.Post = targetOperation
		case "PUT":
			targetPathProps.Put = targetOperation
		case "DELETE":
			targetPathProps.Delete = targetOperation
		case "PATCH":
			targetPathProps.Patch = targetOperation
		}
	}
	return targetPathProps, nil
}

func cueSchemaToParameters(v cue.Value) ([]*spec3.Parameter, error) {
	if !v.Exists() {
		return nil, nil
	}
	if err := v.Err(); err != nil {
		return nil, fmt.Errorf("input CUE value for query params has error: %w", err)
	}

	schemaProps, err := cueSchemaToSpecSchemaProps(v)
	if err != nil {
		return nil, fmt.Errorf("error converting query param CUE schema to OpenAPI props: %w", err)
	}

	if schemaProps.Type == nil || !slices.Contains(schemaProps.Type, "object") || schemaProps.Properties == nil {
		return []*spec3.Parameter{}, nil
	}

	// Extract and sort property keys for deterministic order
	paramNames := make([]string, 0, len(schemaProps.Properties))
	for name := range schemaProps.Properties {
		paramNames = append(paramNames, name)
	}
	sort.Strings(paramNames)

	parameters := make([]*spec3.Parameter, 0, len(paramNames))
	// Iterate through sorted names
	for _, paramName := range paramNames {
		paramSchema := schemaProps.Properties[paramName] // Get schema using sorted name
		required := slices.Contains(schemaProps.Required, paramName)

		param := &spec3.Parameter{}
		param.Name = paramName
		param.In = "query"
		param.Description = paramSchema.Description
		param.Required = required
		param.Schema = &paramSchema
		parameters = append(parameters, param)
	}
	return parameters, nil
}

func cueSchemaToRequestBody(v cue.Value) (*spec3.RequestBody, error) {
	if !v.Exists() {
		return nil, nil
	}
	if err := v.Err(); err != nil {
		return nil, fmt.Errorf("input CUE value for request body has error: %w", err)
	}

	schemaProps, err := cueSchemaToSpecSchemaProps(v)
	if err != nil {
		return nil, fmt.Errorf("error converting request body CUE schema to OpenAPI props: %w", err)
	}

	requestBody := &spec3.RequestBody{
		RequestBodyProps: spec3.RequestBodyProps{
			Required:    len(schemaProps.Required) > 0,
			Description: schemaProps.Description,
			Content: map[string]*spec3.MediaType{
				"application/json": {
					MediaTypeProps: spec3.MediaTypeProps{
						Schema: &spec.Schema{SchemaProps: schemaProps},
					},
				},
			},
		},
	}
	return requestBody, nil
}

func cueSchemaToResponses(v cue.Value) (*spec3.Responses, error) {
	if !v.Exists() {
		return nil, nil
	}
	if err := v.Err(); err != nil {
		return nil, fmt.Errorf("input CUE value for response has error: %w", err)
	}

	schemaProps, err := cueSchemaToSpecSchemaProps(v)
	if err != nil {
		return nil, fmt.Errorf("error converting response CUE schema to OpenAPI props: %w", err)
	}

	response := spec3.Response{
		ResponseProps: spec3.ResponseProps{
			Description: "Default OK response",
			Content: map[string]*spec3.MediaType{
				"application/json": {
					MediaTypeProps: spec3.MediaTypeProps{
						Schema: &spec.Schema{SchemaProps: schemaProps},
					},
				},
			},
		},
	}

	responses := &spec3.Responses{
		ResponsesProps: spec3.ResponsesProps{
			Default: &response,
		},
	}
	return responses, nil
}

func findSchemaFallback(val cue.Value) (cue.Value, error) {
	if _, err := val.LookupPath(cue.MakePath(cue.Str("type"))).String(); err == nil {
		return val, nil
	}

	schemasPath := cue.MakePath(cue.Str("components"), cue.Str("schemas"))
	schemasVal := val.LookupPath(schemasPath)
	if !schemasVal.Exists() {
		return cue.Value{}, fmt.Errorf("no valid schema found")
	}

	it, err := schemasVal.Fields()
	if err != nil {
		return cue.Value{}, fmt.Errorf("error iterating schemas: %w", err)
	}

	var schemas []cue.Value
	for it.Next() {
		schemas = append(schemas, it.Value())
	}

	if len(schemas) == 0 {
		return cue.Value{}, fmt.Errorf("no schemas found")
	}
	if len(schemas) > 1 {
		return cue.Value{}, fmt.Errorf("multiple schemas found, expected single schema")
	}

	return schemas[0], nil
}

func cueSchemaToSpecSchemaProps(v cue.Value) (spec.SchemaProps, error) {
	if !v.Exists() {
		return spec.SchemaProps{}, nil
	}
	if err := v.Err(); err != nil {
		return spec.SchemaProps{}, fmt.Errorf("input CUE value has error: %w", err)
	}

	openapiAST, err := CUEValueToOpenAPI(v, CUEOpenAPIConfig{
		Name:             "_generatedSchema",
		ExpandReferences: true,
	})
	if err != nil {
		return spec.SchemaProps{}, fmt.Errorf("error generating OpenAPI AST from CUE value: %w", err)
	}

	openapiCUEVal := v.Context().BuildFile(openapiAST)
	if openapiCUEVal.Err() != nil {
		astBytes, fmtErr := cueformat.Node(openapiAST)
		var astStr string
		if fmtErr != nil {
			astStr = fmt.Sprintf(" (error formatting AST: %v)", fmtErr)
		} else {
			astStr = string(astBytes)
		}
		return spec.SchemaProps{}, fmt.Errorf("error building CUE value from OpenAPI AST: %w\nAST:\n%s", openapiCUEVal.Err(), astStr)
	}

	schemaPath := cue.MakePath(cue.Str("components"), cue.Str("schemas"), cue.Str("_generatedSchema"))
	schemaVal := openapiCUEVal.LookupPath(schemaPath)

	if !schemaVal.Exists() {
		val, err := findSchemaFallback(openapiCUEVal)
		if err != nil {
			return spec.SchemaProps{}, fmt.Errorf("schema lookup failed: %w", err)
		}
		schemaVal = val
	}

	if !schemaVal.Exists() || schemaVal.Err() != nil {
		return spec.SchemaProps{}, fmt.Errorf("could not locate generated schema definition within OpenAPI CUE value: %v\nValue:\n%s", schemaVal.Err(), CUEValueToString(openapiCUEVal))
	}

	var props spec.SchemaProps
	if err := schemaVal.Decode(&props); err != nil {
		return spec.SchemaProps{}, fmt.Errorf("error decoding schema CUE value into spec.SchemaProps: %w\nSchema Value:\n%s", err, CUEValueToString(schemaVal))
	}

	return props, nil
}
