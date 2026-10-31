package testing

testKind2: {
	kind: "TestKind2"
	plural: "testkind2s"
	current: "v1"
	codegen: ts: enabled: false
	listKeys: false
	search: trash: false
	provisioning: enabled: true
	ui: {
		label:       "Test kind"
		pluralLabel: "Test kinds"
		icon:        "apps"
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
