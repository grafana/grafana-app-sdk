package v1alpha1

import (
	"encoding/json"
	"fmt"
	"maps"
	"strings"

	"k8s.io/kube-openapi/pkg/spec3"

	"github.com/grafana/grafana-app-sdk/app"
)

// ToManifestData is a function which converts this specific version of the AppManifestSpec (v1alpha1)
// to the generic app.ManifestData type for usage with an app.Manifest.
// nolint:gocognit,funlen,gocyclo
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
			// PrinterColumns
			if kind.AdditionalPrinterColumns != nil {
				k.AdditionalPrinterColumns = make([]app.ManifestVersionKindAdditionalPrinterColumn, len(kind.AdditionalPrinterColumns))
				for i, col := range kind.AdditionalPrinterColumns {
					translated := app.ManifestVersionKindAdditionalPrinterColumn{
						Name:     col.Name,
						Type:     col.Type,
						JSONPath: col.JsonPath,
					}
					if col.Format != nil {
						translated.Format = *col.Format
					}
					if col.Description != nil {
						translated.Description = *col.Description
					}
					if col.Priority != nil {
						copied := *col.Priority
						translated.Priority = &copied
					}
					k.AdditionalPrinterColumns[i] = translated
				}
			}
			// Schema
			if kind.Schema != nil {
				// Make a copy of the map
				kindSchema := make(map[string]any)
				maps.Copy(kindSchema, kind.Schema)

				// v1alpha1 uses a CRD-like schema, so we convert this into an OpenAPI-like schema for the ManifestData
				var err error
				k.Schema, err = app.VersionSchemaFromMap(map[string]any{
					"components": map[string]any{
						"schemas": map[string]any{
							kind.Kind: map[string]any{
								"properties": kindSchema,
								"type":       "object",
							},
						},
					},
				}, k.Kind)
				if err != nil {
					return app.ManifestData{}, err
				}
			}
			// Routes
			if len(kind.Routes) > 0 {
				k.Routes = make(map[string]spec3.PathProps)
				marshaled, err := json.Marshal(kind.Routes)
				if err != nil {
					return app.ManifestData{}, err
				}
				err = json.Unmarshal(marshaled, &k.Routes)
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
	} else if len(s.Versions) > 0 {
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

// SpecFromManifestData is a function which converts an instance of app.ManifestData
// to this specific version of the AppManifestSpec (v1alpha1). This conversion may lose data contained in the app.ManifestData
// instance as v1alpha1 is an older version of the AppManifest kind.
// nolint:gocognit,funlen
func SpecFromManifestData(data app.ManifestData) (*AppManifestSpec, error) {
	spec := AppManifestSpec{
		AppName:  data.AppName,
		Group:    data.Group,
		Versions: make([]AppManifestManifestVersion, 0),
	}
	if data.PreferredVersion != "" {
		spec.PreferredVersion = &data.PreferredVersion
	}
	// Versions
	for _, version := range data.Versions {
		ver := AppManifestManifestVersion{
			Name:   version.Name,
			Served: &version.Served,
			Kinds:  make([]AppManifestManifestVersionKind, 0),
		}
		for _, kind := range version.Kinds {
			k := AppManifestManifestVersionKind{
				Kind:             kind.Kind,
				Scope:            AppManifestManifestVersionKindScope(kind.Scope),
				SelectableFields: kind.SelectableFields,
				Conversion:       &kind.Conversion,
			}
			// Convert the ManifestData's Schema into CRD Schema for v1alpha1
			if kind.Schema != nil {
				sch, err := kind.Schema.AsCRDMap(k.Kind)
				if err != nil {
					return nil, fmt.Errorf("unable to convert %s/%s schema: %w", k.Kind, ver.Name, err)
				}
				k.Schema = sch
			}
			if kind.Plural != "" {
				k.Plural = &kind.Plural
			}
			if kind.Admission != nil {
				k.Admission = &AppManifestAdmissionCapabilities{}
				if kind.Admission.Mutation != nil {
					k.Admission.Mutation = &AppManifestMutationCapability{
						Operations: make([]AppManifestAdmissionOperation, len(kind.Admission.Mutation.Operations)),
					}
					for i := 0; i < len(kind.Admission.Mutation.Operations); i++ {
						k.Admission.Mutation.Operations[i] = AppManifestAdmissionOperation(kind.Admission.Mutation.Operations[i])
					}
				}
				if kind.Admission.Validation != nil {
					k.Admission.Validation = &AppManifestValidationCapability{
						Operations: make([]AppManifestAdmissionOperation, len(kind.Admission.Validation.Operations)),
					}
					for i := 0; i < len(kind.Admission.Validation.Operations); i++ {
						k.Admission.Validation.Operations[i] = AppManifestAdmissionOperation(kind.Admission.Validation.Operations[i])
					}
				}
			}
			if len(kind.AdditionalPrinterColumns) > 0 {
				k.AdditionalPrinterColumns = make([]AppManifestAdditionalPrinterColumns, len(kind.AdditionalPrinterColumns))
				for i := 0; i < len(kind.AdditionalPrinterColumns); i++ {
					k.AdditionalPrinterColumns[i] = AppManifestAdditionalPrinterColumns{
						Name:        kind.AdditionalPrinterColumns[i].Name,
						Type:        kind.AdditionalPrinterColumns[i].Type,
						Format:      &kind.AdditionalPrinterColumns[i].Format,
						Description: &kind.AdditionalPrinterColumns[i].Description,
						Priority:    kind.AdditionalPrinterColumns[i].Priority,
						JsonPath:    kind.AdditionalPrinterColumns[i].JSONPath,
					}
				}
			}
			// Routes
			if kind.Routes != nil {
				k.Routes = make(map[string]any)
				for path := range kind.Routes {
					k.Routes[path] = kind.Routes[path]
				}
			}
			ver.Kinds = append(ver.Kinds, k)
		}
		spec.Versions = append(spec.Versions, ver)
	}
	// Permissions
	if data.ExtraPermissions != nil && data.ExtraPermissions.AccessKinds != nil {
		spec.ExtraPermissions = &AppManifestV1alpha1SpecExtraPermissions{
			AccessKinds: make([]AppManifestKindPermission, len(data.ExtraPermissions.AccessKinds)),
		}
		for idx, access := range data.ExtraPermissions.AccessKinds {
			perm := AppManifestKindPermission{
				Group:    access.Group,
				Resource: access.Resource,
				Actions:  make([]string, len(access.Actions)),
			}
			for aidx, action := range access.Actions {
				perm.Actions[aidx] = string(action)
			}
			spec.ExtraPermissions.AccessKinds[idx] = perm
		}
	}
	// Operator Info
	if data.Operator != nil {
		spec.Operator = &AppManifestOperatorInfo{
			Url: &data.Operator.URL,
		}
		if data.Operator.Webhooks != nil {
			spec.Operator.Webhooks = &AppManifestOperatorWebhookProperties{
				ConversionPath: &data.Operator.Webhooks.ConversionPath,
				ValidationPath: &data.Operator.Webhooks.ValidationPath,
				MutationPath:   &data.Operator.Webhooks.MutationPath,
			}
		}
	}
	return &spec, nil
}
