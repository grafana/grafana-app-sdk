package kinds

// This is our issue definition, which contains metadata about the schema, and the schema itself, as a Lineage
issue: {
	// Name is the human-readable name which is used for generated type names.
	name: "Issue"
	// [OPTIONAL]
	// The human-readable plural form of the "name" field.
	// Will default to <name>+"s" if not present.
	pluralName: "Issues"
	// Group determines the grouping of the kind in the API server and elsewhere.
	// This is typically the same root as the plugin ID.
	group: "issue-tracker-project"
	// CRD is a field that indicates to the codegen, and to the CUE kind system (kindsys), that this is a kind
	// which can be expressed as an API Server resource (Custom Resource Definition or otherwise).
	// When present, it also imposes certain generation and runtime restrictions on the form of the kind's schema(s).
	// It can be left as an empty object, or the fields can be populated, as we do below,
	// to either be explicit or use non-default values (here we ware being explicit about the fields).
	crd: {
		// [OPTIONAL]
		// Scope is the scope of the API server resource, limited to "Namespaced" (default), or "Cluster"
		// "Namespaced" kinds have resources which live in specific namespaces, whereas
		// "Cluster" kinds' resources all exist in a global namespace and cannot be localized to a single one.
		scope: "Namespaced"
	}
	// [OPTIONAL]
	// currentVersion is the current version of the Schema to use for codegen.
	// Will default to the latest if not present.
	currentVersion: [0,0]
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
	// lineage defines Thema lineage for the issue. This is the collection of schemas (with different versioning)
	// that are contained by the Issue kind. In-code, we always work with the one specified by `currentVersion`,
	// but the list of multiple schemas tied to versions allow us to parse any and convert it.
	lineage: {
		// The lineage name is a machine version of the kind name above.
		// This must be specified, and must be the pascalCase of the kind's name.
		name: "issue"
		// Schemas is the list of all possible schemas (each with a distinct version) for the kind.
		// They are arranged sequentially, with each new schema being either a minor or major version up from the previous one.
		// Breaking changes, such as additional required fields, require a major version bump.
		schemas: [{
			// The first version must be [0,0], and following versions must always increment by one either the
			// major [1,0] or minor [0,1] version.
			version: [0,0]
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
			}
		}]
	}
}

// apiIssue is a version of issue which is meant for API usage, and thus has different codegen parameters and a different schema.
// Most of the top-level fields are the same as "issue," but are commented where they diverge.
apiIssue: {
	name: "Issue"
	// Note that we omit the "crd" field. This means that this kind is NOT intended to be expressed as an API server resource

	codegen: {
		// frontend is set to "true" so we generate front-end code (TypeScript interface) for this as well as back-end.
		// Since this is the model we'll be using for our API, we want a corresponding TypeScript interface.
		frontend: true
		backend: true
	}
    lineage: {
        name: "issue"
        // As this is not an API-server-expressed resource, we do not need to follow the format of:
        // {
		//     metadata: { ... } // optional
		//     spec: { ... }
		//     status: { ... } // optional
		// }
		// Instead, one object will be generated for us based on the contents of `schema`.
        schemas: {
        	version: [0,0]
        	schema: {
				id: string
				title: string
				description: string
				status: "open" | "in_progress" | "closed"
			}
        }
    }
}