package testing

import "time"

customManifest: {
	appName: "custom-app"
	kinds: [customKind]
}

customKind: {
	kind: "CustomKind"
	current: "v1-0"
	versions: {
		"v0-0": {
			schema: {
				spec: {
					field1: string
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
                #UnionType: #Type1 | #Type2
                spec: {
                    field1: string
                    inner: #InnerObject1
                    union: #UnionType
                    map: {
                        [string]: #Type2
                    }
                    timestamp: string & time.Time
                    enum: "val1" | "val2" | "val3" | "val4" | *"default"
                    i32: int32 & <= 123456
                    i64: int64 & >= 123456
                    boolField: bool | *false
                    floatField: float64
                }
                status: {
                    statusField1: string
                }
                metadata: {
                    customMetadataField: string
                    otherMetadataField: string
                }
			}
		}
	}
}
