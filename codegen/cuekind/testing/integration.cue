package testing

integrationManifest: {
    appName: "integration"
    versions: {
        "v1": integrationV1,
    }
}

integrationV1: {
    kinds: [{
        kind: "Foo"
        plural: "foos"
        schema: {
            #LinkedListNode: {
                value: string
                next?: #LinkedListNode
            }
            spec: {
                foo: string
                bar: int
                list: #LinkedListNode
            }
        }
        routes: {
            "/details": {
                "GET": {
                    name: "getDetails"
                    response: {
                        spec: {
                            elements: int
                        }
                    }
                    responseMetadata: objectMeta: true
                }
            }
        }
    }]
    routes: {
		namespaced: {
			"/foo": {
				"GET": {
					name: "getFoo",
					response: {
						foo: string
					}
					responseMetadata: typeMeta: false
				}
			}
		}
		cluster: {
			"/bar": {
				"POST": {
					name: "createBar"
					response: {
						bar: int
					}
				}
			}
		}
	}
}