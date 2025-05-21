package kinds

testKind: {
	kind: "TestKind"
	plural: "testkinds"
	codegen: ts: enabled: false
	schema: {
		spec: {
			testField: string
		}
	}
}