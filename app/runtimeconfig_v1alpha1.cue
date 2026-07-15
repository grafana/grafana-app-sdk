package app

// Shared runtime-config types, hoisted to package scope so both
// runtimeConfigv1alpha1 and appRoutev1alpha1 reference them DRY.

#TLSOptions: {
	caData?:       string
	skipTLSVerify: bool | *false
}

#GroupVersion: {
	group:   string
	version: string
}

#APIServerConfig: {
	url: string
	tls: #TLSOptions
	// groupVersions this full apiserver serves. Enough to statically assemble
	// the /openapi/v3 root directory; leaf specs are proxied on demand.
	groupVersions: [...#GroupVersion]
}

#OperatorWebhookOptions: {
	conversionPath?: string
	validationPath?: string
	mutationPath?:   string
}

#OperatorConfig: {
	url:       string
	tls:       #TLSOptions
	webhooks?: #OperatorWebhookOptions
}

#PluginConfig: {
	url: string
	tls: #TLSOptions
}

runtimeConfigv1alpha1: runtimeConfigKind & {
	schema: spec: {
		mode:       "apiserver" | "plugin" | "operator"
		apiServer?: #APIServerConfig
		operator?:  #OperatorConfig
		plugin?:    #PluginConfig
	}
}
