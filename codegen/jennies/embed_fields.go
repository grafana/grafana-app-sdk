package jennies

import (
	"fmt"

	"cuelang.org/go/cue"

	"github.com/grafana/grafana-app-sdk/codegen"
)

func validateEmbedFields(vk codegen.VersionedKind, version string) error {
	if vk.Embed == nil || !vk.Schema.Exists() {
		return nil
	}
	for _, field := range vk.Embed.Fields {
		// A custom builder supplies pathless fields; they need no resource schema entry.
		if field.Path == "" {
			continue
		}
		leaf, resolved, err := resolveSearchFieldPath(vk.Schema, field.Path)
		if err == nil && resolved && !embedFieldSupportsText(leaf) {
			err = fmt.Errorf("embedding input must be a string or string array, got schema type %s", leaf.IncompleteKind())
		}
		if err != nil {
			return fmt.Errorf("kind %q version %q embed field %q: %w", vk.Kind, version, field.Name, err)
		}
	}
	return nil
}

func embedFieldSupportsText(leaf cue.Value) bool {
	kind := leaf.IncompleteKind() &^ cue.NullKind
	if kind == 0 || kind&cue.StringKind != 0 {
		return true
	}
	if kind != cue.ListKind {
		return false
	}

	// Remove null before inspecting the list. Checking only whether it unifies
	// with [...string] would also accept [...int], since both permit an empty list.
	list := leaf.Unify(leaf.Context().CompileString("[...]"))
	if elem := list.LookupPath(cue.MakePath(cue.AnyIndex)); elem.Exists() && !embedScalarSupportsText(elem) {
		return false
	}
	values, err := list.List()
	if err != nil {
		return false
	}
	for values.Next() {
		if !embedScalarSupportsText(values.Value()) {
			return false
		}
	}
	return true
}

func embedScalarSupportsText(value cue.Value) bool {
	kind := value.IncompleteKind() &^ cue.NullKind
	return kind == 0 || kind&cue.StringKind != 0
}
