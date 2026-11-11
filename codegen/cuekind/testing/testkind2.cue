package testing

testKind2: {
	kind: "TestKind2"
	plural: "testkind2s"
	current: "v1"
	codegen: ts: enabled: false
	listKeys: false
	fixedNames: ["current"]
	namingStrategy: {
		mode: "owner"
		owner: {
			owners: ["namespace", "user"]
			allowOrgAdmin: true
		}
	}
	search: trash: false
	extensions: provisioning: enabled: true
	versions: {
		"v1": {
			watch: {
				needs: "signal"
				metadata: {
					labels: ["app"]
					finalizers: ["testapp.ext.grafana.app/finalizer"]
					generation: true
					deletionTimestamp: true
				}
			}
			schema: {
				spec: {
					testField: string
				}
			}
		}
	}
}
