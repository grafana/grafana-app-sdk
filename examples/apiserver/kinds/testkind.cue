package kinds

testKind: {
	kind: "TestKind"
	plural: "testkinds"
	codegen: ts: enabled: false
	conversion: true
}

testKindv0alpha1: testKind & {
	schema: {
		spec: {
			testField: int
		}
	}
}

testKindv1alpha1: testKind & {
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
				responseMetadata: {
					typeMeta: true
					objectMeta: true
				}
			}
		}
		"/bar": {
			"GET": {
				name: "getMessage"
				response: {
					message: string
				}
			}
		}
		"/recurse": {
			"GET": {
				name: "getRecursiveResponse"
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