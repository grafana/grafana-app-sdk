package testing

configJson: {
	definitions: {
		manifestSchemas: true
		manifestVersion: "v1alpha1"
		path:            "codegen/testing/golden_generated/crd"
		encoding:        "json"
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
	manifestSelectors: ["customManifest", "testManifest"]
}

configYaml: {
	definitions: {
		manifestSchemas: true
		manifestVersion: "v1alpha1"
		path:            "codegen/testing/golden_generated/crd"
		encoding:        "yaml"
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
	manifestSelectors: ["customManifest", "testManifest"]
}

configKind: {
	definitions: {
		manifestSchemas: true
		manifestVersion: "v1alpha1"
		path:            "codegen/testing/golden_generated/crd"
		genManfiest:     false
		genCRDs:         false
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

configIntegrationGen1: {
	definitions: {
		manifestSchemas: true
		manifestVersion: "v1alpha1"
		encoding:        "json"
	}
	codegen: {
		goGenPath: "pkg/gen1"
		tsGenPath: "ts/gen1"
	}
	kinds: {
		grouping: "kind"
	}
	manifestSelectors: ["testManifest", "customManifest"]
}

configIntegrationGen2: {
	definitions: {
		manifestSchemas: true
		manifestVersion: "v1alpha1"
		encoding:        "yaml"
	}
	codegen: {
		goGenPath: "pkg/gen2"
		tsGenPath: "ts/gen2"
	}
	kinds: {
		grouping: "group"
	}
	manifestSelectors: ["testManifest", "customManifest"]
}

configIntegrationGen3: {
	manifestSelectors: ["customManifest"]
}
