package testing

integrationManifest: {
	appName: "integration"
	versions: {
		"v1": integrationV1
	}
}

// The generated manifest's openapi section combines custom routes from four
// sources: kind routes, version-level routes, inline OpenAPI, and an OpenAPI file.
// Route names identify their source so the integration snapshot shows the merge.
integrationV1: {
	// File-based OpenAPI contributes cluster and namespaced paths and the Report schema.
	importOpenAPIFile: "integration.openapi.json"

	// CRUD++ for managed kinds
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
		// Kind routes become /namespaces/{namespace}/foos/{name}/... in openapi.paths.
		routes: {
			"/sub-kind-from-cue": {
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
	// Legacy version-level routes also populate openapi.paths. Keep these here
	// to verify that they are combined with the inline and file-based definitions.
	routes: {
		namespaced: {
			"/foo-from-routes-namespaced-cue": {
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
			"/bar-from-routes-cluster-cue": {
				"POST": {
					name: "createBar"
					response: {
						bar: int
					}
				}
			}
		}
	}

	// Inline OpenAPI contributes another path to the same generated openapi section.
	openapi: {
		paths: {
			"/foo-from-inline-openapi-cue": {
				get: {
					operationId: "getFooFromCue"
					responses: {
						"200": {
							description: "OK"
						}
					}
				}
			}
		}
	}
}
