package customkind

import (
	"github.com/grafana/thema"
	"time"
	"struct"
	"strings"
)

customkind: thema.#Lineage & {
	joinSchema: {
		metadata: {
			{
				uid:               string
				creationTimestamp: time.Time & {
					string
				}
				deletionTimestamp?: time.Time & {
					string
				}
				finalizers: [...string]
				resourceVersion: string
				labels: {
					[string]: string
				}
			}
			{
				[!~"^(uid|creationTimestamp|deletionTimestamp|finalizers|resourceVersion|labels|updateTimestamp|createdBy|updatedBy|extraFields)$"]: string
			}
			updateTimestamp: time.Time & {
				string
			}
			createdBy: string
			updatedBy: string
			extraFields: {
				[string]: _
			}
		}
		spec:            _
		_specIsNonEmpty: spec & struct.MinFields(0)
		status: {
			{
				[string]: _
			}
			#OperatorState: {
				lastEvaluation:    string
				state:             "success" | "in_progress" | "failed"
				descriptiveState?: string
				details?: {
					[string]: _
				}
			}
			operatorStates?: {
				[string]: #OperatorState
			}
			additionalFields?: {
				[string]: _
			}
		}
	}
} & {
	name: strings.ToLower(strings.Replace("CustomKind", "-", "_", -1)) & {
		"customkind"
	}
	schemas: [{
		version: [0, 0]
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
				inner:  #InnerObject1
				union:  #UnionType
				map: {
					[string]: #Type2
				}
				timestamp: time.Time & {
					string
				}
				enum:       "val1" | "val2" | "val3" | "val4" | *"default"
				i32:        >=-2147483648 & <=123456 & int
				i64:        >=123456 & <=9223372036854775807 & int
				boolField:  bool | *false
				floatField: >=-1.797693134862315708145274237317043567981e+308 & <=1.797693134862315708145274237317043567981e+308
			}
			status: {
				statusField1: string
			}
			metadata: {
				customMetadataField: string
				otherMetadataField:  string
			}
		}
	}]
}