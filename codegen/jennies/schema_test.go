package jennies

import (
	"testing"

	"cuelang.org/go/cue/cuecontext"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestGetCUEValueKindString(t *testing.T) {
	ctx := cuecontext.New()

	tests := []struct {
		name      string
		cue       string
		want      string
		wantErr   bool
	}{
		{
			name: "plain string",
			cue:  `"hello"`,
			want: "string",
		},
		{
			name: "string type",
			cue:  `string`,
			want: "string",
		},
		{
			name: "regex-constrained string",
			cue:  `string & =~"^[a-z]+"`,
			want: "string",
		},
		{
			name: "enum of strings",
			cue:  `"foo" | "bar" | "baz"`,
			want: "string",
		},
		{
			name: "plain bool",
			cue:  `bool`,
			want: "bool",
		},
		{
			name: "bool literal",
			cue:  `true | false`,
			want: "bool",
		},
		{
			name: "plain int",
			cue:  `int`,
			want: "int",
		},
		{
			name: "int with min constraint",
			cue:  `int & >=0`,
			want: "int",
		},
		{
			name: "int64",
			cue:  `int64`,
			want: "int",
		},
		{
			name: "unsupported float",
			cue:  `float`,
			wantErr: true,
		},
		{
			name: "unsupported struct",
			cue:  `{field: string}`,
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			v := ctx.CompileString(tt.cue)
			require.NoError(t, v.Err())

			got, err := getCUEValueKindString(v)
			if tt.wantErr {
				assert.Error(t, err)
			} else {
				require.NoError(t, err)
				assert.Equal(t, tt.want, got)
			}
		})
	}
}
