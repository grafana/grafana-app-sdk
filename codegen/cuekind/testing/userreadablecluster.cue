package testing

userReadableClusterManifest: {
	appName:          "user-readable-cluster-app"
	preferredVersion: "v1"
	versions: {
		"v1": {
			kinds: [{
					schema: userReadableClusterKind.versions["v1"].schema
				} & userReadableClusterKind]
		}
	}
}

userReadableClusterKind: {
	kind:         "UserReadableClusterKind"
	scope:        "Cluster"
	userReadable: true
	versions: {
		"v1": {
			schema: {
				spec: {
					field1: string
				}
			}
		}
	}
}
