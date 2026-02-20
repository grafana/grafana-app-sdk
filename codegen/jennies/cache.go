package jennies

import (
	"fmt"
	"sync"

	"github.com/getkin/kin-openapi/openapi3"

	"github.com/grafana/grafana-app-sdk/app"
	"github.com/grafana/grafana-app-sdk/codegen"
)

var (
	manifests = manifestCache{}
)

type manifestCacheEntry struct {
	withSchemas    *app.ManifestData
	withoutSchemas *app.ManifestData
	kindOpenAPI    map[string]kindVersionOpenAPI
}

type kindVersionOpenAPI struct {
	components openapi3.Components
	kindKey    string
}

type manifestCache struct {
	cache    map[string]manifestCacheEntry
	cacheMux sync.Mutex
}

// GetManifest returns a pre-computed app.ManifestData if the provided manifest has already been parsed,
// or parses and caches the manifest into app.ManifestData if not.
func (mc *manifestCache) GetManifest(manifest codegen.AppManifest, includeSchemas bool) (*app.ManifestData, error) {
	mc.cacheMux.Lock()
	defer mc.cacheMux.Unlock()
	md, err := mc.loadOrBuildManifestEntry(manifest, includeSchemas)
	if err != nil {
		return nil, err
	}
	if includeSchemas {
		return md.withSchemas, nil
	}
	return md.withoutSchemas, nil
}

func (mc *manifestCache) GetKindVersionOpenAPI(manifest codegen.AppManifest, kind string, version string) (*kindVersionOpenAPI, error) {
	mc.cacheMux.Lock()
	defer mc.cacheMux.Unlock()
	md, err := mc.loadOrBuildManifestEntry(manifest, true)
	if err != nil {
		return nil, err
	}
	key := fmt.Sprintf("%s/%s", version, kind)
	kvo, ok := md.kindOpenAPI[key]
	if ok {
		return &kvo, nil
	}
	for _, v := range md.withSchemas.Versions {
		if v.Name != version {
			continue
		}
		for _, k := range v.Kinds {
			if k.Kind != kind {
				continue
			}
			components, err := k.Schema.AsOpenAPI3()
			if err != nil {
				return nil, err
			}
			kvo = kindVersionOpenAPI{
				components: *components,
				kindKey:    k.Kind,
			}
			md.kindOpenAPI[key] = kvo
			return &kvo, nil
		}
	}
	return nil, fmt.Errorf("kind %s/%s not found", version, kind)
}

func (mc *manifestCache) loadOrBuildManifestEntry(manifest codegen.AppManifest, includeSchemas bool) (manifestCacheEntry, error) {
	if mc.cache == nil {
		mc.cache = make(map[string]manifestCacheEntry)
	}
	md, ok := mc.cache[manifest.Properties().AppName]
	if !ok {
		built, err := buildManifestData(manifest, includeSchemas)
		if err != nil {
			return manifestCacheEntry{}, err
		}
		md = manifestCacheEntry{
			kindOpenAPI: make(map[string]kindVersionOpenAPI),
		}
		if includeSchemas {
			md.withSchemas = built
		} else {
			md.withoutSchemas = built
		}
		mc.cache[manifest.Properties().AppName] = md
	}
	if includeSchemas {
		if md.withSchemas == nil {
			built, err := buildManifestData(manifest, true)
			if err != nil {
				return manifestCacheEntry{}, err
			}
			md.withSchemas = built
			mc.cache[manifest.Properties().AppName] = md
		}
		return md, nil
	}
	if md.withoutSchemas == nil {
		built, err := buildManifestData(manifest, false)
		if err != nil {
			return manifestCacheEntry{}, err
		}
		md.withoutSchemas = built
		mc.cache[manifest.Properties().AppName] = md
	}
	return md, nil
}
