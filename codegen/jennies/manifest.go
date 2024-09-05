//nolint:dupl
package jennies

import (
	"fmt"

	"github.com/grafana/codejen"
	"github.com/grafana/grafana-app-sdk/app"
	"github.com/grafana/grafana-app-sdk/codegen"
)

type ManifestOutputEncoder func(any) ([]byte, error)

// ManifestGenerator generates a JSON/YAML App Manifest.
type ManifestGenerator struct {
	Encoder       ManifestOutputEncoder
	FileExtension string
	AppName       string
}

func (*ManifestGenerator) JennyName() string {
	return "ManifestGenerator"
}

// Generate creates one or more codec go files for the provided Kind
// nolint:dupl
func (m *ManifestGenerator) Generate(kinds ...codegen.Kind) (codejen.Files, error) {
	manifest, err := buildManifest(kinds)
	if err != nil {
		return nil, err
	}

	if m.AppName != "" {
		manifest.AppName = m.AppName
	}

	// Make into kubernetes format
	output := make(map[string]any)
	output["apiVersion"] = "apps.grafana.com/v1"
	output["kind"] = "AppManifest"
	output["metadata"] = map[string]string{
		"name": m.AppName,
	}
	output["spec"] = manifest

	files := make(codejen.Files, 0)
	bytes, err := m.Encoder(output)
	if err != nil {
		return nil, err
	}
	files = append(files, codejen.File{
		RelativePath: "manifest." + m.FileExtension,
		Data:         bytes,
		From:         []codejen.NamedJenny{m},
	})

	return files, nil
}

type ManifestGoGenerator struct {
	AppName string
}

func (g *ManifestGoGenerator) JennyName() string {
	return "ManifestGoGenerator"
}

func (g *ManifestGoGenerator) Generate(kinds ...codegen.Kind) (codejen.Files, error) {
	manifest, err := buildManifest(kinds)
	if err != nil {
		return nil, err
	}

	if g.AppName != "" {
		manifest.AppName = g.AppName
	}

	return nil, nil
}

func buildManifest(kinds []codegen.Kind) (*app.ManifestData, error) {
	manifest := app.ManifestData{
		Kinds: make([]app.ManifestKind, 0),
	}

	for _, kind := range kinds {
		if kind.Properties().APIResource == nil {
			continue
		}
		if manifest.AppName == "" {
			manifest.AppName = kind.Properties().Group
		}
		if manifest.Group == "" {
			manifest.Group = kind.Properties().APIResource.Group
		}
		if kind.Properties().APIResource.Group != manifest.Group {
			return nil, fmt.Errorf("all kinds must have the same group %q", manifest.Group)
		}

		mkind := app.ManifestKind{
			Kind:       kind.Name(),
			Scope:      kind.Properties().APIResource.Scope,
			Conversion: kind.Properties().APIResource.Conversion,
			Versions:   make([]app.ManifestKindVersion, 0),
		}

		for _, version := range kind.Versions() {
			mver := app.ManifestKindVersion{
				Name: version.Version,
			}
			if len(version.Mutation.Operations) > 0 {
				mver.Admission = &app.AdmissionCapabilities{
					Mutation: &app.MutationCapability{
						Operations: version.Mutation.Operations,
					},
				}
			}
			if len(version.Validation.Operations) > 0 {
				if mver.Admission == nil {
					mver.Admission = &app.AdmissionCapabilities{}
				}
				mver.Admission.Validation = &app.ValidationCapability{
					Operations: version.Validation.Operations,
				}
			}
			mkind.Versions = append(mkind.Versions, mver)
		}
		manifest.Kinds = append(manifest.Kinds, mkind)
	}

	return &manifest, nil
}
