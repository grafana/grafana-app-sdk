package testing

configJsonCustom: {
	customResourceDefinitions: {
		includeInManifest: true
		useCRDFormat:      true
		path:              "codegen/testing/golden_generated/crd"
		format:            "json"
	}
	codegen: {
		goGenPath:    "codegen/testing/golden_generated/go/groupbygroup"
		goModule:     "codegen-tests"
		goModGenPath: "pkg/generated"
		tsGenPath:    "codegen/testing/golden_generated/typescript/versioned"
	}
	kinds: {
		grouping: "group"
	}
	manifestSelectors: ["customManifest"]
}

configJsonTest: {
	customResourceDefinitions: {
		includeInManifest: true
		useCRDFormat:      true
		path:              "codegen/testing/golden_generated/crd"
		format:            "json"
	}
	codegen: {
		goGenPath:    "codegen/testing/golden_generated/go/groupbygroup"
		goModule:     "codegen-tests"
		goModGenPath: "pkg/generated"
		tsGenPath:    "codegen/testing/golden_generated/typescript/versioned"
	}
	kinds: {
		grouping: "group"
	}
	manifestSelectors: ["testManifest"]
}

configYamlTest: {
	customResourceDefinitions: {
		includeInManifest: true
		useCRDFormat:      true
		path:              "codegen/testing/golden_generated/crd"
		format:            "yaml"
	}
	codegen: {
		goGenPath:    "codegen/testing/golden_generated/go/groupbygroup"
		goModule:     "codegen-tests"
		goModGenPath: "pkg/generated"
		tsGenPath:    "codegen/testing/golden_generated/typescript/versioned"
	}
	kinds: {
		grouping: "group"
	}
	manifestSelectors: ["testManifest"]
}

configYamlCustom: {
	customResourceDefinitions: {
		includeInManifest: true
		useCRDFormat:      true
		path:              "codegen/testing/golden_generated/crd"
		format:            "yaml"
	}
	codegen: {
		goGenPath:    "codegen/testing/golden_generated/go/groupbygroup"
		goModule:     "codegen-tests"
		goModGenPath: "pkg/generated"
		tsGenPath:    "codegen/testing/golden_generated/typescript/versioned"
	}
	kinds: {
		grouping: "group"
	}
	manifestSelectors: ["customManifest"]
}

configKind: {
	customResourceDefinitions: {
		includeInManifest: true
		useCRDFormat:      true
		path:              "codegen/testing/golden_generated/crd"
		format:            "none"
	}
	codegen: {
		goGenPath:    "codegen/testing/golden_generated/go/groupbykind"
		goModule:     "codegen-tests"
		goModGenPath: "pkg/generated"
		tsGenPath:    "codegen/testing/golden_generated/typescript/versioned"
	}
	kinds: {
		grouping: "kind"
	}
	manifestSelectors: ["customManifest"]
}

configIntegrationGen1Custom: {
	customResourceDefinitions: {
		includeInManifest: true
		useCRDFormat:      true
		format:            "json"
	}
	codegen: {
		goGenPath: "pkg/gen1"
		tsGenPath: "ts/gen1"
	}
	kinds: {
		grouping: "kind"
	}
	manifestSelectors: ["customManifest"]
}

configIntegrationGen1Test: {
	customResourceDefinitions: {
		includeInManifest: true
		useCRDFormat:      true
		format:            "json"
	}
	codegen: {
		goGenPath: "pkg/gen1"
		tsGenPath: "ts/gen1"
	}
	kinds: {
		grouping: "kind"
	}
	manifestSelectors: ["testManifest"]
}

configIntegrationGen2Custom: {
	customResourceDefinitions: {
		includeInManifest: true
		useCRDFormat:      true
		format:            "yaml"
	}
	codegen: {
		goGenPath: "pkg/gen2"
		tsGenPath: "ts/gen2"
	}
	kinds: {
		grouping: "group"
	}
	manifestSelectors: ["customManifest"]
}

configIntegrationGen2Test: {
	customResourceDefinitions: {
		includeInManifest: true
		useCRDFormat:      true
		format:            "yaml"
	}
	codegen: {
		goGenPath: "pkg/gen2"
		tsGenPath: "ts/gen2"
	}
	kinds: {
		grouping: "group"
	}
	manifestSelectors: ["testManifest"]
}
