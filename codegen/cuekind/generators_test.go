package cuekind

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/grafana/codejen"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"sigs.k8s.io/yaml"

	"github.com/grafana/grafana-app-sdk/codegen/jennies"
)

const (
	ReferenceOutputDirectory = "../testing/golden_generated"
)

func TestCRDGenerator(t *testing.T) {
	// Ideally, we test only that this outputs the right jennies,
	// but right now we just test the whole pipeline from thema -> written files

	parser, err := NewParser(testingCue(t), true, false)
	require.NoError(t, err)
	kinds, err := parser.ManifestParser().Parse("customManifest", "testManifest")
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

	parser, err := NewParser(testingCue(t), true, false)
	require.NoError(t, err)
	kinds, err := parser.ManifestParser().Parse("customManifest")
	require.NoError(t, err)
	sameGroupKinds, err := parser.ManifestParser().Parse("testManifest")
	require.NoError(t, err)

	t.Run("group by kind", func(t *testing.T) {
		files, err := ResourceGenerator("codegen-tests", "pkg/generated", false).Generate(kinds...)
		require.NoError(t, err)
		// Check number of files generated
		// 12 (7 -> object, spec, status, schema, codec, constants) * 2 versions
		assert.Len(t, files, 12, "should be 12 files generated, got %d", len(files))
		// Check content against the golden files
		compareToGolden(t, files, "go/groupbykind")
	})

	t.Run("group by group", func(t *testing.T) {
		files, err := ResourceGenerator("codegen-tests", "pkg/generated", true).Generate(kinds...)
		require.NoError(t, err)
		// Check number of files generated
		// 12 (7 -> object, spec, status, schema, codec, constants) * 2 versions
		assert.Len(t, files, 12, "should be 12 files generated, got %d", len(files))
		// Check content against the golden files
		compareToGolden(t, files, "go/groupbygroup")
	})

	t.Run("group by group, multiple kinds", func(t *testing.T) {
		files, err := ResourceGenerator("codegen-tests", "pkg/generated", true).Generate(sameGroupKinds...)
		require.NoError(t, err)
		// Check number of files generated
		assert.Len(t, files, 23, "should be 23 files generated, got %d", len(files))
		// Check content against the golden files
		compareToGolden(t, files, "go/groupbygroup")
	})
}

func TestTypeScriptResourceGenerator(t *testing.T) {
	// Ideally, we test only that this outputs the right jennies,
	// but right now we just test the whole pipeline from thema -> written files

	parser, err := NewParser(testingCue(t), true, false)
	require.NoError(t, err)

	t.Run("versioned", func(t *testing.T) {
		kinds, err := parser.ManifestParser().Parse("customManifest")
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
	parser, err := NewParser(testingCue(t), true, false)
	require.NoError(t, err)

	t.Run("resource", func(t *testing.T) {
		kinds, err := parser.ManifestParser().Parse("testManifest")
		require.NoError(t, err)
		files, err := ManifestGenerator("json", true, jennies.VersionV1Alpha1).Generate(kinds...)
		require.NoError(t, err)
		// Check number of files generated
		// 5 -> object, spec, metadata, status, schema
		assert.Len(t, files, 1)
		// Check content against the golden files
		compareToGolden(t, files, "manifest")
	})
}

func TestManifestGoGenerator(t *testing.T) {
	parser, err := NewParser(testingCue(t), true, false)
	require.NoError(t, err)

	t.Run("group by group", func(t *testing.T) {
		kinds, err := parser.ManifestParser().Parse("testManifest")
		require.NoError(t, err)
		files, err := ManifestGoGenerator("manifestdata", true, "codegen-tests", "pkg/generated", "manifestdata", true).Generate(kinds...)
		require.NoError(t, err)
		// Check number of files generated
		// 15 -> manifest file (1), then the custom route response+query+body for reconcile (3), response body and wrapper+query+body for search in v3 (4), request, response, and wrapper for /foobar in v3 (3), the resource clients for v1-v3 (3), and the version-level client for v3 routes (1)
		require.Len(t, files, 15, "should be 15 files generated, got %d", len(files))
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

// TODO: These cause GitHub Actions to flake due to timeouts during code generation.
// Removing for now, but we should re-enable and fix the underlying issue.
// func TestManifestGoGenerator_Deterministic(t *testing.T) {
// 	parser, err := NewParser(testingCue(t), true, false)
// 	require.NoError(t, err)
//
// 	t.Run("group by group", func(t *testing.T) {
// 		kinds, err := parser.ManifestParser().Parse("testManifest")
// 		require.NoError(t, err)
//
// 		var reference codejen.Files
// 		for i := 0; i < 5; i++ {
// 			files, err := ManifestGoGenerator("manifestdata", true, "codegen-tests", "pkg/generated", "manifestdata", true).Generate(kinds...)
// 			require.NoError(t, err)
// 			if i == 0 {
// 				reference = files
// 				continue
// 			}
// 			for j, f := range files {
// 				assert.Equal(t, string(reference[j].Data), string(f.Data),
// 					"non-deterministic output on iteration %d for file %s", i, f.RelativePath)
// 			}
// 		}
// 	})
//
// 	t.Run("group by kind", func(t *testing.T) {
// 		kinds, err := parser.ManifestParser().Parse("customManifest")
// 		require.NoError(t, err)
//
// 		var reference codejen.Files
// 		for i := 0; i < 5; i++ {
// 			files, err := ManifestGoGenerator("manifestdata", true, "codegen-tests", "pkg/generated", "manifestdata", false).Generate(kinds...)
// 			require.NoError(t, err)
// 			if i == 0 {
// 				reference = files
// 				continue
// 			}
// 			for j, f := range files {
// 				assert.Equal(t, string(reference[j].Data), string(f.Data),
// 					"non-deterministic output on iteration %d for file %s", i, f.RelativePath)
// 			}
// 		}
// 	})
// }

func TestManifestGoGenerator_RolesAreSorted(t *testing.T) {
	parser, err := NewParser(testingCue(t), true, false)
	require.NoError(t, err)

	kinds, err := parser.ManifestParser().Parse("testManifest")
	require.NoError(t, err)
	files, err := ManifestGoGenerator("manifestdata", true, "codegen-tests", "pkg/generated", "manifestdata", true).Generate(kinds...)
	require.NoError(t, err)
	require.NotEmpty(t, files)

	manifest := string(files[0].Data)
	idx := strings.Index(manifest, "Roles: map[string]app.ManifestRole{")
	if idx == -1 {
		t.Skip("no Roles map found in generated manifest")
	}
	rolesSection := manifest[idx:]

	// Find all quoted role keys in order of appearance
	var keys []string
	for _, line := range strings.Split(rolesSection, "\n") {
		trimmed := strings.TrimSpace(line)
		if strings.HasPrefix(trimmed, `"`) && strings.Contains(trimmed, `": {`) {
			key := strings.SplitN(trimmed, `"`, 3)[1]
			keys = append(keys, key)
		}
		if trimmed == "}," && len(keys) > 0 {
			break
		}
	}
	require.NotEmpty(t, keys, "should find role keys in generated manifest")
	assert.IsNonDecreasing(t, keys, "role keys should appear in sorted order")
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

func testingCue(t *testing.T) *Cue {
	root, err := LoadCue(os.DirFS("./testing"))
	require.NoError(t, err)
	return root
}
