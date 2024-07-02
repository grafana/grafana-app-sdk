package testing

import (
    "time"
)

customKind: {
    name: "CustomKind"
    crd: {}
    group: "custom"
    latest: [1,0]
    lineage: {
        name: "customkind",
        schemas: [{
            version: [0,0]
            schema: {
                spec: {
                    field1: string
                    deprecatedField: string
                }
            }
        },{
            version: [1,0]
            schema: {
                spec: {
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

                    field1: string
                    inner: #InnerObject1
                    union: #UnionType
                    map: {
                        [string]: #Type2
                    }
                    timestamp: string & time.Time
                    enum: "val1" | "val2" | "val3" | "val4" | *"default" @cuetsy(kind="enum",memberNames="val1|val2|val3|val4|default")
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
        }]
    }
}
