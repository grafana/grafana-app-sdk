package testing

import (
    "time"
)

customKind2: {
    name: "CustomKind2"
    group: "custom"
    codegen: {
        frontend: true
        backend: true
    }
    lineage: {
        name: "customkind2",
        schemas: [{
            version: [0,0]
            schema: {
                #InnerObject1: {
                    innerField1: string
                    innerField2: [...string]
                    innerField3: [...#InnerObject2]
                } @cuetsy(kind="interface")
                #InnerObject2: {
                    name: string
                    details: {
                        [string]: _
                    }
                } @cuetsy(kind="interface")
                #Type1: {
                    group: string
                    options?: [...string]
                } @cuetsy(kind="interface")
                #Type2: {
                    group: string
                    details: {
                        [string]: _
                    }
                } @cuetsy(kind="interface")
                #UnionType: #Type1 | #Type2 @cuetsy(kind="type")
                field1: string
                inner: #InnerObject1
                union: #UnionType
                map: {
                    [string]: #Type2
                }
                timestamp: string & time.Time @cuetsy(kind="string")
                enum: "val1" | "val2" | "val3" | "val4" | *"default" @cuetsy(kind="enum")
                i32: int32 & <= 123456
                i64: int64 & >= 123456
                boolField: bool | *false
                floatField: float64
            }
        }]
    }
}