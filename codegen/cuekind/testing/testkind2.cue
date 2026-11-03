package testing

testKind2: {
	kind: "TestKind2"
	plural: "testkind2s"
	current: "v1"
	codegen: ts: enabled: false
	listKeys: false
	search: trash: false
	extensions: {
		provisioning: enabled: true
		ui: {
			labelKey: "testkind2"
			icon:     "apps"
		}
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
