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
	operatorURL: "https://foo.bar:8443"
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
	}
}