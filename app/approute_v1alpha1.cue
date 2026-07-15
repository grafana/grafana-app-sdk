package app

appRoutev1alpha1: appRouteKind & {
	// Discriminated union on `mode`. Each branch is a closed definition, so a
	// route carries exactly its mode's fields — nothing leaks across modes.
	//   - apiserver: a pure proxy target. No manifest data — the remote apiserver
	//     owns its kinds/schemas/OpenAPI; we only need where to reach it and which
	//     groupVersions it serves (to assemble the /openapi/v3 root).
	//   - operator/plugin: flatten the full AppManifest spec, since the platform
	//     serves those kinds and the manifest data is authoritative.
	#apiserverRoute: {
		mode:      "apiserver"
		apiServer: #APIServerConfig
	}
	#operatorRoute: {
		mode: "operator"
		appManifestv1alpha3.schema.spec
		operator: #OperatorConfig
	}
	#pluginRoute: {
		mode: "plugin"
		appManifestv1alpha3.schema.spec
		plugin: #PluginConfig
	}

	schema: spec: #apiserverRoute | #operatorRoute | #pluginRoute
}
