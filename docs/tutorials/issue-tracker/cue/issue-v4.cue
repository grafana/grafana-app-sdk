package kinds

import "time"

// This is our issue definition, which contains metadata about the schema, and the schema itself, as a Lineage
issue: {
	// Kind is the human-readable name which is used for generated type names.
	kind: "Issue"
	// [OPTIONAL]
	// The human-readable plural form of the "name" field.
	// Will default to <name>+"s" if not present.
	pluralName: "Issues"
	// Group determines the grouping of the kind in the API server and elsewhere.
	// This is typically the same root as the plugin ID.
	group: "issue-tracker-project"
	// apiResource is a field that indicates to the codegen, and to the CUE kind system (kindsys), that this is a kind
	// which can be expressed as an API Server resource (Custom Resource Definition or otherwise).
	// When present, it also imposes certain generation and runtime restrictions on the form of the kind's schema(s).
	// It can be left as an empty object, or the fields can be populated, as we do below,
	// to either be explicit or use non-default values (here we ware being explicit about the fields).
	apiResource: {
		// [OPTIONAL]
		// Scope is the scope of the API server resource, limited to "Namespaced" (default), or "Cluster"
		// "Namespaced" kinds have resources which live in specific namespaces, whereas
		// "Cluster" kinds' resources all exist in a global namespace and cannot be localized to a single one.
		scope: "Namespaced"
		// Validation is used when generating the manifest to indicate support for validating admission control
		// for this kind. Here, we list that we want to do validation for CREATE and UPDATE operations.
		validation: operations: ["CREATE","UPDATE"]
		// Validation is used when generating the manifest to indicate support for mutating admission control
		// for this kind. Here, we list that we want to do mutation for CREATE and UPDATE operations.
		mutation: operations: ["CREATE","UPDATE"]
	}
	// Current is the current version of the Schema.
	current: "v1"
	// Codegen is an object which provides information to the codegen tooling about what sort of code you want generated.
	codegen: {
		// [OPTIONAL]
		// fronend tells the CLI to generate front-end code (TypeScript interfaces) for the schema.
		// Will default to true if not present.
		frontend: true
		// [OPTIONAL]
		// backend tells the CLI to generate backend-end code (Go types) for the schema.
		// Will default to true if not present.
		backend: true
	}
	// versions is a map of all supported versions of this Kind, with each key being the version name.
	versions: {
		"v1": {
			// Schema is the actual shape of the object. Each schema must have the form:
			// {
			//     metadata: { ... } // optional
			//     spec: { ... }
			//     status: { ... } // optional
			// }
			// The form of schemas is subject to change prior to v1.0, and is likely to include new restrictions on non-spec top-level fields.
			schema: {
				// spec is the schema of our resource.
				// We could include `status` or `metadata` top-level fields here as well,
				// but `status` is for state information, which we don't need to track,
				// and `metadata` is for kind/schema-specific custom metadata in addition to the existing
				// common metadata, and we don't need to track any specific custom metadata.
				spec: {
					title: string
					description: string
					status: string
				}
				status: {
					processedTimestamp: string & time.Time
				}
			}
		}
	}
}
