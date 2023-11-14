package thema

// CustomKind
testCustom: {
	name: "Foo"
    group: "test"
	machineName: "foo"
	pluralName: "Foos"
	pluralMachineName: "foos"
	currentVersion: [1,0]
	crd: {}
    codegen: {
        frontend: false
    }
}

testCustom: lineage: name: "foo"
testCustom: lineage: schemas: [{
    version: [0,0]
    schema: {
        spec: {
            firstfield: string
        }
    }
},{
    version: [1,0]
    schema: {
        spec: {
            firstfield: string
            secondfield: int
        }
    }
}]

testCustom2: {
	name: "Foo2"
    group: "test"
	machineName: "foo2"
	pluralName: "Foo2s"
	pluralMachineName: "foo2s"
	currentVersion: [1,0]
    crd: {
        scope: "Cluster"
    }
    codegen: frontend: false
}

testCustom2: lineage: name: "foo2"
testCustom2: lineage: schemas: [{
    version: [0,0]
    schema: {
        spec: {
            firstfield: string
        }
        metadata: {
            mdField: string
        }
    }
},{
    version: [1,0]
    schema: {
        spec: {
            firstfield: string
            secondfield: int
        }
        metadata: {
            mdField: string
            extraMeta: string
        }
    }
}]