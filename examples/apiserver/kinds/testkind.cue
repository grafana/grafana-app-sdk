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
	customRoutes: {
		"/foo": {
			"GET": {
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