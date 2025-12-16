package config

configA: {
	kinds: {
		grouping:       "group"
		perKindVersion: true
	}
	customResourceDefinitions: {
		includeInManifest: false
		format:            "yaml"
		path:              "custom/defs"
		useCRDFormat:      true
	}
	codegen: {
		goModule:                       "github.com/example/module"
		goModGenPath:                   "internal/mod"
		goGenPath:                      "alt/pkg/"
		tsGenPath:                      "alt/ts/"
		enableK8sPostProcessing:        true
		enableOperatorStatusGeneration: false
	}
}

configB: {
	codegen: {
		goModule: "github.com/example/module"
	}
}
