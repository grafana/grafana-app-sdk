package jennies

import (
	"bytes"
	"fmt"
	"go/format"
	"path/filepath"

	"github.com/grafana/codejen"
	"golang.org/x/tools/imports"

	"github.com/grafana/grafana-app-sdk/codegen"
	"github.com/grafana/grafana-app-sdk/codegen/templates"
)

// informerPackageName is the package the per-kind informers are generated into. They are kept out of the kind's own
// package because they depend on the operator machinery, which a consumer of the types alone should not have to
// build.
const informerPackageName = "informer"

// InformerJenny generates, per kind, the kind's watch declaration and a constructor which resolves it through a
// simple.InformerFactory — the bus, periodic or watch-backed informer, whichever the deployment can provide.
type InformerJenny struct {
	// GroupByKind determines whether kinds are grouped by GroupVersionKind or just GroupVersion, matching
	// ClientJenny.
	GroupByKind bool
	// ProjectRepo is the go module the generated code lives in, for importing the kind's own package.
	ProjectRepo string
	// CodegenPath is the path within ProjectRepo the generated code is written to.
	CodegenPath        string
	SkipImportsProcess bool
}

func (*InformerJenny) JennyName() string {
	return "InformerJenny"
}

func (j *InformerJenny) Generate(appManifest codegen.AppManifest) (codejen.Files, error) {
	files := make(codejen.Files, 0)
	for version, kind := range codegen.VersionedKinds(appManifest) {
		if !kind.Codegen.Go.Enabled {
			continue
		}
		kindPath := getGeneratedPathForKind(j.GroupByKind, appManifest.Properties().Group, kind, version.Name())
		prefix := exportField(kind.Kind)
		if j.GroupByKind {
			prefix = ""
		}
		md := templates.GoInformerMetadata{
			PackageName:       informerPackageName,
			KindName:          exportField(kind.Kind),
			KindPrefix:        prefix,
			VersionPackage:    ToPackageName(version.Name()),
			VersionImportPath: filepath.Join(j.ProjectRepo, j.CodegenPath, kindPath),
		}
		if kind.Watch != nil {
			md.Needs = exportField(kind.Watch.Needs)
			applyWatchMetadata(&md, kind.Watch.Metadata)
		}

		b := bytes.Buffer{}
		if err := templates.WriteGoInformer(md, &b); err != nil {
			return nil, err
		}
		formatted, err := format.Source(b.Bytes())
		if err != nil {
			return nil, err
		}
		if !j.SkipImportsProcess {
			formatted, err = imports.Process("", formatted, &imports.Options{Comments: true})
			if err != nil {
				return nil, err
			}
		}
		files = append(files, codejen.File{
			RelativePath: filepath.Join(kindPath, informerPackageName,
				fmt.Sprintf("%s_informer_gen.go", kind.MachineName)),
			Data: formatted,
			From: []codejen.NamedJenny{j},
		})
	}
	return files, nil
}

// applyWatchMetadata copies the kind's announced metadata onto the template data. Announcing any finalizer means
// the informer can read the finalizer list, which is what the operator-side flag says.
func applyWatchMetadata(md *templates.GoInformerMetadata, watch *codegen.WatchMetadata) {
	if watch == nil {
		return
	}
	md.AnnouncedLabels = watch.Labels
	md.AnnouncedFinalizers = len(watch.Finalizers) > 0
	md.AnnouncedGeneration = watch.Generation
	md.AnnouncedDeletionTimestamp = watch.DeletionTimestamp
}
