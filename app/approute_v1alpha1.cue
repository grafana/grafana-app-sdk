package app

appRoutev1alpha1: appRouteKind & {
	// K8s-style tagged union: a `mode` discriminator plus optional member structs.
	// A top-level CUE disjunction (oneOf) can't be code-generated into a Go struct,
	// and another kind's spec can't be embedded as a typed field (the codegen can't
	// resolve a multi-hop selector reference), so:
	//   - purity of the union is enforced in the validating admission hook, and
	//   - manifest data is referenced by name, not embedded (no duplication/drift).
	//
	// Modes:
	//   - apiserver: only `apiServer` set. A pure proxy target — the remote apiserver
	//     owns its kinds/schemas/OpenAPI; we only need where to reach it and which
	//     groupVersions it serves (to assemble the /openapi/v3 root).
	//   - operator/plugin: `manifestName` + the respective config set. The platform
	//     serves those kinds, so the referenced AppManifest is authoritative.
	schema: spec: {
		// mode selects which member below applies.
		mode: "apiserver" | "operator" | "plugin"

		// apiserver mode
		apiServer?: #APIServerConfig

		// operator / plugin modes
		operator?: #OperatorConfig
		plugin?:   #PluginConfig

		// manifestName references the (cluster-scoped) AppManifest this route serves.
		// Required for operator/plugin modes; unused for apiserver mode.
		manifestName: string
	}
}
