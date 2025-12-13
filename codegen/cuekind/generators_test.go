package cuekind

import (
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

func TestCRDGenerator(t *testing.T) {
	// Ideally, we test only that this outputs the right jennies,
	// but right now we just test the whole pipeline from thema -> written files

	parser, err := NewParser(os.DirFS(TestCUEDirectory), "config")
	require.NoError(t, err)
	kinds, err := parser.KindParser().Parse("customManifest", "testManifest")
	require.NoError(t, err)

	t.Run("JSON", func(t *testing.T) {
		files, err := CRDGenerator(jsonEncoder, "json").Generate(kinds...)
		require.NoError(t, err)
		// Check number of files generated
		assert.Len(t, files, 3)
		// Check content against the golden files
		compareToGolden(t, files, "crd")
	})

	t.Run("YAML", func(t *testing.T) {
		files, err := CRDGenerator(yaml.Marshal, "yaml").Generate(kinds...)
		require.NoError(t, err)
		// Check number of files generated
		assert.Len(t, files, 3)
		// Check content against the golden files
		compareToGolden(t, files, "crd")
	})
}

func TestResourceGenerator(t *testing.T) {
	// Ideally, we test only that this outputs the right jennies,
	// but right now we just test the whole pipeline from thema -> written files

	parser, err := NewParser(os.DirFS(TestCUEDirectory), "config")
	require.NoError(t, err)
	kinds, err := parser.KindParser().Parse("customManifest")
	require.NoError(t, err)
	sameGroupKinds, err := parser.KindParser().Parse("testManifest")
	require.NoError(t, err)

	t.Run("group by kind", func(t *testing.T) {
		files, err := ResourceGenerator(false).Generate(kinds...)
		require.NoError(t, err)
		// Check number of files generated
		// 14 (7 -> object, spec, metadata, status, schema, codec, constants) * 2 versions
		assert.Len(t, files, 14, "should be 14 files generated, got %d", len(files))
		// Check content against the golden files
		compareToGolden(t, files, "go/groupbykind")
	})

	t.Run("group by group", func(t *testing.T) {
		files, err := ResourceGenerator(true).Generate(kinds...)
		require.NoError(t, err)
		// Check number of files generated
		// 14 (7 -> object, spec, metadata, status, schema, codec, constants) * 2 versions
		assert.Len(t, files, 14, "should be 14 files generated, got %d", len(files))
		// Check content against the golden files
		compareToGolden(t, files, "go/groupbygroup")
	})

	t.Run("group by group, multiple kinds", func(t *testing.T) {
		files, err := ResourceGenerator(true).Generate(sameGroupKinds...)
		require.NoError(t, err)
		// Check number of files generated
		assert.Len(t, files, 27, "should be 27 files generated, got %d", len(files))
		// Check content against the golden files
		compareToGolden(t, files, "go/groupbygroup")
	})
}

func TestTypeScriptResourceGenerator(t *testing.T) {
	// Ideally, we test only that this outputs the right jennies,
	// but right now we just test the whole pipeline from thema -> written files

	parser, err := NewParser(os.DirFS(TestCUEDirectory), "config")
	require.NoError(t, err)

	t.Run("versioned", func(t *testing.T) {
		kinds, err := parser.KindParser().Parse("customManifest")
		require.NoError(t, err)
		files, err := TypeScriptResourceGenerator().Generate(kinds...)
		require.NoError(t, err)
		// Check number of files generated
		assert.Len(t, files, 8)
		// Check content against the golden files
		compareToGolden(t, files, "typescript/versioned")
	})
}

func TestManifestGenerator(t *testing.T) {
	parser, err := NewParser(os.DirFS(TestCUEDirectory), "config")
	require.NoError(t, err)

	cfg := parser.GetParsedConfig()

	t.Run("resource", func(t *testing.T) {
		kinds, err := parser.ManifestParser().Parse("testManifest")
		require.NoError(t, err)
		files, err := ManifestGenerator(cfg.CustomResourceDefinitions).Generate(kinds...)
		require.NoError(t, err)
		// Check number of files generated
		// 5 -> object, spec, metadata, status, schema
		assert.Len(t, files, 1)
		// Check content against the golden files
		compareToGolden(t, files, "manifest")
	})
}

func TestManifestGoGenerator(t *testing.T) {
	parser, err := NewParser(os.DirFS(TestCUEDirectory), "config")
	require.NoError(t, err)

	t.Run("group by group", func(t *testing.T) {
		kinds, err := parser.ManifestParser().Parse("testManifest")
		require.NoError(t, err)
		files, err := ManifestGoGenerator("manifestdata", true, "codegen-tests", "pkg/generated", "manifestdata", true).Generate(kinds...)
		require.NoError(t, err)
		// Check number of files generated
		// 14 -> manifest file (1), then the custom route response+query+body for reconcile (3), response body and wrapper+query+body for search in v3 (4), request, response, and wrapper for /foobar in v3 (3), +1 client per version (3)
		require.Len(t, files, 14, "should be 14 files generated, got %d", len(files))
		// Check content against the golden files
		for _, file := range files {
			compareToGolden(t, codejen.Files{file}, "go/groupbygroup")
		}
	})

	t.Run("group by kind", func(t *testing.T) {
		kinds, err := parser.ManifestParser().Parse("customManifest")
		require.NoError(t, err)
		files, err := ManifestGoGenerator("manifestdata", true, "codegen-tests", "pkg/generated", "manifestdata", false).Generate(kinds...)
		require.NoError(t, err)
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
		require.NoError(t, err)
		// Compare the contents of the file to the generated contents
		// Use strings for easier-to-read diff in the event of a mismatch
		assert.Equal(t, string(file), string(f.Data))
	}
}
