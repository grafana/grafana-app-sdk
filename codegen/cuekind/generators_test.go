package cuekind

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"testing"

	"github.com/grafana/codejen"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gopkg.in/yaml.v3"
)

const (
	TestCUEDirectory         = "./testing"
	ReferenceOutputDirectory = "../testing/golden_generated"
)

func TestCRDGenerator(t *testing.T) {
	// Ideally, we test only that this outputs the right jennies,
	// but right now we just test the whole pipeline from thema -> written files

	parser, err := NewParser()
	require.Nil(t, err)
	kinds, err := parser.Parse(os.DirFS(TestCUEDirectory), "customKind")
	require.Nil(t, err)

	t.Run("JSON", func(t *testing.T) {
		files, err := CRDGenerator(json.Marshal, "json").Generate(kinds...)
		require.Nil(t, err)
		// Check number of files generated
		assert.Len(t, files, 1)
		// Check content against the golden files
		compareToGolden(t, files, "")
	})

	t.Run("YAML", func(t *testing.T) {
		files, err := CRDGenerator(yaml.Marshal, "yaml").Generate(kinds...)
		require.Nil(t, err)
		// Check number of files generated
		assert.Len(t, files, 1)
		// Check content against the golden files
		compareToGolden(t, files, "")
	})
}

func TestResourceGenerator(t *testing.T) {
	// Ideally, we test only that this outputs the right jennies,
	// but right now we just test the whole pipeline from thema -> written files

	parser, err := NewParser()
	require.Nil(t, err)
	kinds, err := parser.Parse(os.DirFS(TestCUEDirectory), "customKind")
	fmt.Println(err)
	require.Nil(t, err)

	files, err := ResourceGenerator(false).Generate(kinds...)
	require.Nil(t, err)
	// Check number of files generated
	// 5 -> object, spec, metadata, status, schema
	assert.Len(t, files, 5)
	// Check content against the golden files
	compareToGolden(t, files, "")
}

func TestTypeScriptModelsGenerator(t *testing.T) {
	// Ideally, we test only that this outputs the right jennies,
	// but right now we just test the whole pipeline from thema -> written files

	parser, err := NewParser()
	require.Nil(t, err)

	t.Run("resource", func(t *testing.T) {
		kinds, err := parser.Parse(os.DirFS(TestCUEDirectory), "customKind")
		require.Nil(t, err)
		files, err := TypeScriptModelsGenerator(false).Generate(kinds...)
		require.Nil(t, err)
		// Check number of files generated
		// 5 -> object, spec, metadata, status, schema
		assert.Len(t, files, 1)
		// Check content against the golden files
		compareToGolden(t, files, "")
	})

	t.Run("model", func(t *testing.T) {
		kinds, err := parser.Parse(os.DirFS(TestCUEDirectory), "customKind2")
		fmt.Println(err)
		require.Nil(t, err)
		files, err := TypeScriptModelsGenerator(false).Generate(kinds...)
		require.Nil(t, err)
		// Check number of files generated
		// 5 -> object, spec, metadata, status, schema
		assert.Len(t, files, 1)
		// Check content against the golden files
		compareToGolden(t, files, "")
	})
	/*
			kinds, err := parser.Parse(os.DirFS(TestCUEDirectory), "customKind2")
			fmt.Println(err)
			require.Nil(t, err)

			_ = kinds[0].Versions()[0].Schema.Context().CompileString(`
		import "time"

		customKind2: {
			group: "custom"
			kind: "CustomKind2"
			current: "v0-0"
			codegen: {
		        frontend: true
		        backend: true
		    }
			versions: {
				"v0-0": {
					version: "v0-0"
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
				}
			}
		}`)

			files, err := TypeScriptModelsGenerator(false).Generate(kinds..., /*&codegen.AnyKind{
				Props: codegen.KindProperties{
					Current:     "v0-0",
					Kind:        "CustomKind2",
					MachineName: "customkind2",
					Codegen: codegen.CodegenProperties{
						Frontend: true,
					},
				},
				AllVersions: []codegen.KindVersion{{
					Schema: val.LookupPath(cue.MakePath(cue.Str("customKind2"), cue.Str("versions"), cue.Str("v0-0"), cue.Str("schema"))),
					Codegen: codegen.CodegenProperties{
						Frontend: true,
					},
					Version: "v0-0",
				}},
			}*/ /*)
	require.Nil(t, err)
	// Check number of files generated
	// 5 -> object, spec, metadata, status, schema
	assert.Len(t, files, 1)
	// Check content against the golden files
	compareToGolden(t, files, "")*/
}

func compareToGolden(t *testing.T, files codejen.Files, pathPrefix string) {
	for _, f := range files {
		// Check if there's a golden generated file to compare against
		file, err := os.ReadFile(filepath.Join(ReferenceOutputDirectory, f.RelativePath+".txt"))
		require.Nil(t, err)
		// Compare the contents of the file to the generated contents
		// Use strings for easier-to-read diff in the event of a mismatch
		assert.Equal(t, string(file), string(f.Data))
	}
}
