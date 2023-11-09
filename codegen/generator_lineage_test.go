package codegen

import (
	"os"
	"testing"

	"cuelang.org/go/cue/cuecontext"
	"github.com/grafana/thema"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestLineageGenerator_Generate(t *testing.T) {
	parser, err := NewCustomKindParser(thema.NewRuntime(cuecontext.New()), os.DirFS(testCueDir))
	require.Nil(t, err)
	files, err := parser.Generate(wrapJenny(&lineageGenerator{}), "customKind")
	// Check number of files generated
	assert.Len(t, files, 1)
	// Check content against the golden files
	compareToGolden(t, files, "")
}

func TestLineageGenerator_JennyName(t *testing.T) {
	g := &lineageGenerator{}
	assert.Equal(t, "CustomKindParser", g.JennyName())
}

func TestCueGenerator_Generate(t *testing.T) {
	parser, err := NewCustomKindParser(thema.NewRuntime(cuecontext.New()), os.DirFS(testCueDir))
	require.Nil(t, err)
	files, err := parser.Generate(wrapJenny(&cueGenerator{}), "customKind")
	// Check number of files generated (lineage and cue.module)
	assert.Len(t, files, 2)
	// Check content against the golden files
	compareToGolden(t, files, "")
}

func TestCueGenerator_JennyName(t *testing.T) {
	g := &cueGenerator{}
	assert.Equal(t, "CUEGenerator", g.JennyName())
}
