package v1alpha1

import (
	"github.com/grafana/grafana-app-sdk/app"
)

// ToManifestData is a function which converts this specific version of the AppManifestSpec (v1alpha1)
// to the generic app.ManifestData type for usage with an app.Manifest.
func (s *AppManifestSpec) ToManifestData() (app.ManifestData, error) {
	data := app.ManifestData{
		AppName: s.AppName,
		Group:   s.Group,
		Kinds:   make([]app.ManifestKind, len(s.Kinds)),
	}
	// Kinds
	for idx, kind := range s.Kinds {
		k := app.ManifestKind{
			Kind:       kind.Kind,
			Scope:      kind.Scope,
			Conversion: kind.Conversion,
			Versions:   make([]app.ManifestKindVersion, len(kind.Versions)),
		}
		for vidx, version := range kind.Versions {
			ver := app.ManifestKindVersion{
				Name:             version.Name,
				SelectableFields: version.SelectableFields,
			}
			// Admission
			if version.Admission != nil {
				adm := app.AdmissionCapabilities{}
				if version.Admission.Validation != nil {
					adm.Validation = &app.ValidationCapability{
						Operations: make([]app.AdmissionOperation, len(version.Admission.Validation.Operations)),
					}
					for oidx, op := range version.Admission.Validation.Operations {
						adm.Validation.Operations[oidx] = app.AdmissionOperation(op) // TODO: make this case-insensitive?
					}
				}
				if version.Admission.Mutation != nil {
					adm.Mutation = &app.MutationCapability{
						Operations: make([]app.AdmissionOperation, len(version.Admission.Mutation.Operations)),
					}
					for oidx, op := range version.Admission.Mutation.Operations {
						adm.Mutation.Operations[oidx] = app.AdmissionOperation(op) // TODO: make this case-insensitive?
					}
				}
				ver.Admission = &adm
			}

			if version.CustomRoutes != nil {
				ver.CustomRoutes = make(map[string]app.CustomRoute)
				for path, route := range *version.CustomRoutes {
					customRoute := app.CustomRoute{
						Summary:     route.Summary,
						Description: route.Description,
					}

					if route.Operations != nil {
						customRoute.Operations = &app.CustomRouteOperations{
							Get:    convertCustomRouteOperation(route.Operations.Get),
							Post:   convertCustomRouteOperation(route.Operations.Post),
							Put:    convertCustomRouteOperation(route.Operations.Put),
							Delete: convertCustomRouteOperation(route.Operations.Delete),
							Patch:  convertCustomRouteOperation(route.Operations.Patch),
						}
					}

					if len(route.Parameters) > 0 {
						customRoute.Parameters = convertParameters(route.Parameters)
					}

					ver.CustomRoutes[path] = customRoute
				}
			}
			// Schema
			if version.Schema != nil {
				var err error
				ver.Schema, err = app.VersionSchemaFromMap(version.Schema)
				if err != nil {
					return app.ManifestData{}, err
				}
			}
			k.Versions[vidx] = ver
		}
		data.Kinds[idx] = k
	}
	// Permissions
	if s.ExtraPermissions != nil && s.ExtraPermissions.AccessKinds != nil {
		data.ExtraPermissions = &app.Permissions{
			AccessKinds: make([]app.KindPermission, len(s.ExtraPermissions.AccessKinds)),
		}
		for idx, access := range s.ExtraPermissions.AccessKinds {
			perm := app.KindPermission{
				Group:    access.Group,
				Resource: access.Resource,
				Actions:  make([]app.KindPermissionAction, len(access.Actions)),
			}
			for aidx, action := range access.Actions {
				perm.Actions[aidx] = app.KindPermissionAction(action)
			}
			data.ExtraPermissions.AccessKinds[idx] = perm
		}
	}
	return data, nil
}

func convertParameters(params []AppManifestCustomRouteParameter) []app.CustomRouteParameter {
	result := make([]app.CustomRouteParameter, 0, len(params))
	for _, param := range params {
		parameter := app.CustomRouteParameter{
			Name:        *param.Name,
			Description: *param.Description,
			In:          app.CustomRouteParameterLocation(*param.In),
			Required:    *param.Required,
			AllowEmpty:  *param.AllowEmptyValue,
		}

		if param.Schema != nil {
			if schema, err := app.VersionSchemaFromMap(param.Schema); err == nil {
				parameter.Schema = schema
			}
		}
		result = append(result, parameter)
	}
	return result
}

func convertCustomRouteOperation(op *AppManifestCustomRouteOperation) *app.CustomRouteOperation {
	if op == nil {
		return nil
	}

	result := &app.CustomRouteOperation{
		Tags:        op.Tags,
		Summary:     op.Summary,
		Description: op.Description,
		OperationID: op.OperationId,
		Deprecated:  op.Deprecated,
		Consumes:    op.Consumes,
		Produces:    op.Produces,
	}

	if op.Parameters != nil {
		result.Parameters = convertParameters(op.Parameters)
	}

	if op.Responses != nil {
		result.Responses = convertResponses(op.Responses)
	}

	return result
}

func convertResponses(responses *AppManifestV1alpha1CustomRouteOperationResponses) *app.CustomRouteOperationResponses {
	if responses == nil {
		return nil
	}

	result := &app.CustomRouteOperationResponses{
		StatusCodeResponses: make(map[int]app.CustomRouteResponse),
	}

	if responses.Default != nil {
		result.Default = convertResponse(responses.Default)
	}

	if responses.StatusCodeResponses != nil {
		if statusCodes, ok := responses.StatusCodeResponses.(map[int]AppManifestCustomRouteResponse); ok {
			for code, resp := range statusCodes {
				result.StatusCodeResponses[code] = *convertResponse(&resp)
			}
		}
	}

	return result
}

func convertResponse(resp *AppManifestCustomRouteResponse) *app.CustomRouteResponse {
	if resp == nil {
		return nil
	}

	result := &app.CustomRouteResponse{
		Description: resp.Description,
	}

	if resp.Schema != nil {
		if schema, err := app.VersionSchemaFromMap(resp.Schema); err == nil {
			result.Schema = schema
		}
	}

	if resp.Examples != nil {
		if examples, err := app.VersionSchemaFromMap(resp.Examples); err == nil {
			result.Examples = examples
		}
	}

	return result
}
