package codegen

import (
	"os"
	"testing"

	"cuelang.org/go/cue/cuecontext"
	"github.com/grafana/thema"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestResourceObjectGenerator_Generate(t *testing.T) {
	parser, err := NewCustomKindParser(thema.NewRuntime(cuecontext.New()), os.DirFS(testCueDir))
	require.Nil(t, err)
	files, err := parser.Generate(wrapJenny(&resourceObjectGenerator{}))
	// Check number of files generated
	assert.Len(t, files, 1)
	// Check content against the golden files
	compareToGolden(t, files, "")
}
func TestResourceObjectGenerator_JennyName(t *testing.T) {
	g := &resourceObjectGenerator{}
	assert.Equal(t, "ResourceObjectGenerator", g.JennyName())
}
