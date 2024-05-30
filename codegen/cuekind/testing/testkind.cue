package testing

import "time"

testKind: {
	kind: "TestKind"
	plural: "testkinds"
	group: "test"
	apiResource: {}
	current: "v1"
	codegen: frontend: false
	versions: {
		"v1": {
			schema: {
				spec: {
					stringField: string
				}
			}
		}
		"v2": {
			codegen: frontend: true
			schema: {
				spec: {
					stringField: string
					intField: int64
					timeField: string & time.Time
				}
			}
		}
	}
}