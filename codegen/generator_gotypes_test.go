package codegen

import (
	"os"
	"path/filepath"
	"testing"

	"cuelang.org/go/cue/cuecontext"
	"github.com/grafana/codejen"
	"github.com/grafana/grafana-app-sdk/kindsys"
	"github.com/grafana/thema"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

const testCueDir = "./testing/cue"

func TestResourceGoTypesGenerator_Generate(t *testing.T) {
	parser, err := NewCustomKindParser(thema.NewRuntime(cuecontext.New()), os.DirFS(testCueDir))
	require.Nil(t, err)
	files, err := parser.Generate(wrapJenny(&resourceGoTypesGenerator{}))
	// Check number of files generated (should be spec, status, and metadata)
	assert.Len(t, files, 3)
	// Check content against the golden files
	compareToGolden(t, files, "")
}

func TestResourceGoTypesGenerator_JennyName(t *testing.T) {
	g := &resourceGoTypesGenerator{}
	assert.Equal(t, "GoResourceTypes", g.JennyName())
}

func wrapJenny(jenny codejen.Jenny[kindsys.Custom]) Generator {
	g := codejen.JennyListWithNamer[kindsys.Custom](namerFunc)
	g.Append(jenny)
	return g
}

func compareToGolden(t *testing.T, files codejen.Files, pathPrefix string) {
	for _, f := range files {
		// Check if there's a golden generated file to compare against
		file, err := os.ReadFile(filepath.Join("./testing/golden_generated", f.RelativePath+".txt"))
		require.Nil(t, err)
		// Compare the contents of the file to the generated contents
		// Use strings for easier-to-read diff in the event of a mismatch
		assert.Equal(t, string(file), string(f.Data))
	}
}
