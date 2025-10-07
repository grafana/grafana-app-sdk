package v1alpha2

import (
	"encoding/json"
	"fmt"
	"maps"
	"strings"

	"k8s.io/kube-openapi/pkg/spec3"
	"k8s.io/kube-openapi/pkg/validation/spec"

	"github.com/grafana/grafana-app-sdk/app"
)

// ToManifestData is a function which converts this specific version of the AppManifestSpec (v1alpha2)
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
			if kind.Schemas != nil {
				toParse := make(map[string]any)
				maps.Copy(toParse, kind.Schemas)
				if _, ok := toParse[kind.Kind]; !ok && len(toParse) > 0 {
					return app.ManifestData{}, fmt.Errorf("schemas for %v must contain an entry named '%v'", kind.Kind, kind.Kind)
				}

				var err error
				k.Schema, err = app.VersionSchemaFromMap(toParse, k.Kind)
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

		// Routes
		if version.Routes != nil {
			if len(version.Routes.Namespaced) > 0 {
				v.Routes.Namespaced = make(map[string]spec3.PathProps)
				marshaled, err := json.Marshal(version.Routes.Namespaced)
				if err != nil {
					return app.ManifestData{}, err
				}
				err = json.Unmarshal(marshaled, &v.Routes.Namespaced)
				if err != nil {
					return app.ManifestData{}, err
				}
			}
			if len(version.Routes.Cluster) > 0 {
				v.Routes.Cluster = make(map[string]spec3.PathProps)
				marshaled, err := json.Marshal(version.Routes.Cluster)
				if err != nil {
					return app.ManifestData{}, err
				}
				err = json.Unmarshal(marshaled, &v.Routes.Cluster)
				if err != nil {
					return app.ManifestData{}, err
				}
			}
			if len(version.Routes.Schemas) > 0 {
				v.Routes.Schemas = make(map[string]spec.Schema)
				marshaled, err := json.Marshal(version.Routes.Schemas)
				if err != nil {
					return app.ManifestData{}, err
				}
				err = json.Unmarshal(marshaled, &v.Routes.Schemas)
				if err != nil {
					return app.ManifestData{}, err
				}
			}
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

// SpecFromManifestData is a function which converts an instance of app.ManifestData
// to this specific version of the AppManifestSpec (v1alpha1).
// nolint:gocognit,funlen
func SpecFromManifestData(data app.ManifestData) (*AppManifestSpec, error) {
	manifestSpec := AppManifestSpec{
		AppName:  data.AppName,
		Group:    data.Group,
		Versions: make([]AppManifestManifestVersion, 0),
	}
	if data.PreferredVersion != "" {
		manifestSpec.PreferredVersion = &data.PreferredVersion
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
			if kind.Schema != nil {
				k.Schemas = kind.Schema.AsOpenAPI3SchemasMap()
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
		// Routes
		if len(version.Routes.Namespaced) > 0 || len(version.Routes.Cluster) > 0 {
			ver.Routes = NewAppManifestManifestVersionRoutes()
			if len(version.Routes.Namespaced) > 0 {
				ver.Routes.Namespaced = make(map[string]any)
				for path := range version.Routes.Namespaced {
					ver.Routes.Namespaced[path] = version.Routes.Namespaced[path]
				}
			}
			if len(version.Routes.Cluster) > 0 {
				ver.Routes.Cluster = make(map[string]any)
				for path := range version.Routes.Cluster {
					ver.Routes.Cluster[path] = version.Routes.Cluster[path]
				}
			}
			if len(version.Routes.Schemas) > 0 {
				ver.Routes.Schemas = make(map[string]any)
				for path := range version.Routes.Schemas {
					ver.Routes.Schemas[path] = version.Routes.Schemas[path]
				}
			}
		}
		manifestSpec.Versions = append(manifestSpec.Versions, ver)
	}
	// Permissions
	if data.ExtraPermissions != nil && data.ExtraPermissions.AccessKinds != nil {
		manifestSpec.ExtraPermissions = &AppManifestV1alpha2SpecExtraPermissions{
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
			manifestSpec.ExtraPermissions.AccessKinds[idx] = perm
		}
	}
	// Operator Info
	if data.Operator != nil {
		manifestSpec.Operator = &AppManifestOperatorInfo{
			Url: &data.Operator.URL,
		}
		if data.Operator.Webhooks != nil {
			manifestSpec.Operator.Webhooks = &AppManifestOperatorWebhookProperties{
				ConversionPath: &data.Operator.Webhooks.ConversionPath,
				ValidationPath: &data.Operator.Webhooks.ValidationPath,
				MutationPath:   &data.Operator.Webhooks.MutationPath,
			}
		}
	}
	return &manifestSpec, nil
}
