package cuekind

import (
	"fmt"

	"cuelang.org/go/cue"
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

type ParsedConfig struct {
	Kinds                     *KindsConfig
	CustomResourceDefinitions *CRDConfig
	Codegen                   *CodegenConfig
}

// GroupKinds returns true if the config is set to group by kind
func (cfg *ParsedConfig) GroupKinds() bool {
	return cfg.Kinds.Grouping == "group"
}

func parseConfig(val cue.Value, configName string) (*ParsedConfig, error) {
	// Load config with defaults, if present in the cue config they are overidden
	cfg := &ParsedConfig{
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
	}
	configVal := val.LookupPath(cue.MakePath(cue.Str(configName)))
	if err := configVal.Err(); err != nil {
		_, _ = fmt.Printf("[WARN] Error parsing config, using defaults: %s\n", err.Error())
		return cfg, nil
	}

	err := configVal.Decode(cfg)
	if err != nil {
		return nil, err
	}

	return cfg, nil
}
