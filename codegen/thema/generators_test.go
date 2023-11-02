package thema

import (
	"encoding/json"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
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
		compareToGolden(t, files, "")
	})

	t.Run("YAML", func(t *testing.T) {
		files, err := parser.Generate(CRDGenerator(yaml.Marshal, "yaml"), "customKind")
		require.Nil(t, err)
		// Check number of files generated
		assert.Len(t, files, 1)
		// Check content against the golden files
		os.WriteFile("crd.yaml", files[0].Data, fs.ModePerm)
		compareToGolden(t, files, "")
	})
}

func TestResourceGenerator(t *testing.T) {
	// Ideally, we test only that this outputs the right jennies,
	// but right now we just test the whole pipeline from thema -> written files

	parser, err := NewCustomKindParser(thema.NewRuntime(cuecontext.New()), os.DirFS(TestCUEDirectory))
	require.Nil(t, err)

	files, err := parser.Generate(ResourceGenerator(), "customKind")
	fmt.Println(err)
	require.Nil(t, err)
	for _, f := range files {
		err := os.WriteFile(f.RelativePath, f.Data, fs.ModePerm)
		fmt.Println(err)
		require.Nil(t, err)
	}
	// Check number of files generated
	//assert.Len(t, files, 1)
	// Check content against the golden files
	compareToGolden(t, files, "")
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
