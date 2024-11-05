//nolint:dupl
package jennies

import (
	"bytes"
	"fmt"
	"go/format"
	"strings"

	"github.com/grafana/codejen"

	"github.com/grafana/grafana-app-sdk/app"
	"github.com/grafana/grafana-app-sdk/codegen"
	"github.com/grafana/grafana-app-sdk/codegen/templates"
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
func (m *ManifestGenerator) Generate(appManifest codegen.AppManifest) (codejen.Files, error) {
	manifestData, err := buildManifestData(appManifest)
	if err != nil {
		return nil, err
	}

	if m.AppName != "" {
		manifestData.AppName = m.AppName
	}
	if manifestData.Group == "" {
		if len(manifestData.Kinds) > 0 {
			// API Resource kinds that have no group are not allowed, error at this point
			return nil, fmt.Errorf("all APIResource kinds must have a non-empty group")
		}
		// No kinds, make an assumption for the group name
		manifestData.Group = fmt.Sprintf("%s.ext.grafana.com", manifestData.AppName)
	}

	// Make into kubernetes format
	output := make(map[string]any)
	output["apiVersion"] = "apps.grafana.com/v1"
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
	AppName string
	Package string
}

func (*ManifestGoGenerator) JennyName() string {
	return "ManifestGoGenerator"
}

func (g *ManifestGoGenerator) Generate(appManifest codegen.AppManifest) (codejen.Files, error) {
	manifestData, err := buildManifestData(appManifest)
	if err != nil {
		return nil, err
	}

	if g.AppName != "" {
		manifestData.AppName = g.AppName
	}
	if manifestData.Group == "" {
		if len(manifestData.Kinds) > 0 {
			// API Resource kinds that have no group are not allowed, error at this point
			return nil, fmt.Errorf("all APIResource kinds must have a non-empty group")
		}
		// No kinds, make an assumption for the group name
		manifestData.Group = fmt.Sprintf("%s.ext.grafana.com", manifestData.AppName)
	}

	buf := bytes.Buffer{}
	err = templates.WriteManifestGoFile(templates.ManifestGoFileMetadata{
		Package:      g.Package,
		ManifestData: *manifestData,
	}, &buf)
	if err != nil {
		return nil, err
	}

	formatted, err := format.Source(buf.Bytes())
	if err != nil {
		return nil, err
	}
	files := make(codejen.Files, 0)
	files = append(files, codejen.File{
		Data:         formatted,
		RelativePath: "manifest.go",
		From:         []codejen.NamedJenny{g},
	})

	return files, nil
}

func buildManifestData(m codegen.AppManifest) (*app.ManifestData, error) {
	manifest := app.ManifestData{
		Kinds: make([]app.ManifestKind, 0),
	}

	manifest.AppName = m.Name()
	manifest.Group = m.Properties().Group

	for _, kind := range m.Kinds() {
		// TODO
		if manifest.AppName == "" {
			manifest.AppName = kind.Properties().Group
		}
		if manifest.Group == "" {
			manifest.Group = kind.Properties().Group
		}
		if kind.Properties().Group == "" {
			return nil, fmt.Errorf("all APIResource kinds must have a non-empty group")
		}
		if kind.Properties().Group != manifest.Group {
			return nil, fmt.Errorf("all kinds must have the same group %q", manifest.Group)
		}

		mkind := app.ManifestKind{
			Kind:       kind.Name(),
			Scope:      kind.Properties().Scope,
			Conversion: kind.Properties().Conversion,
			Versions:   make([]app.ManifestKindVersion, 0),
		}

		for _, version := range kind.Versions() {
			mver := app.ManifestKindVersion{
				Name: version.Version,
			}
			if len(version.Mutation.Operations) > 0 {
				operations, err := sanitizeAdmissionOperations(version.Mutation.Operations)
				if err != nil {
					return nil, fmt.Errorf("mutation operations error: %w", err)
				}
				mver.Admission = &app.AdmissionCapabilities{
					Mutation: &app.MutationCapability{
						Operations: operations,
					},
				}
			}
			if len(version.Validation.Operations) > 0 {
				if mver.Admission == nil {
					mver.Admission = &app.AdmissionCapabilities{}
				}
				operations, err := sanitizeAdmissionOperations(version.Validation.Operations)
				if err != nil {
					return nil, fmt.Errorf("validation operations error: %w", err)
				}
				mver.Admission.Validation = &app.ValidationCapability{
					Operations: operations,
				}
			}
			crd, err := KindVersionToCRDSpecVersion(version, mkind.Kind, true)
			if err != nil {
				return nil, err
			}
			mver.Schema, err = app.VersionSchemaFromMap(crd.Schema)
			if err != nil {
				return nil, fmt.Errorf("version schema error: %w", err)
			}
			mver.SelectableFields = version.SelectableFields
			mkind.Versions = append(mkind.Versions, mver)
		}
		manifest.Kinds = append(manifest.Kinds, mkind)
	}

	return &manifest, nil
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
