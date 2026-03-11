package jennies

import (
	"bytes"
	"go/format"
	"path/filepath"

	"github.com/grafana/codejen"

	"github.com/grafana/grafana-app-sdk/codegen"
	"github.com/grafana/grafana-app-sdk/codegen/templates"
)

// Constants is a Jenny which creates package-wide exported constants
type Constants struct {
	// GroupByKind determines whether kinds are grouped by GroupVersionKind or just GroupVersion.
	// If GroupByKind is true, generated paths are <kind>/<version>/<file>, instead of the default <version>/<file>.
	GroupByKind bool
}

func (*Constants) JennyName() string {
	return "ConstantsGenerator"
}

type constantsFileParams struct {
	group   string
	version string
	path    string
}

func (c *Constants) Generate(appManifest codegen.AppManifest) (codejen.Files, error) {
	m := make(map[string]constantsFileParams)
	for _, v := range appManifest.Versions() {
		path := filepath.Join(ToPackageName(appManifest.Properties().Group), ToPackageName(v.Name()))
		m[path] = constantsFileParams{
			group:   appManifest.Properties().FullGroup,
			version: v.Name(),
			path:    filepath.Join(path, "constants.go"),
		}
	}
	for v, k := range codegen.VersionedKinds(appManifest) {
		path := GetGeneratedGoTypePath(c.GroupByKind, appManifest.Properties().Group, v.Name(), k.MachineName)
		if _, ok := m[path]; !ok {
			m[path] = constantsFileParams{
				group:   appManifest.Properties().FullGroup,
				version: v.Name(),
				path:    filepath.Join(path, "constants.go"),
			}
		}
	}
	files := make(codejen.Files, 0)
	for _, v := range m {
		b := bytes.Buffer{}
		err := templates.WriteConstantsFile(templates.ConstantsMetadata{
			Package: ToPackageName(v.version),
			Group:   v.group,
			Version: v.version,
		}, &b)
		if err != nil {
			return nil, err
		}
		formatted, err := format.Source(b.Bytes())
		if err != nil {
			return nil, err
		}
		files = append(files, codejen.File{
			RelativePath: v.path,
			From:         []codejen.NamedJenny{c},
			Data:         formatted,
		})
	}
	return files, nil
}
