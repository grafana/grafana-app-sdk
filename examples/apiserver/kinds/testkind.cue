package kinds

testKind: {
	kind: "TestKind"
	plural: "testkinds"
	codegen: ts: enabled: false
	validation: operations: ["CREATE","UPDATE"]
	schema: {
		#FooBar: {
			foo: string
			bar?: #FooBar
		}
		spec: {
			testField: string
			foobar?: #FooBar
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