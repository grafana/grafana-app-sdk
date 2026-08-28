package jennies

import (
	"bytes"
	"fmt"
	"go/format"
	"path/filepath"

	"golang.org/x/tools/imports"

	"github.com/grafana/codejen"

	"github.com/grafana/grafana-app-sdk/codegen"
	"github.com/grafana/grafana-app-sdk/codegen/templates"
)

// BackendJenny generates a per-Kind `<Kind>Backend` interface, which developers can implement to
// back a Kind's storage with something other than the SDK's generic, etcd/CRD-backed store
// (for example, a SQL database, an external API, or computed data), while still being able to
// serve the Kind through the aggregated API server via apiserver.NewCustomStorage.
type BackendJenny struct {
	// GroupByKind determines whether kinds are grouped by GroupVersionKind or just GroupVersion.
	// If GroupByKind is true, generated paths are <kind>/<version>/<file>, instead of the default <version>/<file>.
	GroupByKind        bool
	SkipImportsProcess bool
}

func (*BackendJenny) JennyName() string {
	return "BackendJenny"
}

func (r *BackendJenny) Generate(appManifest codegen.AppManifest) (codejen.Files, error) {
	files := make(codejen.Files, 0, 1)
	for version, kind := range codegen.VersionedKinds(appManifest) {
		if !kind.Codegen.Go.Enabled || !kind.Codegen.Go.CustomBackend {
			continue
		}
		md := templates.GoBackendMetadata{
			PackageName: ToPackageName(version.Name()),
			KindName:    exportField(kind.Kind),
		}

		b := bytes.Buffer{}
		if err := templates.WriteGoBackend(md, &b); err != nil {
			return nil, err
		}
		formatted, err := format.Source(b.Bytes())
		if err != nil {
			return nil, err
		}
		if !r.SkipImportsProcess {
			formatted, err = imports.Process("", formatted, &imports.Options{
				Comments: true,
			})
			if err != nil {
				return nil, err
			}
		}
		files = append(files, codejen.File{
			RelativePath: filepath.Join(getGeneratedPathForKind(r.GroupByKind, appManifest.Properties().Group, kind, version.Name()), fmt.Sprintf("%s_backend_gen.go", kind.MachineName)),
			Data:         formatted,
			From:         []codejen.NamedJenny{r},
		})
	}
	return files, nil
}
