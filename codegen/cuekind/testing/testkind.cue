package testing

import "time"

testManifest: {
	appName: "test-app"
	kinds: [testKind, testKind2]
	versions: {
		"v1": testManifestV1
		"v2": testManifestV2
		"v3": testManifestV3
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
					stringField: string
				}
			}
		}
		"v2": {
			codegen: ts: enabled: true
			schema: {
				spec: {
					stringField: string
					intField: int64
					timeField: string & time.Time
				}
			}
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
			mutation: operations: ["create","update"]
			validation: operations: ["create","update"]
			routes: {
				"/reconcile": {
					POST: {
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
							}]
							total: int
						}
					}
				}
			}
		}
	}
}
