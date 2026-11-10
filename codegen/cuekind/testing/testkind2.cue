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
	ui: {
		labelKey: "testkind2"
		icon:     "apps"
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
