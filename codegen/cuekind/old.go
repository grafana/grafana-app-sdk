package cuekind

import (
	"cuelang.org/go/cue"

	"github.com/grafana/grafana-app-sdk/codegen"
)

type OldManifest struct {
	AppName          string                                        `json:"appName"`
	Group            string                                        `json:"group,omitempty"`
	Kinds            []OldKind                                     `json:"kinds"`
	ExtraPermissions codegen.AppManifestPropertiesExtraPermissions `json:"extraPermissions"`
	OperatorURL      *string                                       `json:"operatorURL,omitempty"`
	FullGroup        string                                        `json:"fullGroup,omitempty"`
}

type OldKind struct {
	codegen.KindProperties
	Versions map[string]OldVersion `json:"versions"`
}

type OldVersion struct {
	Version                  string                                    `json:"version"`
	Schema                   cue.Value                                 `json:"schema"`
	Served                   bool                                      `json:"served"`
	Codegen                  codegen.KindCodegenProperties             `json:"codegen"`
	SelectableFields         []string                                  `json:"selectableFields"`
	Validation               codegen.KindAdmissionCapability           `json:"validation"`
	Mutation                 codegen.KindAdmissionCapability           `json:"mutation"`
	AdditionalPrinterColumns []codegen.AdditionalPrinterColumn         `json:"additionalPrinterColumns,omitempty"`
	Routes                   map[string]map[string]codegen.CustomRoute `json:"routes,omitempty"`
}

func (m *OldManifest) toSimpleManifest() *codegen.SimpleManifest {
	vers := make(map[string]*codegen.SimpleVersion)
	pref := ""
	for _, kind := range m.Kinds {
		for _, ver := range kind.Versions {
			v, ok := vers[ver.Version]
			if !ok {
				v = &codegen.SimpleVersion{
					VersionProperties: codegen.VersionProperties{Name: ver.Version, Codegen: ver.Codegen},
				}
				vers[ver.Version] = v
			}
			if ver.Served {
				v.Served = true
			}
			v.AllKinds = append(v.AllKinds, ver.toVersionedKind(kind.KindProperties))
		}
		if kind.Current > pref {
			pref = kind.Current
		}
	}
	return &codegen.SimpleManifest{
		AppManifestProperties: codegen.AppManifestProperties{
			AppName:          m.AppName,
			Group:            m.Group,
			FullGroup:        m.FullGroup,
			ExtraPermissions: m.ExtraPermissions,
			OperatorURL:      m.OperatorURL,
			PreferredVersion: pref,
		},
		AllVersions: vers,
	}
}

func (v *OldVersion) toVersionedKind(props codegen.KindProperties) codegen.VersionedKind {
	return codegen.VersionedKind{
		Kind:                     props.Kind,
		MachineName:              props.MachineName,
		PluralName:               props.PluralName,
		PluralMachineName:        props.PluralMachineName,
		Scope:                    props.Scope,
		Validation:               props.Validation,
		Mutation:                 props.Mutation,
		Conversion:               props.Conversion,
		ConversionWebhookProps:   props.ConversionWebhookProps,
		Codegen:                  v.Codegen,
		Served:                   v.Served,
		SelectableFields:         v.SelectableFields,
		AdditionalPrinterColumns: v.AdditionalPrinterColumns,
		Schema:                   v.Schema,
		Routes:                   v.Routes,
	}
}
