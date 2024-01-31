package themagenerator

import (
	"github.com/grafana/thema"
)

myObject: {
	name: "MyObject"
	lineage: thema.#Lineage
	scope: "Namespaced"
	layer: "storage"
	pluginID: "my_example_plugin"
}

// Fill out the lineage
myObject: lineage: name: myObject.name
myObject: lineage: seqs: [
    {
        schemas: [
            { // 0.0
                spec: {
                	firstfield: string
                }
            }
        ]
    },
    {
        schemas: [
            { // 1.0
            		spec: {
            			  firstfield: string
                		secondfield: >=0
                		thirdField: {
                				innerField: [...string]
                		}
            		}
            }
        ]

        lens: forward: {
            from: seqs[0].schemas[0]
            to: seqs[1].schemas[0]
            rel: {
                firstfield: from.spec.firstfield
                secondfield: 1
            }
            lacunas: [
                thema.#Lacuna & {
                    targetFields: [{
                        path: "secondfield"
                        value: to.spec.secondfield
                    }]
                    message: "-1 used as a placeholder value - replace with a real value before persisting!"
                    type: thema.#LacunaTypes.Placeholder
                }
            ]
            translated: to & rel
        }
        lens: reverse: {
            from: seqs[1].schemas[0]
            to: seqs[0].schemas[0]
            rel: {
                // Map the first field back
                firstfield: from.spec.firstfield
            }
            translated: to & rel
        }
    }
]

// myOtherObject uses the same lineage for the sake of this example
myOtherObject: {
	name: "MyOtherObject"
	plural: "MyOtherObjectPlurals"
	lineage: myObject.lineage
	scope: "Namespaced"
	layer: "storage"
	pluginID: myObject.pluginID
}