package testing

import "time"

testManifest: {
	appName: "test-app"
	kinds: [testKind, testKind2]
	extraPermissions: {
		accessKinds: [{
			group: "foo.bar"
			resource: "foos"
			actions: ["get","list","watch"]
		}]
	}
}

testKind: {
	kind: "TestKind"
	plural: "testkinds"
	validation: operations: ["create","update"]
	conversion: true
	conversionWebhookProps: url: "http://foo.bar/convert"
	current: "v1"
	codegen: ts: enabled: false
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
			codegen: ts: enabled: true
			schema: {
				spec: {
					stringField: string
				}
			}
			customRoutes: {
				"/test": {
					summary: "Test custom route"
					description: "Test custom route description"
					operations: {
						get: {
							tags: ["test", "example"]
							summary: "Test GET operation"
							description: "Test GET operation description"
							operationId: "testGet"
							deprecated: false
							consumes: ["application/json"]
							produces: ["application/json"]
							parameters: [
								{
									name: "param1"
									"in": "query"
									description: "Example query parameter"
									required: false
								}
							]
							responses: {
								default: {
									description: "Success response"
									schema: {
										type: "object"
										properties: {
											message: {
												type: "string"
											}
										}
									}
								}
								statusCodeResponses: {
									400: {
										description: "Bad request"
										schema: {
											type: "object"
											properties: {
												error: {
													type: "string"
												}
											}
										}
									}
									500: {
										description: "Internal server error"
									}
								}
							}
						}
						post: {
							summary: "Test POST operation"
							description: "Test POST operation description"
							operationId: "testPost"
							consumes: ["application/json"]
							produces: ["application/json"]
							parameters: [
								{
									name: "param1"
									"in": "body"
									description: "Example body parameter"
									required: true
									schema: {
										type: "object"
										properties: {
											foo: {
												type: "string"
											}
											bar: {
												type: "integer"
												format: "int64"
											}
										}
									}
								}
							]
							responses: {
								default: {
									description: "Success response"
									schema: {
										type: "object"
										properties: {
											id: {
												type: "string"
											}
										}
									}
								}
								statusCodeResponses: {
									400: {
										description: "Bad request"
									}
									409: {
										description: "Resource conflict"
									}
								}
							}
						}
					}
				}
			}
		}
	}
}