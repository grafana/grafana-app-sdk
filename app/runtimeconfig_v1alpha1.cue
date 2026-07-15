package app

runtimeConfigv1alpha1: runtimeConfigKind & {
	schema: {
		#TLSOptions: {
			caData?:       string
			skipTLSVerify: bool | *false
		}

		#APIServer: {
			url: string
			tls: #TLSOptions
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

		spec: {
			mode:       "apiserver" | "plugin" | "operator"
			apiServer?: #APIServer
			operator?:  #OperatorConfig
			plugin?:    #PluginConfig
		}
	}
}
