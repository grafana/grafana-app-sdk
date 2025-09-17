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
		mysubresource: {
			extraValue: string
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
		"/recurse": {
			"GET": {
				name: "GetRecursiveResponse"
				response: {
					#RecursiveType: {
						message: string
						next?: #RecursiveType
					}
					message: string
					next?: #RecursiveType
				}
			}
		}
	}
}