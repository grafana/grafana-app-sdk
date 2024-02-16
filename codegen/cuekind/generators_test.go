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
	// 5 -> object, spec, metadata, status, schema, codec
	assert.Len(t, files, 6)
	// Check content against the golden files
	compareToGolden(t, files, "")
}

func TestModelsGenerator(t *testing.T) {
	// Ideally, we test only that this outputs the right jennies,
	// but right now we just test the whole pipeline from thema -> written files

	parser, err := NewParser()
	require.Nil(t, err)
	kinds, err := parser.Parse(os.DirFS(TestCUEDirectory), "customKind2")
	fmt.Println(err)
	require.Nil(t, err)

	files, err := ModelsGenerator(false).Generate(kinds...)
	require.Nil(t, err)
	// Check number of files generated
	// 1 -> just the go type
	assert.Len(t, files, 1)
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
