package testing

testKind2: {
	kind: "TestKind2"
	plural: "testkind2s"
	group: "test"
	apiResource: {}
	current: "v1"
	codegen: frontend: false
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