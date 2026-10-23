package codegen

import (
	"github.com/grafana/codejen"
)

type JennyList[T any] interface {
	Generate(...T) (codejen.Files, error)
}

type Parser[T any] interface {
	Parse(selectors ...string) ([]T, error)
}

func NewGenerator[T any](parser Parser[T]) (*Generator[T], error) {
	return &Generator[T]{
		p: parser,
	}, nil
}

type Generator[T any] struct {
	p Parser[T]
}

func (g *Generator[T]) Generate(jennies JennyList[T], selectors ...string) (codejen.Files, error) {
	return g.FilteredGenerate(jennies, func(_ T) bool {
		return true
	}, selectors...)
}

func (g *Generator[T]) FilteredGenerate(jennies JennyList[T], filterFunc func(T) bool, selectors ...string) (codejen.Files, error) {
	kinds, err := g.p.Parse(selectors...)
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
