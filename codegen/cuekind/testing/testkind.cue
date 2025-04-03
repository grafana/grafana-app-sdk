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
			customRoutes: {
				"/reconcile": {
					summary: "Resource reconciliation"
					description: "Manually trigger resource reconciliation"
					parameters: [{
						name: "dryRun"
						in: "query"
						description: "When true, only validate the request without performing reconciliation"
						required: false
						schema: {
							type: "boolean"
						}
					}, {
						name: "timeout"
						in: "query"
						description: "Timeout in seconds for the reconciliation operation"
						required: false
						schema: {
							type: "integer"
							format: "int32"
							minimum: 1
							maximum: 300
							default: 60
						}
					}]
					post: {
						summary: "Reconcile resource"
						description: "Trigger a manual reconciliation of the resource"
						operationId: "reconcileResource"
						tags: ["operations", "management"]
						externalDocs: {
							description: "Learn more about reconciliation"
							url: "https://example.com/docs/reconciliation"
						}
						parameters: [{
							name: "X-Tenant-ID"
							in: "header"
							description: "Tenant identifier for multi-tenant operations"
							required: false
							schema: {
								type: "string"
							}
						}]
						deprecated: false
						servers: [{
							url: "https://api-test.example.com/v3"
							description: "Test API server"
						}, {
							url: "https://api.example.com/v3"
							description: "Production API server"
						}]
						requestBody: {
							description: "Reconciliation options"
							required: true
							content: {
								"application/json": {
									schema: {
										type: "object"
										properties: {
											force: {
												type: "boolean"
												description: "Force reconciliation even if already in progress"
											}
											timeout: {
												type: "integer"
												format: "int32"
												description: "Maximum time to wait for reconciliation (in seconds)"
											}
											skipValidation: {
												type: "boolean"
												description: "Skip pre-reconciliation validation checks"
											}
										}
										required: ["force"]
									}
								}
							}
						}
						responses: {
							"200": {
								description: "Successful reconciliation"
								content: {
									"application/json": {
										schema: {
											type: "object"
											properties: {
												id: {
													type: "string"
													description: "Reconciliation operation ID"
												}
												status: {
													type: "string"
													description: "Status of the reconciliation operation"
													enum: ["success", "in-progress", "failed"]
												}
												message: {
													type: "string"
													description: "Additional information about the reconciliation"
												}
												startTime: {
													type: "string"
													format: "date-time"
													description: "When the reconciliation was started"
												}
												completionTime: {
													type: "string"
													format: "date-time"
													description: "When the reconciliation was completed (if finished)"
												}
											}
											required: ["id", "status"]
										}
										examples: {
											default: {
												value: {
													id: "rec-1234-abcd"
													status: "success"
													message: "Resource successfully reconciled"
													startTime: "2023-10-24T14:15:22Z"
													completionTime: "2023-10-24T14:15:42Z"
												}
											}
										}
									}
								}
							}
							"400": {
								description: "Invalid request"
								content: {
									"application/json": {
										schema: {
											type: "object"
											properties: {
												error: {
													type: "string"
													description: "Error message"
												}
												details: {
													type: "string"
													description: "Detailed error information"
												}
											}
											required: ["error"]
										}
										examples: {
											"application/json": {
												value: {
													error: "Invalid reconciliation request"
													details: "Missing required field 'force'"
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
		}
	}
}