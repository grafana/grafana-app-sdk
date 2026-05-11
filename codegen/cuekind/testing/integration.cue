package testing

integrationManifest: {
	appName: "integration"
	versions: {
		"v1": integrationV1
	}
}

integrationV1: {
	kinds: [{
		kind:   "Foo"
		plural: "foos"
		schema: {
			#LinkedListNode: {
				value: string
				next?: #LinkedListNode
			}
			spec: {
				foo:  string
				bar:  int
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
	}, {
		// Selectable fields through a discriminated union (named definitions).
		kind:   "Notification"
		plural: "notifications"
		schema: {
			#RoutingType: "Direct" | "Tree"
			#DirectRoute: {
				type:   #RoutingType & "Direct"
				target: string
			}
			#TreeRoute: {
				type: #RoutingType & "Tree"
				tree: string
			}
			#Routing: #DirectRoute | #TreeRoute
			spec: {
				title:    string
				routing?: #Routing
				nullable?: #DirectRoute | null // Union with null should collapse to just optional in go
			}
		}
		selectableFields: [
			".spec.title",
			".spec.routing.type",
			".spec.routing.target",
			".spec.routing.tree",
			".spec.nullable.target",
		]
	}]
	routes: {
		namespaced: {
			"/foo": {
				"GET": {
					name: "getFoo"
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
