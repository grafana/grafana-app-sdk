package codegen

import (
	"embed"
	"fmt"
	"os"
	"testing"

	"cuelang.org/go/cue/cuecontext"
	"github.com/grafana/codejen"
	"github.com/grafana/grafana-app-sdk/kindsys"
	"github.com/grafana/thema"
	"github.com/stretchr/testify/assert"
)

//go:embed testing.cue cue.mod/module.cue
var modFS embed.FS

//go:embed testing_invalidtypes.cue cue.mod/module.cue
var invalidTypesFS embed.FS

func TestNewThemaParser(t *testing.T) {
	t.Run("no cue.mod", func(t *testing.T) {
		g, err := NewCustomKindParser(thema.NewRuntime(cuecontext.New()), os.DirFS("../"))
		assert.Nil(t, g)
		assert.NotNil(t, err)
		assert.Contains(t, err.Error(), "provided fs.FS is not a valid CUE module")
	})
	t.Run("success", func(t *testing.T) {
		g, err := NewCustomKindParser(thema.NewRuntime(cuecontext.New()), modFS)
		assert.Nil(t, err)
		assert.NotNil(t, g)
	})
}

func TestThemaParser_Generate(t *testing.T) {
	t.Run("bad selector", func(t *testing.T) {
		g, err := NewCustomKindParser(thema.NewRuntime(cuecontext.New()), modFS)
		assert.Nil(t, err)
		jen := codejen.JennyListWithNamer[kindsys.Custom](nil)
		jen.Append(&mockKindGenStep{
			GenerateFunc: func(gen kindsys.Custom) (*codejen.File, error) {
				return nil, fmt.Errorf("step should not be called")
			},
		})
		files, err := g.Generate(jen, "bad")
		assert.Empty(t, files)
		assert.Equal(t, fmt.Errorf("bad: selector does not exist"), err)
	})

	t.Run("lineage selector", func(t *testing.T) {
		g, err := NewCustomKindParser(thema.NewRuntime(cuecontext.New()), invalidTypesFS)
		assert.Nil(t, err)
		jen := codejen.JennyListWithNamer[kindsys.Custom](nil)
		jen.Append(&mockKindGenStep{
			GenerateFunc: func(gen kindsys.Custom) (*codejen.File, error) {
				return nil, fmt.Errorf("step should not be called")
			},
		})
		files, err := g.Generate(jen, "testLin")
		assert.Empty(t, files)
		assert.Contains(t, err.Error(), "not a kind")
	})

	t.Run("step error", func(t *testing.T) {
		g, err := NewCustomKindParser(thema.NewRuntime(cuecontext.New()), modFS)
		assert.Nil(t, err)
		jen := codejen.JennyListWithNamer[kindsys.Custom](nil)
		stepErr := fmt.Errorf("I AM ERROR")
		jen.Append(&mockKindGenStep{
			GenerateFunc: func(gen kindsys.Custom) (*codejen.File, error) {
				return nil, stepErr
			},
		})
		files, err := g.Generate(jen, "testCustom")
		assert.Empty(t, files)
		assert.Contains(t, err.Error(), stepErr.Error())
	})

	t.Run("success", func(t *testing.T) {
		g, err := NewCustomKindParser(thema.NewRuntime(cuecontext.New()), modFS)
		assert.Nil(t, err)
		jen := codejen.JennyListWithNamer[kindsys.Custom](nil)
		gFile := codejen.File{
			RelativePath: "foo",
			Data:         []byte("bar"),
			From:         []codejen.NamedJenny{&mockKindGenStep{}},
		}
		jen.Append(&mockKindGenStep{
			GenerateFunc: func(gen kindsys.Custom) (*codejen.File, error) {
				return &gFile, nil
			},
		})
		files, err := g.Generate(jen, "testCustom")
		assert.Nil(t, err)
		assert.ElementsMatch(t, []codejen.File{gFile}, files)
	})

	t.Run("success, all selectors", func(t *testing.T) {
		g, err := NewCustomKindParser(thema.NewRuntime(cuecontext.New()), modFS)
		assert.Nil(t, err)
		gFile := codejen.File{
			RelativePath: "foo",
			Data:         []byte("bar"),
			From:         []codejen.NamedJenny{&mockKindGenStep{}},
		}
		gFile2 := codejen.File{
			RelativePath: "foo2",
			Data:         []byte("bar2"),
			From:         []codejen.NamedJenny{&mockKindGenStep{}},
		}
		jen := codejen.JennyListWithNamer[kindsys.Custom](nil)
		jen.Append(&mockKindGenStep{
			GenerateFunc: func(gen kindsys.Custom) (*codejen.File, error) {
				switch gen.Def().Properties.Name {
				case "Foo":
					return &gFile, nil
				case "Foo2":
					return &gFile2, nil
				}
				return nil, fmt.Errorf("unknown")
			},
		})
		files, err := g.Generate(jen)
		assert.Nil(t, err)
		assert.ElementsMatch(t, []codejen.File{gFile, gFile2}, files)
	})
}

func TestThemaParser_Validate(t *testing.T) {
	t.Run("bad selector", func(t *testing.T) {
		g, err := NewCustomKindParser(thema.NewRuntime(cuecontext.New()), modFS)
		assert.Nil(t, err)
		errs, err := g.Validate("bad")
		assert.Nil(t, err)
		assert.Len(t, errs, 1)
		assert.Equal(t, fmt.Errorf("selector does not exist"), errs["bad"].Errors[0])
	})

	t.Run("lineage selector", func(t *testing.T) {
		g, err := NewCustomKindParser(thema.NewRuntime(cuecontext.New()), invalidTypesFS)
		assert.Nil(t, err)
		errs, err := g.Validate("testLin")
		assert.Nil(t, err)
		assert.Len(t, errs, 1)
		assert.Contains(t, errs["testLin"].Errors[0].Error(), "not a kind")
	})

	t.Run("bad object", func(t *testing.T) {
		g, err := NewCustomKindParser(thema.NewRuntime(cuecontext.New()), invalidTypesFS)
		assert.Nil(t, err)
		errs, err := g.Validate("testFoo")
		assert.Nil(t, err)
		assert.Len(t, errs, 1)
		assert.Contains(t, errs["testFoo"].Errors[0].Error(), "not a kind")
	})

	t.Run("success", func(t *testing.T) {
		g, err := NewCustomKindParser(thema.NewRuntime(cuecontext.New()), modFS)
		assert.Nil(t, err)
		errs, err := g.Validate("testCustom")
		assert.Nil(t, err)
		assert.Empty(t, errs)
	})
}

type mockKindGenStep struct {
	GenerateFunc func(kindsys.Custom) (*codejen.File, error)
}

func (m *mockKindGenStep) Generate(d kindsys.Custom) (*codejen.File, error) {
	if m.GenerateFunc != nil {
		return m.GenerateFunc(d)
	}
	return nil, nil
}

func (m *mockKindGenStep) JennyName() string {
	return "mock"
}
