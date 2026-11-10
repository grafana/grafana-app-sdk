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
	extensions: {
		provisioning: enabled: true
		ui: {
			labelKey: "testkind2"
			icon:     "apps"
		}
		"kgatlas.ext.grafana.app": someKnowledgeGraphConfigOption: "foo"
	}
	versions: {
		"v1": {
			schema: {
				spec: {
					testField: string
				}
			}
		}
	}
}
