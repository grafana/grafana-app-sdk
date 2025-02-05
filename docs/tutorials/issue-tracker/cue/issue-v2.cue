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
	// [OPTIONAL]
	// Scope is the scope of the API server resource, limited to "Namespaced" (default), or "Cluster"
	// "Namespaced" kinds have resources which live in specific namespaces, whereas
	// "Cluster" kinds' resources all exist in a global namespace and cannot be localized to a single one.
	scope: "Namespaced"
	// Current is the current version of the Schema.
	current: "v1"
	// Codegen is an object which provides information to the codegen tooling about what sort of code you want generated.
	codegen: {
		// [OPTIONAL]
	    // ts contains TypeScript code generation properties for the kind
		ts: {
			// [OPTIONAL]
			// enabled indicates whether the CLI should generate front-end TypeScript code for the kind.
			// Defaults to true if not present.
			enabled: true
		}
		// [OPTIONAL]
		// go contains go code generation properties for the kind
		go: {
			// [OPTIONAL]
			// enabled indicates whether the CLI should generate back-end go code for the kind.
			// Defaults to true if not present.
			enabled: true
		}
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
