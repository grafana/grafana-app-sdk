package codegen

import (
	"encoding/json"
	"os"
	"testing"

	"cuelang.org/go/cue/cuecontext"
	"github.com/grafana/thema"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gopkg.in/yaml.v3"
)

func TestCrdGenerator_Generate_JSON(t *testing.T) {
	parser, err := NewCustomKindParser(thema.NewRuntime(cuecontext.New()), os.DirFS(testCueDir))
	require.Nil(t, err)
	files, err := parser.Generate(wrapJenny(&crdGenerator{
		outputExtension: "json",
		outputEncoder:   json.Marshal,
	}), "customKind")
	// Check number of files generated
	assert.Len(t, files, 1)
	// Check content against the golden files
	compareToGolden(t, files, "")
}

func TestCrdGenerator_Generate_YAML(t *testing.T) {
	parser, err := NewCustomKindParser(thema.NewRuntime(cuecontext.New()), os.DirFS(testCueDir))
	require.Nil(t, err)
	files, err := parser.Generate(wrapJenny(&crdGenerator{
		outputExtension: "yaml",
		outputEncoder:   yaml.Marshal,
	}), "customKind")
	// Check number of files generated
	assert.Len(t, files, 1)
	// Check content against the golden files
	compareToGolden(t, files, "")
}

func TestCrdGenerator_JennyName(t *testing.T) {
	g := &crdGenerator{}
	assert.Equal(t, "CRD Generator", g.JennyName())
}
