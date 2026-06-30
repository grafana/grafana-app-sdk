package testing

import "time"

testManifest: {
	appName: "test-app"
	kinds: [testKind, testKind2]
	versions: {
		"v1": testManifestV1
		"v2": testManifestV2
		"v3": testManifestV3
		"v4": testManifestV4
	}
	preferredVersion: "v1"
	extraPermissions: {
		accessKinds: [{
			group: "foo.bar"
			resource: "foos"
			actions: ["get","list","watch"]
		}]
	}
	operatorURL: "https://foo.bar:8443"
	roles: {
		"test-app:reader": {
			title: "Test App Viewer"
			description: "View Test App Resources"
			kinds: [{
				kind: "TestKind"
				permissionSet: "viewer"
			}]
			routes: ["createFoobar"]
		}
	}
	roleBindings: {
		viewer: ["test-app:reader"]
	}
}

testManifestV1: {
	codegen: ts: enabled: false
	kinds: [
		testKind.versions["v1"] & testKind,
		testKind2.versions["v1"] & testKind2
	]
}

testManifestV2: {
	codegen: ts: enabled: false
	kinds: [testKind & testKind.versions["v2"]]
}

testManifestV3: {
	codegen: ts: enabled: false
	kinds: [testKind & testKind.versions["v3"]]
	routes: namespaced: {
		"/foobar": {
			"POST": {
				name: "createFoobar"
				// Key identifies a foobar entry.
				// It is referenced by a custom route, so its multi-line
				// description exercises the route schema codegen path.
				#Key: {
					name: string
					match?: string
				}
				request: body: {
					keys: [...#Key]
				}
				response: {
					altered: [...#Key]
				}
			}
		}
	}
}

testManifestV4: {
	codegen: ts: enabled: false
	kinds: [testKind & testKind.versions["v4"]]
}

testKind: {
	kind: "TestKind"
	plural: "testkinds"
	validation: operations: ["create","update"]
	conversion: true
	conversionWebhookProps: url: "http://foo.bar/convert"
	current: "v1"
	versions: {
		"v1": {
			schema: {
				spec: {
					stringField: string & =~"^[a-zA-Z_][a-zA-Z0-9_-]*$"
				}
			}
			selectableFields: [".spec.stringField"]
			searchFields: [
				{
					name: "stringField"
					path: "spec.stringField"
					type: "string"
					capabilities: ["filter", "text", "sort", "retrieve"]
					description: "The string field"
				},
			]
		}
		"v2": {
			codegen: ts: enabled: true
			schema: {
				#Def: {
					str: string
					i: int
				}
				#Def2: {
					str: string
					b: bool
				}
				spec: {
					stringField: string
					intField: int64
					timeField: string & time.Time
					unionNull?: #Def | null // This generates a normal go pointer field, but needs to be handled correctly as a selectable field, as the schema logic registers it as a disjunction
					unionNull2?: #Def | #Def2 | null // null variant should be ignore in disjunction when generating the selectable field
				}
			}
			selectableFields: [".spec.stringField", ".spec.intField", ".spec.unionNull.str", ".spec.unionNull2.str"]
			searchFields: [
				{
					name: "stringField"
					path: "spec.stringField"
					type: "string"
					capabilities: ["filter", "text", "sort", "retrieve"]
					description: "The string field"
				},
				{
					name: "intField"
					path: "spec.intField"
					type: "int64"
					capabilities: ["filter", "retrieve"]
					emitZeroIfAbsent: true
				},
			]
			mutation: operations: ["create","update"]
			additionalPrinterColumns: [
                {
                    jsonPath: ".spec.stringField"
                    name: "STRING FIELD"
                    type: "string"
                }
            ]
		}
		"v3": {
			schema: {
				spec: {
					stringField: string
					intField: int64
					timeField: string & time.Time
					boolField: bool
				}
			}
			selectableFields: [".spec.stringField", ".spec.intField", ".spec.boolField"]
			searchFields: [
				{
					name: "stringField"
					path: "spec.stringField"
					type: "string"
					capabilities: ["filter", "text", "sort", "retrieve"]
					description: "The string field"
				},
				{
					name: "intField"
					path: "spec.intField"
					type: "int64"
					capabilities: ["filter", "retrieve"]
					emitZeroIfAbsent: true
				},
				{
					name: "boolField"
					path: "spec.boolField"
					type: "boolean"
					capabilities: ["filter", "retrieve"]
					emitZeroIfAbsent: true
				},
			]
			mutation: operations: ["create","update"]
			validation: operations: ["create","update"]
			routes: {
				"/reconcile": {
					POST: {
						name: "createReconcileRequest"
						request: {
							body: {
								force: bool | *false 
								reason?: string
							}
						}
						response: {
							status: "success" | "failure"
                            // A comment containing "quotes" should not break anything
							message: string
						}
						responseMetadata: typeMeta: false
					}
				}
				"/search": {
					GET: {
						name: "getTestKindSearchResult"
						extensions: {
							"x-grafana-test": true
							"x-grafana-test-value": {
								val: "1"
							}
						}
						request: {
							query: {
								q: string
								limit?: int | *10
								offset?: int | *0
							}
						}
						response: {
							items: [...{
								name: string
								score: float
								list: [...{
									foo: string
								}]
							}]
							total: int
						}
					}
				}
			}
		}
		// v4: selectable field crosses a union nested under spec (spec.union is the disjunction; path .spec.union.spec.name).
		"v4": {
			schema: {
				#UnionVariantA: {
					kind: "VariantA"
					spec: {name: string}
				}
				#UnionVariantB: {
					kind: "VariantB"
					spec: {name: string}
				}
				spec: {
					union: #UnionVariantA | #UnionVariantB
				}
			}
			selectableFields: [".spec.union.spec.name"]
			validation: operations: ["create", "update"]
		}
	}
}
