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
	kinds, err := parser.Parse(os.DirFS(TestCUEDirectory), "testKind", "customKind")
	require.Nil(t, err)

	t.Run("JSON", func(t *testing.T) {
		files, err := CRDGenerator(json.Marshal, "json").Generate(kinds...)
		require.Nil(t, err)
		// Check number of files generated
		assert.Len(t, files, 2)
		// Check content against the golden files
		compareToGolden(t, files, "crd")
	})

	t.Run("YAML", func(t *testing.T) {
		files, err := CRDGenerator(yaml.Marshal, "yaml").Generate(kinds...)
		require.Nil(t, err)
		// Check number of files generated
		assert.Len(t, files, 2)
		// Check content against the golden files
		compareToGolden(t, files, "crd")
	})
}

func TestResourceGenerator(t *testing.T) {
	// Ideally, we test only that this outputs the right jennies,
	// but right now we just test the whole pipeline from thema -> written files

	parser, err := NewParser()
	require.Nil(t, err)
	kinds, err := parser.Parse(os.DirFS(TestCUEDirectory), "customKind")
	require.Nil(t, err)
	sameGroupKinds, err := parser.Parse(os.DirFS(TestCUEDirectory), "testKind", "testKind2")
	require.Nil(t, err)

	t.Run("unversioned", func(t *testing.T) {
		files, err := ResourceGenerator(false, false).Generate(kinds...)
		require.Nil(t, err)
		// Check number of files generated
		// 6 -> object, spec, metadata, status, schema, codec
		assert.Len(t, files, 6)
		// Check content against the golden files
		compareToGolden(t, files, "go/unversioned")
	})

	t.Run("group by kind", func(t *testing.T) {
		files, err := ResourceGenerator(true, false).Generate(kinds...)
		require.Nil(t, err)
		// Check number of files generated
		// 12 (6 -> object, spec, metadata, status, schema, codec) * 2 versions
		assert.Len(t, files, 12)
		// Check content against the golden files
		compareToGolden(t, files, "go/groupbykind")
	})

	t.Run("group by group", func(t *testing.T) {
		files, err := ResourceGenerator(true, true).Generate(kinds...)
		require.Nil(t, err)
		// Check number of files generated
		// 12 (6 -> object, spec, metadata, status, schema, codec) * 2 versions
		assert.Len(t, files, 12)
		// Check content against the golden files
		compareToGolden(t, files, "go/groupbygroup")
	})

	t.Run("group by group, multiple kinds", func(t *testing.T) {
		files, err := ResourceGenerator(true, true).Generate(sameGroupKinds...)
		require.Nil(t, err)
		// Check number of files generated
		assert.Len(t, files, 18)
		// Check content against the golden files
		compareToGolden(t, files, "go/groupbygroup")
	})
}

func TestModelsGenerator(t *testing.T) {
	// Ideally, we test only that this outputs the right jennies,
	// but right now we just test the whole pipeline from thema -> written files

	parser, err := NewParser()
	require.Nil(t, err)
	kinds, err := parser.Parse(os.DirFS(TestCUEDirectory), "customKind2")
	fmt.Println(err)
	require.Nil(t, err)

	t.Run("unversioned", func(t *testing.T) {
		files, err := ModelsGenerator(false, true).Generate(kinds...)
		require.Nil(t, err)
		// Check number of files generated
		// 1 -> just the go type
		assert.Len(t, files, 1)
		// Check content against the golden files
		compareToGolden(t, files, "go/unversioned")
	})
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
		compareToGolden(t, files, "typescript/unversioned")
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
		compareToGolden(t, files, "typescript/unversioned")
	})
}

func TestTypeScriptResourceGenerator(t *testing.T) {
	// Ideally, we test only that this outputs the right jennies,
	// but right now we just test the whole pipeline from thema -> written files

	parser, err := NewParser()
	require.Nil(t, err)

	t.Run("versioned", func(t *testing.T) {
		kinds, err := parser.Parse(os.DirFS(TestCUEDirectory), "customKind")
		require.Nil(t, err)
		files, err := TypeScriptResourceGenerator(true).Generate(kinds...)
		require.Nil(t, err)
		// Check number of files generated
		assert.Len(t, files, 8)
		// Check content against the golden files
		compareToGolden(t, files, "typescript/versioned")
	})
}

func TestManifestGenerator(t *testing.T) {
	parser, err := NewParser()
	require.Nil(t, err)

	t.Run("resource", func(t *testing.T) {
		kinds, err := parser.Parse(os.DirFS(TestCUEDirectory), "testKind", "testKind2")
		require.Nil(t, err)
		files, err := ManifestGenerator(yaml.Marshal, "yaml", "test-app-test-kind").Generate(kinds...)
		require.Nil(t, err)
		// Check number of files generated
		// 5 -> object, spec, metadata, status, schema
		assert.Len(t, files, 1)
		// Check content against the golden files
		compareToGolden(t, files, "manifest")
	})

	t.Run("model", func(t *testing.T) {
		kinds, err := parser.Parse(os.DirFS(TestCUEDirectory), "customKind2")
		fmt.Println(err)
		require.Nil(t, err)
		files, err := ManifestGenerator(yaml.Marshal, "yaml", "test-app-custom-kind-2").Generate(kinds...)
		require.Nil(t, err)
		// Check number of files generated
		// 5 -> object, spec, metadata, status, schema
		assert.Len(t, files, 1)
		// Check content against the golden files
		compareToGolden(t, files, "manifest")
	})
}

func TestManifestGoGenerator(t *testing.T) {
	parser, err := NewParser()
	require.Nil(t, err)

	t.Run("resource", func(t *testing.T) {
		kinds, err := parser.Parse(os.DirFS(TestCUEDirectory), "testKind", "testKind2")
		require.Nil(t, err)
		files, err := ManifestGoGenerator("generated", "test-app").Generate(kinds...)
		require.Nil(t, err)
		// Check number of files generated
		// 5 -> object, spec, metadata, status, schema
		assert.Len(t, files, 1)
		// Check content against the golden files
		compareToGolden(t, files, "manifest/go/testkinds")
	})
}

func compareToGolden(t *testing.T, files codejen.Files, pathPrefix string) {
	for _, f := range files {
		// Check if there's a golden generated file to compare against
		file, err := os.ReadFile(filepath.Join(ReferenceOutputDirectory, pathPrefix, f.RelativePath+".txt"))
		require.Nil(t, err)
		// Compare the contents of the file to the generated contents
		// Use strings for easier-to-read diff in the event of a mismatch
		assert.Equal(t, string(file), string(f.Data))
	}
}
