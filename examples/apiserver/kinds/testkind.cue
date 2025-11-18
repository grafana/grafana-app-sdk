package kinds

testKind: {
	kind:   "TestKind"
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
	validation: operations: ["CREATE", "UPDATE"]
	selectableFields: [".spec.testField"]
	schema: {
		#Foo: {
			foo: string | *"foo"
			bar: #Bar
		}
		#Bar: {
			value: string | *"bar"
			baz:   #Baz
		}
		#Baz: {
			value: int | *10
		}
		spec: {
			testField: string | *"default value"
			foo:       #Foo
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
					typeMeta:   true
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
						next?:   #RecursiveType
					}
					message: string
					next?:   #RecursiveType
				}
			}
		}
	}
}

