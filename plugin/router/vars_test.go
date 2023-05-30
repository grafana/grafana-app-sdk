package router_test

import (
	"context"
	"testing"

	"github.com/grafana/grafana-app-sdk/plugin/router"
	"github.com/stretchr/testify/assert"
)

func TestVars_GetSet(t *testing.T) {
	tests := []struct {
		name     string
		vars     router.Vars
		addName  string
		addValue string
		getName  string
		want     string
		wantOk   bool
	}{
		{
			name:     "when retrieving from empty Vars",
			vars:     router.NewVars(nil),
			addName:  "",
			addValue: "",
			getName:  "some",
			want:     "",
			wantOk:   false,
		},
		{
			name: "when retrieving from non-empty Vars",
			vars: router.NewVars(map[string]string{
				"some": "value",
			}),
			addName:  "",
			addValue: "",
			getName:  "some",
			want:     "value",
			wantOk:   true,
		},
		{
			name: "when retrieving missing key from non-empty Vars",
			vars: router.NewVars(map[string]string{
				"some": "value",
			}),
			addName:  "",
			addValue: "",
			getName:  "other",
			want:     "",
			wantOk:   false,
		},
		{
			name:     "when adding & retrieving from empty Vars",
			vars:     router.NewVars(nil),
			addName:  "some",
			addValue: "value",
			getName:  "some",
			want:     "value",
			wantOk:   true,
		},
		{
			name: "when adding & retrieving from non-empty Vars",
			vars: router.NewVars(map[string]string{
				"some": "value",
			}),
			addName:  "other",
			addValue: "other-value",
			getName:  "other",
			want:     "other-value",
			wantOk:   true,
		},
		{
			name: "when adding & retrieving existing key from non-empty Vars",
			vars: router.NewVars(map[string]string{
				"some": "value",
			}),
			addName:  "some",
			addValue: "other-value",
			getName:  "some",
			want:     "other-value",
			wantOk:   true,
		},
		{
			name: "when adding new key & retrieving missing key from non-empty Vars",
			vars: router.NewVars(map[string]string{
				"some": "value",
			}),
			addName:  "other",
			addValue: "other-value",
			getName:  "yet-another",
			want:     "",
			wantOk:   false,
		},
		{
			name:     "when adding new key with empty value to empty Vars",
			vars:     router.NewVars(nil),
			addName:  "other",
			addValue: "",
			getName:  "other",
			want:     "",
			wantOk:   false,
		},
		{
			name:     "when adding new key to nil Vars",
			vars:     nil,
			addName:  "other",
			addValue: "value",
			getName:  "other",
			want:     "value",
			wantOk:   true,
		},
		{
			name:     "when reading a key from nil Vars",
			vars:     nil,
			addName:  "",
			addValue: "",
			getName:  "some",
			want:     "",
			wantOk:   false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			vars := tt.vars
			if tt.addName != "" {
				vars = vars.Add(tt.addName, tt.addValue)
			}

			val, ok := vars.Get(tt.getName)
			assert.Equal(t, tt.want, val)
			assert.Equal(t, tt.wantOk, ok)
		})
	}
}

func TestVars_Context(t *testing.T) {
	tests := []struct {
		name     string
		ctx      context.Context
		addName  string
		addValue string
		want     router.Vars
	}{
		{
			name:     "when retrieving empty Vars",
			ctx:      context.Background(),
			addName:  "",
			addValue: "",
			want:     router.NewVars(nil),
		},
		{
			name:     "when no Vars are added",
			ctx:      router.CtxWithVar(context.Background(), "some", "value"),
			addName:  "",
			addValue: "",
			want: router.NewVars(map[string]string{
				"some": "value",
			}),
		},
		{
			name:     "when some Vars are added",
			ctx:      router.CtxWithVar(context.Background(), "some", "value"),
			addName:  "other",
			addValue: "other-value",
			want: router.NewVars(map[string]string{
				"some":  "value",
				"other": "other-value",
			}),
		},
		{
			name:     "when some Vars are added but none yet exist",
			ctx:      context.Background(),
			addName:  "other",
			addValue: "other-value",
			want: router.NewVars(map[string]string{
				"other": "other-value",
			}),
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ctx := tt.ctx

			if tt.addName != "" {
				ctx = router.CtxWithVar(ctx, tt.addName, tt.addValue)
			}

			assert.Equal(t, tt.want, router.VarsFromCtx(ctx))
		})
	}
}
