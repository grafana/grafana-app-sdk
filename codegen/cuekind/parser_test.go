package cuekind

import (
	"fmt"
	"io/fs"
	"os"
	"testing"

	"github.com/grafana/grafana-app-sdk/codegen/jennies"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

const joinSchema = `{
	metadata: {
		namespace: string
		name: string
		...
	}
}`

func TestParser_Parse(t *testing.T) {
	p := &Parser{}
	kinds, err := p.ParseFS(os.DirFS("./testing"))
	//kinds, err := p.Parse("./testing")
	fmt.Println(err)
	require.Nil(t, err)
	assert.Equal(t, 1, len(kinds))

	// Feed it to the jenny for funzos
	/*file, err := generators.CRDGenerator(yaml.Marshal, "yaml").Generate(kinds[0])
	fmt.Println(err)
	require.Nil(t, err)
	fmt.Println(string(file.Data))
	assert.Greater(t, 0, len(file.Data))*/

	gens := jennies.GoTypes{
		GenerateOnlyCurrent: true,
		Depth:               1,
	}
	files, err := gens.Generate(kinds[0])
	fmt.Println(err)
	require.Nil(t, err)
	assert.Greater(t, 0, len(files))
	for _, f := range files {
		fmt.Println(f.RelativePath)
		err = os.WriteFile(f.RelativePath, f.Data, fs.ModePerm)
		fmt.Println(err)
		require.Nil(t, err)
	}
}

func TestParser_Parse2(t *testing.T) {
	p := Parser{}
	p.Parse2()
	t.Fail()
}
