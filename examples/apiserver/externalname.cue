package core

externalName: {
	kind: "ExternalName"
	group: "core"
	apiResource: {
		groupOverride: "core.grafana.internal"
		scope: "Cluster"
	}
	codegen: {
		frontend: false
		backend: true
	}
	pluralName: "ExternalNames"
	current: "v1"
	versions: {
	    "v1": {
	        schema: {
                spec: {
					host: string
                }
	        }
	    }
	}
}
