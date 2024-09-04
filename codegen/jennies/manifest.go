//nolint:dupl
package jennies

import (
	"github.com/grafana/codejen"

	"github.com/grafana/grafana-app-sdk/codegen"
)

// ManifestGenerator generates an in-code and a JSON/YAML App Manifest.
type ManifestGenerator struct {
	Encoding string
}

func (*ManifestGenerator) JennyName() string {
	return "ManifestGenerator"
}

// Generate creates one or more codec go files for the provided Kind
// nolint:dupl
func (m *ManifestGenerator) Generate(kinds ...codegen.Kind) (codejen.Files, error) {
	// TODO
	return nil, nil
}
