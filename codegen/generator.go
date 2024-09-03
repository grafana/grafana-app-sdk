package codegen

import (
	"io/fs"

	"github.com/grafana/codejen"
)

type JennyList[T any] interface {
	Generate(...T) (codejen.Files, error)
}

type Parser[T any] interface {
	Parse(fs.FS, ...string) ([]T, error)
}

func NewGenerator[T any](parser Parser[T], files fs.FS) (*Generator[T], error) {
	return &Generator[T]{
		p:     parser,
		files: files,
	}, nil
}

type Generator[T any] struct {
	files fs.FS
	p     Parser[T]
}

func (g *Generator[T]) Generate(jennies JennyList[T], selectors ...string) (codejen.Files, error) {
	return g.FilteredGenerate(jennies, func(_ T) bool {
		return true
	}, selectors...)
}

func (g *Generator[T]) FilteredGenerate(jennies JennyList[T], filterFunc func(T) bool, selectors ...string) (codejen.Files, error) {
	kinds, err := g.p.Parse(g.files, selectors...)
	if err != nil {
		return nil, err
	}
	filteredKinds := make([]T, 0)
	for _, kind := range kinds {
		if !filterFunc(kind) {
			continue
		}
		filteredKinds = append(filteredKinds, kind)
	}
	return jennies.Generate(filteredKinds...)
}
