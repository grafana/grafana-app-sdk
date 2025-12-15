package config

import (
	"errors"
	"fmt"

	"cuelang.org/go/cue"
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
	ManifestSelector          string
}

// GroupKinds returns true if the config is set to group by kind
func (cfg *Config) GroupKinds() bool {
	return cfg.Kinds.Grouping == "group"
}

// Load loads configuration from the given source into a Config struct.
// HACK: Base config is only used for backwards compatibilit with CLI flags.
func Load(src any, selector string, baseConfig *Config) (*Config, error) {
	if selector == "" {
		selector = DefaultConfigSelector
	}
	if baseConfig == nil {
		baseConfig = NewDefaultConfig()
	}

	var err error
	switch v := src.(type) {
	case cue.Value:
		err = baseConfig.loadFromCueValue(v, selector)
	default:
		// unknown source type, return baseConfig
	}
	if err != nil {
		return nil, err
	}

	if baseConfig.Kinds.Grouping != KindGroupingGroup && baseConfig.Kinds.Grouping != KindGroupingKind {
		return nil, errors.New("grouping must be one of 'group'|'kind'")
	}

	return baseConfig, nil
}

func (cfg *Config) loadFromCueValue(val cue.Value, selector string) error {
	configVal := val.LookupPath(cue.MakePath(cue.Str(selector)))
	if err := configVal.Err(); err != nil {
		_, _ = fmt.Printf("[WARN] Error parsing config from cue, using defaults: %s\n", err.Error())
		return nil
	}

	err := configVal.Decode(cfg)
	if err != nil {
		return err
	}

	return nil
}

func NewDefaultConfig() *Config {
	return &Config{
		Kinds: &KindsConfig{
			Grouping:       "kind",
			PerKindVersion: false,
		},
		CustomResourceDefinitions: &CRDConfig{
			IncludeInManifest: true,
			Format:            "json",
			Path:              "definitions",
			UseCRDFormat:      false,
		},
		Codegen: &CodegenConfig{
			GoGenPath:                      "pkg/generated/",
			TsGenPath:                      "plugin/src/generated/",
			EnableK8sPostProcessing:        false,
			GoModule:                       "",
			GoModGenPath:                   "",
			EnableOperatorStatusGeneration: true,
		},
		ManifestSelector: "manifest",
	}
}
