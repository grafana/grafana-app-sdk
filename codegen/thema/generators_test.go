package thema

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"cuelang.org/go/cue/cuecontext"
	"github.com/grafana/codejen"
	"github.com/grafana/thema"
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

	parser, err := NewCustomKindParser(thema.NewRuntime(cuecontext.New()), os.DirFS(TestCUEDirectory))
	require.Nil(t, err)

	t.Run("JSON", func(t *testing.T) {
		files, err := parser.Generate(CRDGenerator(json.Marshal, "json"), "customKind")
		require.Nil(t, err)
		// Check number of files generated
		assert.Len(t, files, 1)
		// Check content against the golden files
		compareToGolden(t, files, "crd")
	})

	t.Run("YAML", func(t *testing.T) {
		files, err := parser.Generate(CRDGenerator(yaml.Marshal, "yaml"), "customKind")
		require.Nil(t, err)
		// Check number of files generated
		assert.Len(t, files, 1)
		// Check content against the golden files
		compareToGolden(t, files, "crd")
	})
}

func TestResourceGenerator(t *testing.T) {
	// Ideally, we test only that this outputs the right jennies,
	// but right now we just test the whole pipeline from thema -> written files

	parser, err := NewCustomKindParser(thema.NewRuntime(cuecontext.New()), os.DirFS(TestCUEDirectory))
	require.Nil(t, err)

	files, err := parser.Generate(ResourceGenerator(), "customKind")
	require.NoError(t, err)
	// Check number of files generated
	// 8 -> object, spec, metadata, status, lineage (CUE), lineage (go), schema, codec, cue.mod/module.cue
	assert.Len(t, files, 9)
	// Since the codec file differs for thema, we need to use the re-named version we have in testing/golden_generated
	for i := 0; i < len(files); i++ {
		if strings.Contains(files[i].RelativePath, "codec_gen") {
			files[i].RelativePath = strings.Replace(files[i].RelativePath, "codec_gen", "thema_codec_gen", 1)
		}
	}
	// Check content against the golden files
	compareToGolden(t, files, "go/unversioned")
}

func TestModelsGenerator(t *testing.T) {
	// Ideally, we test only that this outputs the right jennies,
	// but right now we just test the whole pipeline from thema -> written files

	parser, err := NewCustomKindParser(thema.NewRuntime(cuecontext.New()), os.DirFS(TestCUEDirectory))
	require.Nil(t, err)

	files, err := parser.Generate(ModelsGenerator(), "customKind2")
	require.Nil(t, err)
	// Check number of files generated
	// 4 -> go type, lineage, functions wrapper for type/lineage, cue module
	assert.Len(t, files, 4)
	// Check content against the golden files
	compareToGolden(t, files, "go/unversioned")
}

func TestTypeScriptModelsGenerator(t *testing.T) {
	// Ideally, we test only that this outputs the right jennies,
	// but right now we just test the whole pipeline from thema -> written files

	parser, err := NewCustomKindParser(thema.NewRuntime(cuecontext.New()), os.DirFS(TestCUEDirectory))
	require.Nil(t, err)

	t.Run("resource", func(t *testing.T) {
		files, err := parser.Generate(TypeScriptModelsGenerator(), "customKind")
		require.Nil(t, err)
		// Check number of files generated
		assert.Len(t, files, 1)
		// Check content against the golden files
		compareToGolden(t, files, "typescript/unversioned")
	})
	t.Run("model", func(t *testing.T) {
		files, err := parser.Generate(TypeScriptModelsGenerator(), "customKind2")
		require.Nil(t, err)
		// Check number of files generated
		assert.Len(t, files, 1)
		// Check content against the golden files
		compareToGolden(t, files, "typescript/unversioned")
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
