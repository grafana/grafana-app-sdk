package cuekind

type KindsConfig struct {
	Grouping            string
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
