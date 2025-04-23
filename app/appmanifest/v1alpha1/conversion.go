package v1alpha1

import "github.com/grafana/grafana-app-sdk/app"

// ToManifestData is a function which converts this specific version of the AppManifestSpec (v1alpha1)
// to the generic app.ManifestData type for usage with an app.Manifest.
// nolint:gocognit
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
	// Operator Info
	if s.Operator != nil {
		data.Operator = &app.ManifestOperatorInfo{}
		if s.Operator.Url != nil {
			data.Operator.URL = *s.Operator.Url
		}
		if s.Operator.Webhooks != nil {
			webhooks := app.ManifestOperatorWebhookProperties{}
			if s.Operator.Webhooks.ConversionPath != nil {
				webhooks.ConversionPath = *s.Operator.Webhooks.ConversionPath
			}
			if s.Operator.Webhooks.ValidationPath != nil {
				webhooks.ValidationPath = *s.Operator.Webhooks.ValidationPath
			}
			if s.Operator.Webhooks.MutationPath != nil {
				webhooks.MutationPath = *s.Operator.Webhooks.MutationPath
			}
			data.Operator.Webhooks = &webhooks
		}
	}
	return data, nil
}
