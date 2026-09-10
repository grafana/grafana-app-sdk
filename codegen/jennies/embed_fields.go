package jennies

import (
	"fmt"

	"cuelang.org/go/cue"

	"github.com/grafana/grafana-app-sdk/codegen"
)

func validateEmbedFields(vk codegen.VersionedKind, version string) error {
	if vk.Embed == nil {
		return nil
	}
	for _, field := range vk.Embed.Fields {
		if field.Path == "" {
			return fmt.Errorf("kind %q version %q embed field %q requires a path", vk.Kind, version, field.Name)
		}
		if !vk.Schema.Exists() {
			continue
		}
		leaf, resolved, err := resolveSearchFieldPath(vk.Schema, field.Path)
		if err == nil {
			if !resolved {
				err = fmt.Errorf("cannot determine embedding input type for path %q across CUE variants", field.Path)
			} else if !embedFieldSupportsText(leaf) {
				err = fmt.Errorf("embedding input must be a string or string array, got schema type %s", leaf.IncompleteKind())
			}
		}
		if err != nil {
			return fmt.Errorf("kind %q version %q embed field %q: %w", vk.Kind, version, field.Name, err)
		}
	}
	return nil
}

func embedFieldSupportsText(leaf cue.Value) bool {
	kind := leaf.IncompleteKind() &^ cue.NullKind
	if kind == cue.StringKind {
		return true
	}
	if kind != cue.ListKind && kind != cue.StringKind|cue.ListKind {
		return false
	}

	// Isolate the list branch before inspecting its elements. Checking only whether it unifies
	// with [...string] would also accept [...int], since both permit an empty list.
	list := leaf.Unify(leaf.Context().CompileString("[...]"))
	elementKinds := cue.BottomKind
	if elem := list.LookupPath(cue.MakePath(cue.AnyIndex)); elem.Exists() {
		elementKinds |= elem.IncompleteKind()
	}
	values, err := list.List()
	if err != nil {
		return false
	}
	for values.Next() {
		elementKinds |= values.Value().IncompleteKind()
	}
	// Empty lists remain valid; null placeholders are allowed when another element or
	// the scalar branch can provide text, as in [string, null] or string | [...null].
	if elementKinds == cue.BottomKind {
		return true
	}
	elementKinds |= kind & cue.StringKind
	return elementKinds&^cue.NullKind == cue.StringKind
}
