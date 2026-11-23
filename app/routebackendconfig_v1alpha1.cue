package app

// Shared backend-config types, at package scope.

#TLSOptions: {
	caData?:       string
	skipTLSVerify: bool | *false
}

// #CommonBackendConfig is the common shape for every backend target: where to reach it and
// how to trust it. The three concrete configs embed it. GroupVersions are NOT
// carried here — they are derived from the same-name AppManifest for all modes.
#CommonBackendConfig: {
	url: string
	tls: #TLSOptions
}

#OperatorWebhookOptions: {
	conversionPath?: string
	validationPath?: string
	mutationPath?:   string
}

#OperatorConfig: {
	#CommonBackendConfig
	webhooks?: #OperatorWebhookOptions
}

#PluginConfig: {
	#CommonBackendConfig
}

routeBackendv1alpha1: routeBackendKind & {
	// A RouteBackend targets one backend, described by whichever of the three
	// configs applies. At least one of apiServer/operator/plugin must be set;
	// that "at least one" rule can't be expressed as a code-generatable schema
	// (a CUE disjunction degrades to an untyped object), so it is enforced in
	// the validating admission hook instead. Declaring validation here makes the
	// generated manifest advertise the webhook so the platform calls it.
	validation: operations: ["CREATE", "UPDATE"]
	schema: spec: {
		mode: "forward" | "plugin" | "operator"
		// Basic HTTP Proxy mode
		forward?: #CommonBackendConfig
		// HTTP based operator
		operator?:  #OperatorConfig
		// Plugin based handler
		plugin?:    #PluginConfig
	}
}
