package cuekind

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/grafana/codejen"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"sigs.k8s.io/yaml"
)

const (
	TestCUEDirectory         = "./testing"
	ReferenceOutputDirectory = "../testing/golden_generated"
)

var jsonEncoder = func(v any) ([]byte, error) {
	return json.MarshalIndent(v, "", "    ")
}

func TestCRDGenerator(t *testing.T) {
	// Ideally, we test only that this outputs the right jennies,
	// but right now we just test the whole pipeline from thema -> written files

	parser, err := NewParser()
	require.Nil(t, err)
	kinds, err := parser.KindParser(ParseConfig{
		GenOperatorState: true,
	}).Parse(os.DirFS(TestCUEDirectory), "customManifest", "testManifest")
	require.Nil(t, err)

	t.Run("JSON", func(t *testing.T) {
		files, err := CRDGenerator(jsonEncoder, "json").Generate(kinds...)
		require.Nil(t, err)
		// Check number of files generated
		assert.Len(t, files, 3)
		// Check content against the golden files
		compareToGolden(t, files, "crd")
	})

	t.Run("YAML", func(t *testing.T) {
		files, err := CRDGenerator(yaml.Marshal, "yaml").Generate(kinds...)
		require.Nil(t, err)
		// Check number of files generated
		assert.Len(t, files, 3)
		// Check content against the golden files
		compareToGolden(t, files, "crd")
	})
}

func TestResourceGenerator(t *testing.T) {
	// Ideally, we test only that this outputs the right jennies,
	// but right now we just test the whole pipeline from thema -> written files

	parser, err := NewParser()
	require.Nil(t, err)
	kinds, err := parser.KindParser(ParseConfig{
		GenOperatorState: true,
	}).Parse(os.DirFS(TestCUEDirectory), "customManifest")
	require.Nil(t, err)
	sameGroupKinds, err := parser.KindParser(ParseConfig{
		GenOperatorState: true,
	}).Parse(os.DirFS(TestCUEDirectory), "testManifest")
	require.Nil(t, err)

	t.Run("group by kind", func(t *testing.T) {
		files, err := ResourceGenerator(false).Generate(kinds...)
		require.Nil(t, err)
		// Check number of files generated
		// 14 (7 -> object, spec, metadata, status, schema, codec, constants) * 2 versions
		assert.Len(t, files, 14, "should be 14 files generated, got %d", len(files))
		// Check content against the golden files
		compareToGolden(t, files, "go/groupbykind")
	})

	t.Run("group by group", func(t *testing.T) {
		files, err := ResourceGenerator(true).Generate(kinds...)
		require.Nil(t, err)
		// Check number of files generated
		// 14 (7 -> object, spec, metadata, status, schema, codec, constants) * 2 versions
		assert.Len(t, files, 14, "should be 14 files generated, got %d", len(files))
		// Check content against the golden files
		compareToGolden(t, files, "go/groupbygroup")
	})

	t.Run("group by group, multiple kinds", func(t *testing.T) {
		files, err := ResourceGenerator(true).Generate(sameGroupKinds...)
		require.Nil(t, err)
		// Check number of files generated
		assert.Len(t, files, 27, "should be 27 files generated, got %d", len(files))
		// Check content against the golden files
		compareToGolden(t, files, "go/groupbygroup")
	})
}

func TestTypeScriptResourceGenerator(t *testing.T) {
	// Ideally, we test only that this outputs the right jennies,
	// but right now we just test the whole pipeline from thema -> written files

	parser, err := NewParser()
	require.Nil(t, err)

	t.Run("versioned", func(t *testing.T) {
		kinds, err := parser.KindParser(ParseConfig{
			GenOperatorState: true,
		}).Parse(os.DirFS(TestCUEDirectory), "customManifest")
		require.Nil(t, err)
		files, err := TypeScriptResourceGenerator().Generate(kinds...)
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
		kinds, err := parser.ManifestParser(ParseConfig{
			GenOperatorState: true,
		}).Parse(os.DirFS(TestCUEDirectory), "testManifest")
		require.Nil(t, err)
		files, err := ManifestGenerator(yaml.Marshal, "yaml", true, true).Generate(kinds...)
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

	t.Run("group by group", func(t *testing.T) {
		kinds, err := parser.ManifestParser(ParseConfig{
			GenOperatorState: true,
		}).Parse(os.DirFS(TestCUEDirectory), "testManifest")
		require.Nil(t, err)
		files, err := ManifestGoGenerator("manifestdata", true, "codegen-tests", "pkg/generated", "manifestdata", true).Generate(kinds...)
		require.Nil(t, err)
		// Check number of files generated
		// 10 -> manifest file, then the custom route response+query+body for reconcile, response body and wrapper+query+body for search in v3, +1 client per version (3)
		require.Len(t, files, 11, "should be 11 files generated, got %d", len(files))
		// Check content against the golden files
		for _, file := range files {
			compareToGolden(t, codejen.Files{file}, "go/groupbygroup")
		}
	})

	t.Run("group by kind", func(t *testing.T) {
		kinds, err := parser.ManifestParser(ParseConfig{
			GenOperatorState: true,
		}).Parse(os.DirFS(TestCUEDirectory), "customManifest")
		require.Nil(t, err)
		files, err := ManifestGoGenerator("manifestdata", true, "codegen-tests", "pkg/generated", "manifestdata", false).Generate(kinds...)
		require.Nil(t, err)
		// Check number of files generated
		// 3 -> manifest, client v0_0, client v1_0
		assert.Len(t, files, 3)
		// Check content against the golden files
		for _, file := range files {
			compareToGolden(t, codejen.Files{file}, "go/groupbykind")
		}
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
