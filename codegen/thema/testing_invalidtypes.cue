package thema

import (
	"github.com/grafana/thema"
)

// Raw lineage
testLin: thema.#Lineage
testLin: name: "foo"
testLin: schemas: [{
    version: [0,0]
    schema: {
        firstfield: string
    }
},{
    version: [1,0]
    schema: {
        firstfield: string
        secondfield: string
    }
}]

// Something else
#Foo: {
	bar: string
}
testFoo: #Foo
testFoo: bar: "test"