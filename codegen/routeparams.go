package codegen

import (
	"errors"
	"fmt"
	"regexp"
	"slices"
	"strings"

	"cuelang.org/go/cue"
	"cuelang.org/go/cue/cuecontext"
)

// QuerySchema is the schema of the route's query parameters, whose fields are the parameters: a
// struct with a field for each parameter declared with In "query", optional unless it is required,
// or else the deprecated Query. It does not exist for a route with no query parameters.
func (r CustomRouteRequest) QuerySchema() cue.Value {
	var query []CustomRouteParam
	for _, p := range r.Parameters {
		if p.In == CustomRouteParamInQuery {
			query = append(query, p)
		}
	}
	if len(query) == 0 {
		return r.Query
	}
	ctx := cuecontext.New()
	for _, p := range query {
		if p.Schema.Exists() {
			ctx = p.Schema.Context()
			break
		}
	}
	schema := ctx.CompileString("{}")
	for _, p := range query {
		field := p.Schema
		if !field.Exists() {
			field = ctx.CompileString("string")
		}
		selector := cue.Str(p.Name)
		if p.Required == nil || !*p.Required {
			selector = selector.Optional()
		}
		schema = schema.FillPath(cue.MakePath(selector), field)
	}
	return schema
}

// pathParameterPattern matches a parameter in a route path, such as {name}.
var pathParameterPattern = regexp.MustCompile(`\{([^{}/]+)\}`)

// PathParameterNames are the names of the parameters in a route path. A parameter that matches the
// rest of the path may be written {name:*} or {name...}, and is named name.
func PathParameterNames(path string) []string {
	matches := pathParameterPattern.FindAllStringSubmatch(path, -1)
	names := make([]string, 0, len(matches))
	for _, match := range matches {
		names = append(names, strings.TrimSuffix(strings.TrimSuffix(match[1], ":*"), "..."))
	}
	return names
}

// ValidateRouteParams checks the parameters a route declares. A parameter is identified by its name
// and where it is, as in OpenAPI, so no two share both. A path parameter has to be in the path, and
// is always required. A route declares its query parameters in parameters, or in the deprecated
// query, and not both. An example has to satisfy the parameter's schema, which is a string unless
// the parameter declares one.
func ValidateRouteParams(path string, r CustomRouteRequest) error {
	type identity struct{ name, in string }
	seen := map[identity]bool{}
	inPath := PathParameterNames(path)
	queryParams := 0
	for _, p := range r.Parameters {
		id := identity{p.Name, p.In}
		if seen[id] {
			return fmt.Errorf("%s parameter %q is declared twice", p.In, p.Name)
		}
		seen[id] = true
		switch p.In {
		case CustomRouteParamInPath:
			if !slices.Contains(inPath, p.Name) {
				return fmt.Errorf("path parameter %q is not a parameter in the path %q", p.Name, path)
			}
			if p.Required != nil && !*p.Required {
				return fmt.Errorf("path parameter %q cannot be optional: a path parameter is always required", p.Name)
			}
		case CustomRouteParamInQuery:
			queryParams++
		default:
			// header and cookie parameters have nothing more to check
		}
		if p.Example != nil {
			schema := p.Schema
			if !schema.Exists() {
				schema = cuecontext.New().CompileString("string")
			}
			// Compare kinds: decoding a manifest resolves a default (bool | *false) to its value, so
			// unifying or subsuming would reject any example that is not the default.
			if k := schema.Context().Encode(p.Example).IncompleteKind(); schema.IncompleteKind()&k == 0 {
				return fmt.Errorf("example %v of %s parameter %q does not satisfy its schema: %v is not a %v", p.Example, p.In, p.Name, k, schema.IncompleteKind())
			}
		}
	}
	if queryParams > 0 && r.Query.Exists() {
		return errors.New("the route declares query parameters in parameters and in the deprecated query: declare them in parameters")
	}
	return nil
}
