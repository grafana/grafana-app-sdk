package core

testKind: {
	kind: "TestKind"
	group: "core"
	apiResource: {
		groupOverride: "core.grafana.internal"
	}
	codegen: {
		frontend: false
	}
	current: "v1"
	versions: {
		"v1": {
			schema: {
				#SubType: {
					subField1: int64
					subField2: bool
				}
				spec: {
					stringField: string
					subtypeField: #SubType
				}
			}
		}
	}
}