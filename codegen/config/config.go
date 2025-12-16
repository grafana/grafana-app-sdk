package config

import (
	"fmt"

	"cuelang.org/go/cue"

	"github.com/grafana/grafana-app-sdk/codegen/cuekind"
)

const (
	DefaultConfigSelector = "config"
	KindGroupingGroup     = "group"
	KindGroupingKind      = "kind"
)

type KindsConfig struct {
	Grouping       string
	PerKindVersion bool
}

type CRDConfig struct {
	IncludeInManifest bool
	Format            string
	Path              string
	UseCRDFormat      bool
}

type CodegenConfig struct {
	GoModule                       string
	GoModGenPath                   string
	GoGenPath                      string
	TsGenPath                      string
	EnableK8sPostProcessing        bool
	EnableOperatorStatusGeneration bool
}

type Config struct {
	Kinds                     *KindsConfig
	CustomResourceDefinitions *CRDConfig
	Codegen                   *CodegenConfig
	ManifestSelectors         []string
}

// Load loads configuration from the given source into a Config struct.
// HACK: Base config is only used for backwards compatibilit with CLI flags.
func Load(src any, selector string, baseConfig *Config) (*Config, error) {
	if selector == "" {
		selector = DefaultConfigSelector
	}
	if baseConfig == nil {
		baseConfig = &Config{}
	}

	var err error
	switch v := src.(type) {
	case *cuekind.Cue:
		err = baseConfig.loadFromCue(v, selector)
	default:
		// unknown source type, return baseConfig
	}
	if err != nil {
		return nil, err
	}

	return baseConfig, nil
}

// GroupKinds returns true if the config is set to group by kind
func (cfg *Config) GroupKinds() bool {
	return cfg.Kinds.Grouping == KindGroupingGroup
}

// loadFromCue loads configuration from a cuekind.Cue into the Config struct.
func (cfg *Config) loadFromCue(c *cuekind.Cue, selector string) error {
	configDef := c.Defs.LookupPath(cue.MakePath(cue.Str("Config")))
	if err := configDef.Err(); err != nil {
		return err
	}

	configVal := c.Root.LookupPath(cue.MakePath(cue.Str(selector)))
	if err := configVal.Err(); err != nil {
		_, _ = fmt.Printf("[WARN] Config selector '%s' not found in cue, using defaults\n", selector)
		return nil
	}

	configVal = configVal.Unify(configDef)
	if err := configVal.Err(); err != nil {
		return err
	}

	err := configVal.Decode(cfg)
	if err != nil {
		return err
	}

	return nil
}
