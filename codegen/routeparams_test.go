package codegen

import (
	"testing"

	"cuelang.org/go/cue"
	"cuelang.org/go/cue/cuecontext"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestCustomRouteRequest_QuerySchema(t *testing.T) {
	ctx := cuecontext.New()

	t.Run("a route with no query parameters has no schema", func(t *testing.T) {
		assert.False(t, CustomRouteRequest{}.QuerySchema().Exists())
		assert.False(t, CustomRouteRequest{Parameters: []CustomRouteParam{{Name: "id", In: "path"}}}.QuerySchema().Exists())
	})

	t.Run("the deprecated query is the schema when there are no query parameters", func(t *testing.T) {
		query := ctx.CompileString(`{q: string}`)
		assert.Equal(t, query, CustomRouteRequest{Query: query}.QuerySchema())
	})

	t.Run("query parameters are the fields, optional unless required", func(t *testing.T) {
		schema := CustomRouteRequest{Parameters: []CustomRouteParam{
			{Name: "q", In: "query", Required: new(true)},
			{Name: "limit", In: "query", Schema: ctx.CompileString(`int | *10`)},
			{Name: "X-Request-Id", In: "header"},
			{Name: "id", In: "path"},
		}}.QuerySchema()
		require.NoError(t, schema.Err())

		fields, err := schema.Fields(cue.Optional(true))
		require.NoError(t, err)
		got := map[string]bool{}
		for fields.Next() {
			got[fields.Selector().Unquoted()] = fields.IsOptional()
		}
		assert.Equal(t, map[string]bool{"q": false, "limit": true}, got, "only the query parameters, and q is required")
	})

	t.Run("a parameter with no schema is a string", func(t *testing.T) {
		schema := CustomRouteRequest{Parameters: []CustomRouteParam{{Name: "q", In: "query", Required: new(true)}}}.QuerySchema()
		fields, err := schema.Fields()
		require.NoError(t, err)
		require.True(t, fields.Next())
		assert.Equal(t, cue.StringKind, fields.Value().IncompleteKind())
	})
}

func TestValidateRouteParams(t *testing.T) {
	ctx := cuecontext.New()
	for _, tc := range []struct {
		name    string
		path    string
		request CustomRouteRequest
		err     string
	}{
		{name: "no parameters", path: "/x"},
		{name: "a path parameter in the path", path: "/x/{id}", request: CustomRouteRequest{Parameters: []CustomRouteParam{{Name: "id", In: "path"}}}},
		{name: "a path parameter that is required", path: "/x/{id}", request: CustomRouteRequest{Parameters: []CustomRouteParam{{Name: "id", In: "path", Required: new(true)}}}},
		{name: "a path parameter not in the path", path: "/x/{id}", request: CustomRouteRequest{Parameters: []CustomRouteParam{{Name: "idd", In: "path"}}}, err: `path parameter "idd" is not a parameter in the path "/x/{id}"`},
		{name: "an optional path parameter", path: "/x/{id}", request: CustomRouteRequest{Parameters: []CustomRouteParam{{Name: "id", In: "path", Required: new(false)}}}, err: "cannot be optional"},
		{name: "a name only in the path", path: "/x/{id}", request: CustomRouteRequest{Parameters: []CustomRouteParam{{Name: "id", In: "query"}}}},
		{name: "twice", path: "/x", request: CustomRouteRequest{Parameters: []CustomRouteParam{{Name: "a", In: "header"}, {Name: "a", In: "header"}}}, err: `header parameter "a" is declared twice`},
		{name: "in other places", path: "/x", request: CustomRouteRequest{Parameters: []CustomRouteParam{{Name: "a", In: "header"}, {Name: "a", In: "cookie"}}}},
		{name: "query in two places", path: "/x", request: CustomRouteRequest{Query: ctx.CompileString(`{a: string}`), Parameters: []CustomRouteParam{{Name: "b", In: "query"}}}, err: "deprecated query"},
		{name: "a deprecated query alone", path: "/x", request: CustomRouteRequest{Query: ctx.CompileString(`{a: string}`)}},
		{name: "an example of the schema's type", path: "/x", request: CustomRouteRequest{Parameters: []CustomRouteParam{{Name: "n", In: "query", Schema: ctx.CompileString(`int`), Example: int64(3)}}}},
		{name: "an example that is not", path: "/x", request: CustomRouteRequest{Parameters: []CustomRouteParam{{Name: "n", In: "query", Schema: ctx.CompileString(`int`), Example: "three"}}}, err: "does not satisfy its schema"},
		{name: "an example of a string", path: "/x", request: CustomRouteRequest{Parameters: []CustomRouteParam{{Name: "n", In: "header", Example: "abc"}}}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			err := ValidateRouteParams(tc.path, tc.request)
			if tc.err == "" {
				require.NoError(t, err)
				return
			}
			require.ErrorContains(t, err, tc.err)
		})
	}
}
