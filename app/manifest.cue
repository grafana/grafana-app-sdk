package app

config: {
	codegen: {
		goGenPath: "app"
	}
	definitions: {
		path: "app/definitions"
	}
	kinds: {
		grouping: "group"
	}
}

manifest: {
	appName: "app-manifest"
	groupOverride: "apps.grafana.app"
	versions: {
		"v1alpha1": {
			codegen: ts: enabled: false
			kinds: [appManifestv1alpha1]
		}
		"v1alpha2": {
			codegen: ts: enabled: false
			kinds: [appManifestv1alpha2]
		}
	}
	extraPermissions: {
		accessKinds: [{
			group: "apiextensions.k8s.io",
			resource: "customresourcedefinitions",
			actions: ["get","list","create","update","delete","watch"],
		}]
	}
	roles: {
		"appmanifest:viewer": {
			title: "AppManifest Viewer"
			description: "Get, List, and Watch AppManifests"
			kinds: [{
				kind: "AppManifest",
				permissionSet: "viewer",
			}],
		},
	}
	roleBindings: {
		viewer: ["appmanifest:viewer"]
		editor: ["appmanifest:viewer"]
		admin: ["appmanifest:viewer"]
	}
}

appManifestKind: {
	kind: "AppManifest"
	scope: "Cluster"
	codegen: {
		ts: enabled: false
	}
}
