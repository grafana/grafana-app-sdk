package jennies

import (
	"bytes"
	"go/ast"
	"go/parser"
	"go/token"
	"slices"
	"strings"
)

// alignAliasComments gives local aliases and their target the same documentation.
// Gengo treats them as one type and warns when their comments differ, including
// when cog puts a union's description on its alias but not its generated struct.
func alignAliasComments(data []byte) ([]byte, error) {
	fset := token.NewFileSet()
	file, err := parser.ParseFile(fset, "", data, parser.ParseComments)
	if err != nil {
		return nil, err
	}
	groups := groupAliasDeclarations(file)

	type edit struct {
		start, end int
		text       string
	}
	var edits []edit
groupsLoop:
	for _, group := range groups {
		if len(group) < 2 {
			continue
		}
		var descriptions, directives []string
		for i, decl := range group {
			var description, tags []string
			for line := range strings.SplitSeq(strings.TrimSpace(decl.Doc.Text()), "\n") {
				if strings.HasPrefix(strings.TrimSpace(line), "+") {
					tags = append(tags, line)
				} else {
					description = append(description, line)
				}
			}
			// Do not hide conflicting directives or change which types are generated.
			if i == 0 {
				directives = tags
			} else if !slices.Equal(directives, tags) {
				continue groupsLoop
			}
			text := strings.TrimSpace(strings.Join(description, "\n"))
			if text != "" && !slices.Contains(descriptions, text) {
				descriptions = append(descriptions, text)
			}
		}
		text := strings.Join(descriptions, "\n\n")
		if len(directives) > 0 {
			if text != "" {
				text += "\n"
			}
			text += strings.Join(directives, "\n")
		}
		if text == "" {
			continue
		}
		text = "// " + strings.ReplaceAll(text, "\n", "\n// ") + "\n"
		for _, decl := range group {
			start, end := fset.Position(decl.Pos()).Offset, fset.Position(decl.Pos()).Offset
			if decl.Doc != nil {
				start = fset.Position(decl.Doc.Pos()).Offset
				end = fset.Position(decl.Doc.End()).Offset + 1
			}
			edits = append(edits, edit{start: start, end: end, text: text})
		}
	}
	slices.SortFunc(edits, func(a, b edit) int { return a.start - b.start })
	var out bytes.Buffer
	last := 0
	for _, edit := range edits {
		out.Write(data[last:edit.start])
		out.WriteString(edit.text)
		last = edit.end
	}
	out.Write(data[last:])
	return out.Bytes(), nil
}

func groupAliasDeclarations(file *ast.File) map[string][]*ast.GenDecl {
	type declaration struct {
		gen  *ast.GenDecl
		spec *ast.TypeSpec
	}
	types := make(map[string]declaration)
	var names []string
	for _, decl := range file.Decls {
		gen, ok := decl.(*ast.GenDecl)
		if !ok || gen.Tok != token.TYPE || len(gen.Specs) != 1 || gen.Lparen.IsValid() {
			continue
		}
		spec, ok := gen.Specs[0].(*ast.TypeSpec)
		if !ok {
			continue
		}
		types[spec.Name.Name] = declaration{gen: gen, spec: spec}
		names = append(names, spec.Name.Name)
	}

	groups := make(map[string][]*ast.GenDecl)
	for _, name := range names {
		target := name
		seen := make(map[string]bool)
		for types[target].spec != nil && !seen[target] {
			seen[target] = true
			spec := types[target].spec
			if !spec.Assign.IsValid() {
				break
			}
			ref, ok := spec.Type.(*ast.Ident)
			if !ok {
				target = ""
				break
			}
			target = ref.Name
		}
		if types[target].spec == nil || types[target].spec.Assign.IsValid() {
			continue
		}
		groups[target] = append(groups[target], types[name].gen)
	}

	return groups
}
