package kinds

testKind: {
	kind: "TestKind"
	plural: "testkinds"
	codegen: ts: enabled: false
	validation: operations: ["CREATE","UPDATE"]
	schema: {
		spec: {
			testField: string
		}
	}
	routes: {
		"/foo": {
			"GET": {
				request: {
					query: {
						foo: string
					}
					body: {
						bar: string
					}
				}
				response: {
					status: string
				}
			}
		}
		"/bar": {
			"GET": {
				name: "GetMessage"
				response: {
					message: string
				}
			}
		}
	}
}