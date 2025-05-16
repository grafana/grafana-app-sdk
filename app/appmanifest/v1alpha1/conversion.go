package v1alpha1

import (
	"strings"

	"github.com/grafana/grafana-app-sdk/app"
)

// ToManifestData is a function which converts this specific version of the AppManifestSpec (v1alpha1)
// to the generic app.ManifestData type for usage with an app.Manifest.
// nolint:gocognit,funlen
func (s *AppManifestSpec) ToManifestData() (app.ManifestData, error) {
	data := app.ManifestData{
		AppName:  s.AppName,
		Group:    s.Group,
		Versions: make([]app.ManifestVersion, len(s.Versions)),
	}
	// Versions
	for idx, version := range s.Versions {
		v := app.ManifestVersion{
			Name:   version.Name,
			Served: true,
			Kinds:  make([]app.ManifestVersionKind, len(version.Kinds)),
		}
		if version.Served != nil {
			v.Served = *version.Served
		}

		for kidx, kind := range version.Kinds {
			k := app.ManifestVersionKind{
				Kind:             kind.Kind,
				Plural:           strings.ToLower(kind.Kind) + "s",
				SelectableFields: kind.SelectableFields,
				Scope:            string(kind.Scope),
			}
			if kind.Plural != nil {
				k.Plural = strings.ToLower(*kind.Plural)
			}
			if kind.Conversion != nil {
				k.Conversion = *kind.Conversion
			}
			// Admission
			if kind.Admission != nil {
				adm := app.AdmissionCapabilities{}
				if kind.Admission.Validation != nil {
					adm.Validation = &app.ValidationCapability{
						Operations: make([]app.AdmissionOperation, len(kind.Admission.Validation.Operations)),
					}
					for oidx, op := range kind.Admission.Validation.Operations {
						adm.Validation.Operations[oidx] = app.AdmissionOperation(op) // TODO: make this case-insensitive?
					}
				}
				if kind.Admission.Mutation != nil {
					adm.Mutation = &app.MutationCapability{
						Operations: make([]app.AdmissionOperation, len(kind.Admission.Mutation.Operations)),
					}
					for oidx, op := range kind.Admission.Mutation.Operations {
						adm.Mutation.Operations[oidx] = app.AdmissionOperation(op) // TODO: make this case-insensitive?
					}
				}
				k.Admission = &adm
			}
			// Schema
			if kind.Schema != nil {
				var err error
				k.Schema, err = app.VersionSchemaFromMap(kind.Schema)
				if err != nil {
					return app.ManifestData{}, err
				}
			}
			v.Kinds[kidx] = k
		}

		data.Versions[idx] = v
	}
	if s.PreferredVersion != nil {
		data.PreferredVersion = *s.PreferredVersion
	} else {
		data.PreferredVersion = s.Versions[len(s.Versions)-1].Name
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
	return data, data.Validate()
}
