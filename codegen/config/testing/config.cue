package testing

configA: {
	kinds: {
		grouping:       "group"
		perKindVersion: true
	}
	definitions: {
		manifestSchemas: false
		encoding:        "yaml"
		path:            "custom/defs"
		manifestVersion: "v1alpha1"
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
