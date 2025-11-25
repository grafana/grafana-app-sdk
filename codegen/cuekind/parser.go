package cuekind

import (
	"embed"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"path/filepath"
	"regexp"
	"slices"
	"strings"

	"cuelang.org/go/cue"
	"cuelang.org/go/cue/cuecontext"
	"cuelang.org/go/cue/load"

	"github.com/grafana/grafana-app-sdk/codegen"
)

const DefaultManifestSelector = "manifest"

//go:embed def.cue cue.mod/module.cue
var overlayFS embed.FS

func NewParser() (*Parser, error) {
	return &Parser{}, nil
}

type Parser struct {
	loadedCUEDefinitions *cueDefinitions
}

type ParseConfig struct {
	GenOperatorState bool
	UseOldKinds      bool
}

type cueDefinitions struct {
	Kind        cue.Value
	Schema      cue.Value
	Manifest    cue.Value
	OldKind     cue.Value
	OldManifest cue.Value
}

type parser[T any] struct {
	parseFunc func(fs.FS, ...string) ([]T, error)
}

func (p *parser[T]) Parse(f fs.FS, args ...string) ([]T, error) {
	return p.parseFunc(f, args...)
}

func (p *Parser) ManifestParser(cfg ParseConfig) codegen.Parser[codegen.AppManifest] {
	return &parser[codegen.AppManifest]{
		parseFunc: func(f fs.FS, s ...string) ([]codegen.AppManifest, error) {
			if len(s) == 0 {
				s = []string{"manifest"}
			}
			manifests := make([]codegen.AppManifest, 0, len(s))
			for _, selector := range s {
				m, err := p.ParseManifest(f, selector, cfg)
				if err != nil {
					return nil, err
				}
				manifests = append(manifests, m)
			}
			return manifests, nil
		},
	}
}

// KindParser returns a Parser that returns a list of codegen.Kind.
// If useManifest is true, it will load kinds from a manifest provided by the selector(s) in Parse (or DefaultManifestSelector if no selectors are present),
// rather than loading the selector(s) as kinds.
//
//nolint:revive
func (p *Parser) KindParser(cfg ParseConfig) codegen.Parser[codegen.Kind] {
	return &parser[codegen.Kind]{
		parseFunc: func(f fs.FS, s ...string) ([]codegen.Kind, error) {
			if len(s) == 0 {
				s = []string{"manifest"}
			}
			kinds := make([]codegen.Kind, 0)
			for _, selector := range s {
				m, err := p.ParseManifest(f, selector, cfg)
				if err != nil {
					return nil, err
				}
				kinds = append(kinds, m.Kinds()...)
			}
			return kinds, nil
		},
	}
}

// ParseManifest parses ManifestSelector (or the root object if no selector is provided) as a CUE app manifest,
// returning the parsed codegen.AppManifest object or an error.
//
//nolint:funlen
func (p *Parser) ParseManifest(files fs.FS, manifestSelector string, cfg ParseConfig) (codegen.AppManifest, error) {
	// Load the FS
	// Get the module from cue.mod/module.cue
	modFile, err := files.Open("cue.mod/module.cue")
	if err != nil {
		return nil, fmt.Errorf("provided fs.FS is not a valid CUE module: error opening cue.mod/module.cue: %w", err)
	}
	defer modFile.Close()
	modFileContents, err := io.ReadAll(modFile)
	if err != nil {
		return nil, errors.New("error reading contents of cue.mod/module.cue")
	}
	cueMod := cuecontext.New().CompileString(string(modFileContents))
	if cueMod.Err() != nil {
		return nil, cueMod.Err()
	}
	modPath, _ := cueMod.LookupPath(cue.MakePath(cue.Str("module"))).String()

	overlay := make(map[string]load.Source)
	err = ToOverlay(filepath.Join("/", modPath), files, overlay)
	if err != nil {
		return nil, err
	}
	inst := load.Instances(nil, &load.Config{
		Overlay:    overlay,
		ModuleRoot: filepath.FromSlash(filepath.Join("/", modPath)),
		Module:     modPath,
		Dir:        filepath.FromSlash(filepath.Join("/", modPath)),
	})
	if len(inst) == 0 {
		return nil, errors.New("no data")
	}
	root := cuecontext.New().BuildInstance(inst[0])
	if root.Err() != nil {
		return nil, root.Err()
	}
	var val = root
	if manifestSelector != "" {
		val = root.LookupPath(cue.MakePath(cue.Str(manifestSelector)))
	}

	// Load the kind definition (this function does this only once regardless of how many times the user calls Parse())
	defs, err := p.getCUEDefinitions(cfg.GenOperatorState)
	if err != nil {
		return nil, fmt.Errorf("could not load internal kind definition: %w", err)
	}

	if cfg.UseOldKinds {
		val = val.Unify(defs.OldManifest)
	} else {
		val = val.Unify(defs.Manifest)
	}
	if val.Err() != nil {
		return nil, val.Err()
	}

	// Decode
	manifestProps := codegen.AppManifestProperties{}
	err = val.Decode(&manifestProps)
	if err != nil {
		return nil, err
	}

	manifest := &codegen.SimpleManifest{
		Props: manifestProps,
	}

	if cfg.UseOldKinds {
		err = p.parseManifestKinds(manifest, val, defs)
	} else {
		err = p.parseManifestVersions(manifest, val, defs)
	}
	if err != nil {
		return nil, err
	}

	return manifest, nil
}

func (p *Parser) parseManifestVersions(manifest *codegen.SimpleManifest, val cue.Value, defs *cueDefinitions) error {
	manifest.AllVersions = make([]codegen.Version, 0)
	versionsVal := val.LookupPath(cue.MakePath(cue.Str("versions")))
	if versionsVal.Err() != nil {
		return versionsVal.Err()
	}
	it, err := versionsVal.Fields()
	if err != nil {
		return err
	}
	for it.Next() {
		ver := it.Value()
		vProps := codegen.VersionProperties{}
		err = ver.Decode(&vProps)
		if err != nil {
			return err
		}
		version := &codegen.SimpleVersion{
			Props:    vProps,
			AllKinds: make([]codegen.VersionedKind, 0),
		}
		kinds := ver.LookupPath(cue.MakePath(cue.Str("kinds")))
		if kinds.Err() != nil {
			return kinds.Err()
		}
		kit, err := kinds.List()
		if err != nil {
			return err
		}
		for kit.Next() {
			kind, err := p.parseKind(kit.Value(), defs.Kind, defs.Schema)
			if err != nil {
				return err
			}
			version.AllKinds = append(version.AllKinds, *kind)
		}
		// custom routes
		// Parse custom routes
		version.CustomRoutes.Namespaced, err = p.parseCustomRoutes(ver.LookupPath(cue.MakePath(cue.Str("routes"), cue.Str("namespaced"))))
		if err != nil {
			return fmt.Errorf("could not parse namespaced routes: %w", err)
		}
		version.CustomRoutes.Cluster, err = p.parseCustomRoutes(ver.LookupPath(cue.MakePath(cue.Str("routes"), cue.Str("cluster"))))
		if err != nil {
			return fmt.Errorf("could not parse namespaced routes: %w", err)
		}

		manifest.AllVersions = append(manifest.AllVersions, version)
	}

	return nil
}

func (p *Parser) parseManifestKinds(manifest *codegen.SimpleManifest, val cue.Value, defs *cueDefinitions) error {
	kindsVal := val.LookupPath(cue.MakePath(cue.Str("kinds")))
	if kindsVal.Err() != nil {
		return kindsVal.Err()
	}
	it, err := kindsVal.List()
	if err != nil {
		return err
	}
	kinds := make([]codegen.Kind, 0)
	for it.Next() {
		kind, err := p.parseKindOld(it.Value(), defs.OldKind, defs.Schema)
		if err != nil {
			return err
		}
		kinds = append(kinds, kind)
	}
	// Set up the versions from the kinds
	vers := make(map[string]*codegen.SimpleVersion)
	pref := ""
	for _, kind := range kinds {
		props := kind.Properties()
		for _, ver := range kind.Versions() {
			v, ok := vers[ver.Version]
			if !ok {
				v = &codegen.SimpleVersion{
					Props: codegen.VersionProperties{
						Name:    ver.Version,
						Served:  ver.Served,
						Codegen: ver.Codegen,
					},
					AllKinds: make([]codegen.VersionedKind, 0),
				}
			}
			if ver.Served {
				v.Props.Served = true
			}
			v.AllKinds = append(v.AllKinds, codegen.VersionedKind{
				Kind:                     props.Kind,
				MachineName:              props.MachineName,
				PluralName:               props.PluralName,
				PluralMachineName:        props.PluralMachineName,
				Scope:                    props.Scope,
				Validation:               props.Validation,
				Mutation:                 props.Mutation,
				Conversion:               props.Conversion,
				ConversionWebhookProps:   props.ConversionWebhookProps,
				Codegen:                  ver.Codegen, // Version codegen is inherited from kind in kind-centric old style
				Served:                   ver.Served,
				SelectableFields:         ver.SelectableFields,
				AdditionalPrinterColumns: ver.AdditionalPrinterColumns,
				Schema:                   ver.Schema,
				Routes:                   ver.Routes,
			})
			vers[ver.Version] = v
		}
		if kind.Properties().Current > pref {
			pref = kind.Properties().Current
		}
	}
	manifest.Props.PreferredVersion = pref
	manifest.AllVersions = make([]codegen.Version, 0)
	for key := range vers {
		manifest.AllVersions = append(manifest.AllVersions, vers[key])
	}
	slices.SortFunc(manifest.AllVersions, func(a, b codegen.Version) int {
		return strings.Compare(a.Name(), b.Name())
	})
	return nil
}

func (p *Parser) parseKind(val cue.Value, kindDef, schemaDef cue.Value) (*codegen.VersionedKind, error) {
	// Start by unifying the provided cue.Value with the cue.Value that contains our Kind definition.
	// This gives us default values for all fields that weren't filled out,
	// and will create errors for required fields that may be missing.
	val = val.Unify(kindDef)
	if val.Err() != nil {
		return nil, val.Err()
	}

	// We can't simply decode the version map, because we need to extract some values as types,
	// but leave the schema value as a cue.Value. So we tell cue to decode it into a map,
	// then still need to iterate through the map and adjust values
	someKind := &codegen.VersionedKind{}
	err := val.Decode(someKind)
	if err != nil {
		return nil, err
	}

	someKind.Schema = val.LookupPath(cue.MakePath(cue.Str("schema")))

	// Normally, we would use a conditional unify in the def.cue file of kindDef,
	// but there is a bug where the conditional evaluation creates a nil vertex somewhere
	// when loading with the CLI, so this is a faster fix (TODO: long-term fix)
	someKind.Schema = someKind.Schema.Unify(schemaDef)
	if someKind.Schema.Err() != nil {
		return nil, someKind.Schema.Err()
	}

	// Parse custom routes
	someKind.Routes, err = p.parseCustomRoutes(val.LookupPath(cue.MakePath(cue.Str("routes"))))
	if err != nil {
		return nil, err
	}

	return someKind, nil
}

// getCUEDefinitions loads CUE definitions for various types if not yet loaded,
// and returns a cueDefinitions object with the CUE values for them.
// revive complains about the usage of control flag, but it's not a problem here.
// nolint:revive
func (p *Parser) getCUEDefinitions(genOperatorState bool) (*cueDefinitions, error) {
	if p.loadedCUEDefinitions != nil {
		return p.loadedCUEDefinitions, nil
	}

	kindOverlay := make(map[string]load.Source)
	err := ToOverlay("/github.com/grafana/grafana-app-sdk/codegen/cuekind", overlayFS, kindOverlay)
	if err != nil {
		return nil, err
	}
	kindInstWithDef := load.Instances(nil, &load.Config{
		Overlay:    kindOverlay,
		ModuleRoot: filepath.FromSlash("/github.com/grafana/grafana-app-sdk/codegen/cuekind"),
		Module:     "github.com/grafana/grafana-app-sdk/codegen/cuekind",
		Dir:        filepath.FromSlash("/github.com/grafana/grafana-app-sdk/codegen/cuekind"),
	})[0]
	inst := cuecontext.New().BuildInstance(kindInstWithDef)
	if inst.Err() != nil {
		return nil, inst.Err()
	}
	kindDef := inst.LookupPath(cue.MakePath(cue.Str("Kind")))
	if kindDef.Err() != nil {
		return nil, kindDef.Err()
	}

	var schemaDef cue.Value
	if genOperatorState {
		schemaDef = inst.LookupPath(cue.MakePath(cue.Str("SchemaWithOperatorState")))
		if schemaDef.Err() != nil {
			return nil, schemaDef.Err()
		}
	} else {
		schemaDef = inst.LookupPath(cue.MakePath(cue.Str("Schema")))
		if schemaDef.Err() != nil {
			return nil, schemaDef.Err()
		}
	}

	manifestDef := inst.LookupPath(cue.MakePath(cue.Str("Manifest")))
	if manifestDef.Err() != nil {
		return nil, manifestDef.Err()
	}

	oldKindDef := inst.LookupPath(cue.MakePath(cue.Str("KindOld")))
	if oldKindDef.Err() != nil {
		return nil, oldKindDef.Err()
	}

	oldManifestDef := inst.LookupPath(cue.MakePath(cue.Str("ManifestOld")))
	if oldManifestDef.Err() != nil {
		return nil, oldManifestDef.Err()
	}

	p.loadedCUEDefinitions = &cueDefinitions{
		Kind:        kindDef,
		Schema:      schemaDef,
		Manifest:    manifestDef,
		OldKind:     oldKindDef,
		OldManifest: oldManifestDef,
	}

	return p.loadedCUEDefinitions, nil
}

func ToOverlay(prefix string, vfs fs.FS, overlay map[string]load.Source) error {
	// TODO why not just stick the prefix on automatically...?
	if !filepath.IsAbs(prefix) {
		return fmt.Errorf("must provide absolute path prefix when generating cue overlay, got %q", prefix)
	}
	err := fs.WalkDir(vfs, ".", func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}

		if d.IsDir() {
			return nil
		}

		f, err := vfs.Open(path)
		if err != nil {
			return err
		}
		defer f.Close() // nolint: errcheck

		b, err := io.ReadAll(f)
		if err != nil {
			return err
		}

		overlay[filepath.Join(prefix, path)] = load.FromBytes(b)
		return nil
	})

	if err != nil {
		return err
	}

	return nil
}

func (p *Parser) parseKindOld(val cue.Value, kindDef, schemaDef cue.Value) (codegen.Kind, error) {
	// Start by unifying the provided cue.Value with the cue.Value that contains our Kind definition.
	// This gives us default values for all fields that weren't filled out,
	// and will create errors for required fields that may be missing.
	val = val.Unify(kindDef)
	if val.Err() != nil {
		return nil, val.Err()
	}

	// Decode the unified value into our collection of properties.
	props := codegen.KindProperties{}
	err := val.Decode(&props)
	if err != nil {
		return nil, err
	}

	// We can't simply decode the version map, because we need to extract some values as types,
	// but leave the schema value as a cue.Value. So we tell cue to decode it into a map,
	// then still need to iterate through the map and adjust values
	someKind := &codegen.AnyKind{
		Props:       props,
		AllVersions: make([]codegen.KindVersion, 0),
	}
	goVers := make(map[string]codegen.KindVersion)
	vers := val.LookupPath(cue.MakePath(cue.Str("versions")))
	if vers.Err() != nil {
		return nil, vers.Err()
	}
	err = vers.Decode(&goVers)
	if err != nil {
		return nil, err
	}
	for k, v := range goVers {
		v.Schema = val.LookupPath(cue.MakePath(cue.Str("versions"), cue.Str(k), cue.Str("schema")))
		if v.Schema.Err() != nil {
			return nil, v.Schema.Err()
		}
		// Normally, we would use a conditional unify in the def.cue file of kindDef,
		// but there is a bug where the conditional evaluation creates a nil vertex somewhere
		// when loading with the CLI, so this is a faster fix (TODO: long-term fix)
		v.Schema = v.Schema.Unify(schemaDef)
		if v.Schema.Err() != nil {
			return nil, v.Schema.Err()
		}

		customRoutesVal := val.LookupPath(cue.MakePath(cue.Str("versions"), cue.Str(k), cue.Str("routes")))
		v.Routes, err = p.parseCustomRoutes(customRoutesVal)
		if err != nil {
			return nil, err
		}

		someKind.AllVersions = append(someKind.AllVersions, v)
	}
	// Now we need to sort AllVersions, as map key order is random
	slices.SortFunc(someKind.AllVersions, sortVersions)
	return someKind, nil
}

func (*Parser) parseCustomRoutes(customRoutesVal cue.Value) (map[string]map[string]codegen.CustomRoute, error) {
	if !customRoutesVal.Exists() || customRoutesVal.Err() != nil {
		return nil, nil
	}
	customRoutes := make(map[string]map[string]codegen.CustomRoute)

	pathsIter, err := customRoutesVal.Fields(cue.Optional(true), cue.Definitions(false))
	if err != nil {
		return nil, fmt.Errorf("error iterating customRoutes paths: %w", err)
	}
	for pathsIter.Next() {
		pathStr := pathsIter.Selector().String()
		pathStr = strings.Trim(pathStr, `"`)
		methodsMapVal := pathsIter.Value()
		customRoutes[pathStr] = make(map[string]codegen.CustomRoute)

		methodsIter, err := methodsMapVal.Fields(cue.Optional(true), cue.Definitions(false))
		if err != nil {
			return nil, fmt.Errorf("error iterating customRoutes methods for path '%s': %w", pathStr, err)
		}
		for methodsIter.Next() {
			methodStr := methodsIter.Selector().String()
			methodStr = strings.Trim(methodStr, `"`)
			routeVal := methodsIter.Value()

			requestVal := routeVal.LookupPath(cue.MakePath(cue.Str("request")))
			var querySchema, bodySchema cue.Value
			if requestVal.Exists() && requestVal.Err() == nil {
				querySchema = requestVal.LookupPath(cue.MakePath(cue.Str("query")))
				bodySchema = requestVal.LookupPath(cue.MakePath(cue.Str("body")))
			}

			responseVal := routeVal.LookupPath(cue.MakePath(cue.Str("response")))
			var responseSchema cue.Value
			if responseVal.Exists() && responseVal.Err() == nil {
				responseSchema = responseVal
			}
			responseMetaVal := routeVal.LookupPath(cue.MakePath(cue.Str("responseMetadata")))
			responseMeta := codegen.CustomRouteResponseMetadata{}
			if responseMetaVal.Exists() && responseMetaVal.Err() == nil {
				err = responseMetaVal.Decode(&responseMeta)
				if err != nil {
					return nil, fmt.Errorf("error decoding customRoutes response metadata for path '%s': %w", pathStr, err)
				}
			}

			route := codegen.CustomRoute{
				Request: codegen.CustomRouteRequest{
					Query: querySchema,
					Body:  bodySchema,
				},
				Response: codegen.CustomRouteResponse{
					Schema:   responseSchema,
					Metadata: responseMeta,
				},
			}
			nameStrVal := routeVal.LookupPath(cue.MakePath(cue.Str("name").Optional()))
			if nameStrVal.Exists() && !nameStrVal.IsNull() {
				route.Name, _ = nameStrVal.String()
			}
			if extensions := routeVal.LookupPath(cue.MakePath(cue.Str("extensions"))); extensions.Err() == nil && extensions.Exists() {
				extMap := make(map[string]any)
				err = extensions.Decode(&extMap)
				if err != nil {
					return nil, fmt.Errorf("error decoding customRoutes extensions for path '%s': %w", pathStr, err)
				}
				if len(extMap) > 0 {
					route.Extensions = extMap
				}
			}
			customRoutes[pathStr][methodStr] = route
		}
	}
	return customRoutes, nil
}

var (
	kubeVersionMatcher  = regexp.MustCompile(`v([0-9]+)([a-z]+[0-9]+)?`)
	themaVersionMatcher = regexp.MustCompile(`v([0-9]+)\-([0-9]+)`)
)

// sortVersions is a sort function for codegen.KindVersion objects
//
//nolint:gocritic
func sortVersions(a, b codegen.KindVersion) int {
	var aparts []string
	var bparts []string
	if kubeVersionMatcher.MatchString(a.Version) {
		aparts = kubeVersionMatcher.FindStringSubmatch(a.Version)
	} else if themaVersionMatcher.MatchString(a.Version) {
		aparts = themaVersionMatcher.FindStringSubmatch(a.Version)
	} else {
		aparts = []string{a.Version}
	}
	if kubeVersionMatcher.MatchString(b.Version) {
		bparts = kubeVersionMatcher.FindStringSubmatch(b.Version)
	} else if themaVersionMatcher.MatchString(b.Version) {
		bparts = themaVersionMatcher.FindStringSubmatch(b.Version)
	} else {
		bparts = []string{b.Version}
	}
	if aparts[1] != bparts[1] {
		return strings.Compare(aparts[1], bparts[1])
	}
	if len(aparts) > 2 {
		if len(bparts) > 2 {
			return strings.Compare(aparts[2], bparts[2])
		}
		return 1
	}
	if len(bparts) > 2 {
		return -1
	}
	return 0
}
