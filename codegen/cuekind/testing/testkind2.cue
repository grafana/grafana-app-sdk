package testing

testKind2: {
	kind: "TestKind2"
	plural: "testkind2s"
	current: "v1"
	codegen: ts: enabled: false
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