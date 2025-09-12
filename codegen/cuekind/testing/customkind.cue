package testing

import "time"

customManifest: {
	appName: "custom-app"
	preferredVersion: "v1-0"
	versions: {
		"v0-0": custom_v0_0
		"v1-0": custom_v1_0
	}
}

custom_v0_0: {
	kinds: [{
			schema: customKind.versions["v0-0"].schema
		} & customKind]
}

custom_v1_0: {
	kinds: [{
			schema: customKind.versions["v1-0"].schema
		} & customKind]
}

customKind: {
	kind:    "CustomKind"
	versions: {
		"v0-0": {
			schema: {
				spec: {
					field1:          string
					deprecatedField: string
				}
			}
		}
		"v1-0": {
			schema: {
				#InnerObject1: {
					innerField1: string
					innerField2: [...string]
					innerField3: [...#InnerObject2]
					innerField4: [...{
						[string]: _
					}]
				}
				#InnerObject2: {
					name: string
					details: {
						[string]: _
					}
				}
				#Type1: {
					group: string
					options?: [...string]
				}
				#Type2: {
					group: string
					details: {
						[string]: _
					}
				}
				#RecursiveList: {
					val: string
					next?: #RecursiveList
				}
				#UnionType: #Type1 | #Type2
				spec: {
					field1: string
					inner:  #InnerObject1
					union:  #UnionType
					map: {
						[string]: #Type2
					}
					timestamp:  string & time.Time
					enum:       "val1" | "val2" | "val3" | "val4" | *"default"
					i32:        int32 & <=123456
					i64:        int64 & >=123456
					boolField:  bool | *false
					floatField: float64
					linkedList: #RecursiveList
					exclusiveInt: int & < 100 & > 10
				}
				status: {
					statusField1: string
					[string]:     _
				} @cog(open=true)
				metadata: {
					customMetadataField: string
					otherMetadataField:  string
				}
			}
		}
	}
}
